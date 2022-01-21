/*
 * Copyright (c) 2016, Psiphon Inc.
 * All rights reserved.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package server

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	golanglog "log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ooni/psiphon/tunnel-core/psiphon/common"
	"github.com/ooni/psiphon/tunnel-core/psiphon/common/errors"
	"github.com/ooni/psiphon/tunnel-core/psiphon/common/protocol"
	tris "github.com/ooni/psiphon/tunnel-core/oovendor/tls-tris"
)

const WEB_SERVER_IO_TIMEOUT = 10 * time.Second

type webServer struct {
	support *SupportServices
}

// RunWebServer runs a web server which supports tunneled and untunneled
// Psiphon API requests.
//
// The HTTP request handlers are light wrappers around the base Psiphon
// API request handlers from the SSH API transport. The SSH API transport
// is preferred by new clients. The web API transport provides support for
// older clients.
//
// The API is compatible with all tunnel-core clients but not backwards
// compatible with all legacy clients.
//
// Note: new features, including authorizations, are not supported in the
// web API transport.
//
func RunWebServer(
	support *SupportServices,
	shutdownBroadcast <-chan struct{}) error {

	webServer := &webServer{
		support: support,
	}

	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/handshake", webServer.handshakeHandler)
	serveMux.HandleFunc("/connected", webServer.connectedHandler)
	serveMux.HandleFunc("/status", webServer.statusHandler)
	serveMux.HandleFunc("/client_verification", webServer.clientVerificationHandler)

	certificate, err := tris.X509KeyPair(
		[]byte(support.Config.WebServerCertificate),
		[]byte(support.Config.WebServerPrivateKey))
	if err != nil {
		return errors.Trace(err)
	}

	tlsConfig := &tris.Config{
		Certificates: []tris.Certificate{certificate},
	}

	// TODO: inherits global log config?
	logWriter := NewLogWriter()
	defer logWriter.Close()

	// Note: WriteTimeout includes time awaiting request, as per:
	// https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts

	server := &HTTPSServer{
		&http.Server{
			MaxHeaderBytes: MAX_API_PARAMS_SIZE,
			Handler:        serveMux,
			ReadTimeout:    WEB_SERVER_IO_TIMEOUT,
			WriteTimeout:   WEB_SERVER_IO_TIMEOUT,
			ErrorLog:       golanglog.New(logWriter, "", 0),

			// Disable auto HTTP/2 (https://golang.org/doc/go1.6)
			TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		},
	}

	localAddress := net.JoinHostPort(
		support.Config.ServerIPAddress,
		strconv.Itoa(support.Config.WebServerPort))

	listener, err := net.Listen("tcp", localAddress)
	if err != nil {
		return errors.Trace(err)
	}

	log.WithTraceFields(
		LogFields{"localAddress": localAddress}).Info("starting")

	err = nil
	errorChannel := make(chan error)
	waitGroup := new(sync.WaitGroup)

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()

		// Note: will be interrupted by listener.Close()
		err := server.ServeTLS(listener, tlsConfig)

		// Can't check for the exact error that Close() will cause in Accept(),
		// (see: https://code.google.com/p/go/issues/detail?id=4373). So using an
		// explicit stop signal to stop gracefully.
		select {
		case <-shutdownBroadcast:
		default:
			if err != nil {
				select {
				case errorChannel <- errors.Trace(err):
				default:
				}
			}
		}

		log.WithTraceFields(
			LogFields{"localAddress": localAddress}).Info("stopped")
	}()

	select {
	case <-shutdownBroadcast:
	case err = <-errorChannel:
	}

	listener.Close()

	waitGroup.Wait()

	log.WithTraceFields(
		LogFields{"localAddress": localAddress}).Info("exiting")

	return err
}

// convertHTTPRequestToAPIRequest converts the HTTP request query
// parameters and request body to the JSON object import format
// expected by the API request handlers.
func convertHTTPRequestToAPIRequest(
	w http.ResponseWriter,
	r *http.Request,
	requestBodyName string) (common.APIParameters, error) {

	params := make(common.APIParameters)

	for name, values := range r.URL.Query() {

		// Limitations:
		// - This is intended only to support params sent by legacy
		//   clients; non-base array-type params are not converted.
		// - Only the first values per name is used.

		if len(values) > 0 {
			value := values[0]

			// TODO: faster lookup?
			isArray := false
			for _, paramSpec := range baseSessionAndDialParams {
				if paramSpec.name == name {
					isArray = (paramSpec.flags&requestParamArray != 0)
					break
				}
			}

			if isArray {
				// Special case: a JSON encoded array
				var arrayValue []interface{}
				err := json.Unmarshal([]byte(value), &arrayValue)
				if err != nil {
					return nil, errors.Trace(err)
				}
				params[name] = arrayValue
			} else {
				// All other query parameters are simple strings
				params[name] = value
			}
		}
	}

	if requestBodyName != "" {
		r.Body = http.MaxBytesReader(w, r.Body, MAX_API_PARAMS_SIZE)
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return nil, errors.Trace(err)
		}
		var bodyParams map[string]interface{}

		if len(body) != 0 {
			err = json.Unmarshal(body, &bodyParams)
			if err != nil {
				return nil, errors.Trace(err)
			}
			params[requestBodyName] = bodyParams
		}
	}

	return params, nil
}

func (webServer *webServer) lookupGeoIPData(params common.APIParameters) GeoIPData {

	clientSessionID, err := getStringRequestParam(params, "client_session_id")
	if err != nil {
		// Not all clients send this parameter
		return NewGeoIPData()
	}

	return webServer.support.GeoIPService.GetSessionCache(clientSessionID)
}

func (webServer *webServer) handshakeHandler(w http.ResponseWriter, r *http.Request) {

	params, err := convertHTTPRequestToAPIRequest(w, r, "")

	var responsePayload []byte
	if err == nil {
		responsePayload, err = dispatchAPIRequestHandler(
			webServer.support,
			protocol.PSIPHON_WEB_API_PROTOCOL,
			r.RemoteAddr,
			webServer.lookupGeoIPData(params),
			nil,
			protocol.PSIPHON_API_HANDSHAKE_REQUEST_NAME,
			params)
	}

	if err != nil {
		log.WithTraceFields(LogFields{"error": err}).Warning("failed")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// The legacy response format is newline separated, name prefixed values.
	// Within that legacy format, the modern JSON response (containing all the
	// legacy response values and more) is single value with a "Config:" prefix.
	// This response uses the legacy format but omits all but the JSON value.
	responseBody := append([]byte("Config: "), responsePayload...)

	w.WriteHeader(http.StatusOK)
	w.Write(responseBody)
}

func (webServer *webServer) connectedHandler(w http.ResponseWriter, r *http.Request) {

	params, err := convertHTTPRequestToAPIRequest(w, r, "")

	var responsePayload []byte
	if err == nil {
		responsePayload, err = dispatchAPIRequestHandler(
			webServer.support,
			protocol.PSIPHON_WEB_API_PROTOCOL,
			r.RemoteAddr,
			webServer.lookupGeoIPData(params),
			nil, // authorizedAccessTypes not logged in web API transport
			protocol.PSIPHON_API_CONNECTED_REQUEST_NAME,
			params)
	}

	if err != nil {
		log.WithTraceFields(LogFields{"error": err}).Warning("failed")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(responsePayload)
}

func (webServer *webServer) statusHandler(w http.ResponseWriter, r *http.Request) {

	params, err := convertHTTPRequestToAPIRequest(w, r, "statusData")

	var responsePayload []byte
	if err == nil {
		responsePayload, err = dispatchAPIRequestHandler(
			webServer.support,
			protocol.PSIPHON_WEB_API_PROTOCOL,
			r.RemoteAddr,
			webServer.lookupGeoIPData(params),
			nil, // authorizedAccessTypes not logged in web API transport
			protocol.PSIPHON_API_STATUS_REQUEST_NAME,
			params)
	}

	if err != nil {
		log.WithTraceFields(LogFields{"error": err}).Warning("failed")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(responsePayload)
}

// clientVerificationHandler is kept for compliance with older Android clients
func (webServer *webServer) clientVerificationHandler(w http.ResponseWriter, r *http.Request) {

	params, err := convertHTTPRequestToAPIRequest(w, r, "verificationData")

	var responsePayload []byte
	if err == nil {
		responsePayload, err = dispatchAPIRequestHandler(
			webServer.support,
			protocol.PSIPHON_WEB_API_PROTOCOL,
			r.RemoteAddr,
			webServer.lookupGeoIPData(params),
			nil, // authorizedAccessTypes not logged in web API transport
			protocol.PSIPHON_API_CLIENT_VERIFICATION_REQUEST_NAME,
			params)
	}

	if err != nil {
		log.WithTraceFields(LogFields{"error": err}).Warning("failed")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(responsePayload)
}
