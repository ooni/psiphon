/*
 * Copyright (c) 2015, Psiphon Inc.
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

/*
Copyright (c) 2012 The Go Authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google Inc. nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

// Originally based on https://gopkg.in/getlantern/tlsdialer.v1.

package psiphon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	std_errors "errors"
	"io/ioutil"
	"net"

	"github.com/ooni/psiphon/tunnel-core/psiphon/common"
	"github.com/ooni/psiphon/tunnel-core/psiphon/common/errors"
	"github.com/ooni/psiphon/tunnel-core/psiphon/common/parameters"
	"github.com/ooni/psiphon/tunnel-core/psiphon/common/prng"
	"github.com/ooni/psiphon/tunnel-core/psiphon/common/protocol"
	tris "github.com/ooni/psiphon/tunnel-core/oovendor/tls-tris"
	utls "github.com/refraction-networking/utls"
)

// CustomTLSConfig specifies the parameters for a CustomTLSDial, supporting
// many TLS-related network obfuscation mechanisms.
type CustomTLSConfig struct {

	// Parameters is the active set of parameters.Parameters to use for the TLS
	// dial. Must not be nil.
	Parameters *parameters.Parameters

	// Dial is the network connection dialer. TLS is layered on top of a new
	// network connection created with dialer. Must not be nil.
	Dial common.Dialer

	// DialAddr overrides the "addr" input to Dial when specified
	DialAddr string

	// UseDialAddrSNI specifies whether to always use the dial "addr"
	// host name in the SNI server_name field. When DialAddr is set,
	// its host name is used.
	UseDialAddrSNI bool

	// SNIServerName specifies the value to set in the SNI
	// server_name field. When blank, SNI is omitted. Note that
	// underlying TLS code also automatically omits SNI when
	// the server_name is an IP address.
	// SNIServerName is ignored when UseDialAddrSNI is true.
	SNIServerName string

	// VerifyServerName specifies a domain name that must appear in the server
	// certificate. When specified, certificate verification checks for
	// VerifyServerName in the server certificate, in place of the dial or SNI
	// hostname.
	VerifyServerName string

	// VerifyPins specifies one or more certificate pin values, one of which must
	// appear in the verified server certificate chain. A pin value is the
	// base64-encoded SHA2 digest of a certificate's public key. When specified,
	// at least one pin must match at least one certificate in the chain, at any
	// position; e.g., the root CA may be pinned, or the server certificate,
	// etc.
	VerifyPins []string

	// VerifyLegacyCertificate is a special case self-signed server
	// certificate case. Ignores IP SANs and basic constraints. No
	// certificate chain. Just checks that the server presented the
	// specified certificate.
	//
	// When VerifyLegacyCertificate is set, none of VerifyServerName, VerifyPins,
	// SkipVerify may be set.
	VerifyLegacyCertificate *x509.Certificate

	// SkipVerify completely disables server certificate verification.
	//
	// When SkipVerify is set, none of VerifyServerName, VerifyPins,
	// VerifyLegacyCertificate may be set.
	SkipVerify bool

	// TLSProfile specifies a particular indistinguishable TLS profile to use for
	// the TLS dial. Setting TLSProfile allows the caller to pin the selection so
	// all TLS connections in a certain context (e.g. a single meek connection)
	// use a consistent value. The value should be selected by calling
	// SelectTLSProfile, which will pick a value at random, subject to
	// compatibility constraints.
	//
	// When TLSProfile is "", a profile is selected at random and
	// DisableFrontingProviderTLSProfiles is ignored.
	TLSProfile string

	// NoDefaultTLSSessionID specifies whether to set a TLS session ID by
	// default, for a new TLS connection that is not resuming a session.
	// When nil, the parameter is set randomly.
	NoDefaultTLSSessionID *bool

	// RandomizedTLSProfileSeed specifies the PRNG seed to use when generating
	// a randomized TLS ClientHello, which applies to TLS profiles where
	// protocol.TLSProfileIsRandomized is true. The PRNG seed allows for
	// optional replay of a particular randomized Client Hello.
	RandomizedTLSProfileSeed *prng.Seed

	// TLSPadding indicates whether to move or add a TLS padding extension to the
	// front of the exension list and apply the specified padding length. Ignored
	// when 0.
	TLSPadding int

	// TrustedCACertificatesFilename specifies a file containing trusted
	// CA certs. See Config.TrustedCACertificatesFilename.
	TrustedCACertificatesFilename string

	// ObfuscatedSessionTicketKey enables obfuscated session tickets
	// using the specified key.
	ObfuscatedSessionTicketKey string

	// PassthroughMessage, when specified, is a 32 byte value that is sent in the
	// ClientHello random value field. The value should be generated using
	// obfuscator.MakeTLSPassthroughMessage.
	PassthroughMessage []byte

	clientSessionCache utls.ClientSessionCache
}

// EnableClientSessionCache initializes a cache to use to persist session
// tickets, enabling TLS session resumability across multiple
// CustomTLSDial calls or dialers using the same CustomTLSConfig.
func (config *CustomTLSConfig) EnableClientSessionCache() {
	if config.clientSessionCache == nil {
		config.clientSessionCache = utls.NewLRUClientSessionCache(0)
	}
}

// NewCustomTLSDialer creates a new dialer based on CustomTLSDial.
func NewCustomTLSDialer(config *CustomTLSConfig) common.Dialer {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return CustomTLSDial(ctx, network, addr, config)
	}
}

// CustomTLSDial dials a new TLS connection using the parameters set in
// CustomTLSConfig.
//
// The dial aborts if ctx becomes Done before the dial completes.
func CustomTLSDial(
	ctx context.Context,
	network, addr string,
	config *CustomTLSConfig) (net.Conn, error) {

	if (config.SkipVerify &&
		(config.VerifyLegacyCertificate != nil ||
			len(config.VerifyServerName) > 0 ||
			len(config.VerifyPins) > 0)) ||

		(config.VerifyLegacyCertificate != nil &&
			(config.SkipVerify ||
				len(config.VerifyServerName) > 0 ||
				len(config.VerifyPins) > 0)) {

		return nil, errors.TraceNew("incompatible certification verification parameters")
	}

	p := config.Parameters.Get()

	dialAddr := addr
	if config.DialAddr != "" {
		dialAddr = config.DialAddr
	}

	rawConn, err := config.Dial(ctx, network, dialAddr)
	if err != nil {
		return nil, errors.Trace(err)
	}

	hostname, _, err := net.SplitHostPort(dialAddr)
	if err != nil {
		rawConn.Close()
		return nil, errors.Trace(err)
	}

	var tlsConfigRootCAs *x509.CertPool
	if !config.SkipVerify &&
		config.VerifyLegacyCertificate == nil &&
		config.TrustedCACertificatesFilename != "" {

		tlsConfigRootCAs = x509.NewCertPool()
		certData, err := ioutil.ReadFile(config.TrustedCACertificatesFilename)
		if err != nil {
			return nil, errors.Trace(err)
		}
		tlsConfigRootCAs.AppendCertsFromPEM(certData)
	}

	// In some cases, config.SkipVerify is false, but
	// utls.Config.InsecureSkipVerify will be set to true to disable verification
	// in utls that will otherwise fail: when SNI is omitted, and when
	// VerifyServerName differs from SNI. In these cases, the certificate chain
	// is verified in VerifyPeerCertificate.

	tlsConfigInsecureSkipVerify := false
	tlsConfigServerName := ""
	verifyServerName := hostname

	if config.SkipVerify {
		tlsConfigInsecureSkipVerify = true
	}

	if config.UseDialAddrSNI {

		// Set SNI to match the dial hostname. This is the standard case.
		tlsConfigServerName = hostname

	} else if config.SNIServerName != "" {

		// Set a custom SNI value. If this value doesn't match the server
		// certificate, SkipVerify and/or VerifyServerName may need to be
		// configured; but by itself this case doesn't necessarily require
		// custom certificate verification.
		tlsConfigServerName = config.SNIServerName

	} else {

		// Omit SNI. If SkipVerify is not set, this case requires custom certificate
		// verification, which will check that the server certificate matches either
		// the dial hostname or VerifyServerName, as if the SNI were set to one of
		// those values.
		tlsConfigInsecureSkipVerify = true
	}

	// When VerifyServerName does not match the SNI, custom certificate
	// verification is necessary.
	if config.VerifyServerName != "" && config.VerifyServerName != tlsConfigServerName {
		verifyServerName = config.VerifyServerName
		tlsConfigInsecureSkipVerify = true
	}

	// With the VerifyPeerCertificate callback, we perform any custom certificate
	// verification at the same point in the TLS handshake as standard utls
	// verification; and abort the handshake at the same point, if custom
	// verification fails.
	var tlsConfigVerifyPeerCertificate func([][]byte, [][]*x509.Certificate) error
	if !config.SkipVerify {
		tlsConfigVerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {

			if config.VerifyLegacyCertificate != nil {
				return verifyLegacyCertificate(
					rawCerts, config.VerifyLegacyCertificate)
			}

			if tlsConfigInsecureSkipVerify {

				// Limitation: this verification path does not set the utls.Conn's
				// ConnectionState certificate information.

				if len(verifiedChains) > 0 {
					return errors.TraceNew("unexpected verified chains")
				}
				var err error
				verifiedChains, err = verifyServerCertificate(
					tlsConfigRootCAs, rawCerts, verifyServerName)
				if err != nil {
					return errors.Trace(err)
				}
			}

			if len(config.VerifyPins) > 0 {
				err := verifyCertificatePins(
					config.VerifyPins, verifiedChains)
				if err != nil {
					return errors.Trace(err)
				}
			}

			return nil
		}
	}

	tlsConfig := &utls.Config{
		RootCAs:               tlsConfigRootCAs,
		InsecureSkipVerify:    tlsConfigInsecureSkipVerify,
		ServerName:            tlsConfigServerName,
		VerifyPeerCertificate: tlsConfigVerifyPeerCertificate,
	}

	selectedTLSProfile := config.TLSProfile

	if selectedTLSProfile == "" {
		selectedTLSProfile = SelectTLSProfile(false, false, "", p)
	}

	utlsClientHelloID, utlsClientHelloSpec, err := getUTLSClientHelloID(
		p, selectedTLSProfile)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var randomizedTLSProfileSeed *prng.Seed
	isRandomized := protocol.TLSProfileIsRandomized(selectedTLSProfile)
	if isRandomized {

		randomizedTLSProfileSeed = config.RandomizedTLSProfileSeed

		if randomizedTLSProfileSeed == nil {

			randomizedTLSProfileSeed, err = prng.NewSeed()
			if err != nil {
				return nil, errors.Trace(err)
			}
		}

		utlsClientHelloID.Seed = new(utls.PRNGSeed)
		*utlsClientHelloID.Seed = [32]byte(*randomizedTLSProfileSeed)
	}

	// As noted here,
	// https://gitlab.com/yawning/obfs4/commit/ca6765e3e3995144df2b1ca9f0e9d823a7f8a47c,
	// the dynamic record sizing optimization in crypto/tls is not commonly
	// implemented in browsers. Disable it for all utls parrots and select it
	// randomly when using the randomized client hello.
	if isRandomized {
		PRNG, err := prng.NewPRNGWithSaltedSeed(randomizedTLSProfileSeed, "tls-dynamic-record-sizing")
		if err != nil {
			return nil, errors.Trace(err)
		}
		tlsConfig.DynamicRecordSizingDisabled = PRNG.FlipCoin()
	} else {
		tlsConfig.DynamicRecordSizingDisabled = true
	}

	conn := utls.UClient(rawConn, tlsConfig, utlsClientHelloID)

	if utlsClientHelloSpec != nil {
		err := conn.ApplyPreset(utlsClientHelloSpec)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	clientSessionCache := config.clientSessionCache
	if clientSessionCache == nil {
		clientSessionCache = utls.NewLRUClientSessionCache(0)
	}

	conn.SetSessionCache(clientSessionCache)

	// TODO: can conn.SetClientRandom be made to take effect if called here? In
	// testing, the random value appears to be overwritten. As is, the overhead
	// of needRemarshal is now always required to handle
	// config.PassthroughMessage.

	// Build handshake state in advance to obtain the TLS version, which is used
	// to determine whether the following customizations may be applied. Don't use
	// getClientHelloVersion, since that may incur additional overhead.

	err = conn.BuildHandshakeState()
	if err != nil {
		return nil, errors.Trace(err)
	}

	isTLS13 := false
	for _, vers := range conn.HandshakeState.Hello.SupportedVersions {
		if vers == utls.VersionTLS13 {
			isTLS13 = true
			break
		}
	}

	// Add the obfuscated session ticket only when using TLS 1.2.
	//
	// Obfuscated session tickets are not currently supported in TLS 1.3, but we
	// allow UNFRONTED-MEEK-SESSION-TICKET-OSSH to use TLS 1.3 profiles for
	// additional diversity/capacity; TLS 1.3 encrypts the server certificate,
	// so the desired obfuscated session tickets property of obfuscating server
	// certificates is satisfied. We know that when the ClientHello offers TLS
	// 1.3, the Psiphon server, in these direct protocol cases, will negotiate
	// it.

	if config.ObfuscatedSessionTicketKey != "" && !isTLS13 {

		var obfuscatedSessionTicketKey [32]byte

		key, err := hex.DecodeString(config.ObfuscatedSessionTicketKey)
		if err == nil && len(key) != 32 {
			err = std_errors.New("invalid obfuscated session key length")
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		copy(obfuscatedSessionTicketKey[:], key)

		obfuscatedSessionState, err := tris.NewObfuscatedClientSessionState(
			obfuscatedSessionTicketKey)
		if err != nil {
			return nil, errors.Trace(err)
		}

		conn.SetSessionState(
			utls.MakeClientSessionState(
				obfuscatedSessionState.SessionTicket,
				obfuscatedSessionState.Vers,
				obfuscatedSessionState.CipherSuite,
				obfuscatedSessionState.MasterSecret,
				nil,
				nil))

		// Apply changes to utls
		err = conn.BuildHandshakeState()
		if err != nil {
			return nil, errors.Trace(err)
		}

		// Ensure that TLS ClientHello has required session ticket extension and
		// obfuscated session ticket cipher suite; the latter is required by
		// utls/tls.Conn.loadSession. If these requirements are not met the
		// obfuscation session ticket would be ignored, so fail.

		if !tris.ContainsObfuscatedSessionTicketCipherSuite(
			conn.HandshakeState.Hello.CipherSuites) {
			return nil, errors.TraceNew(
				"missing obfuscated session ticket cipher suite")
		}

		if len(conn.HandshakeState.Hello.SessionTicket) == 0 {
			return nil, errors.TraceNew("missing session ticket extension")
		}
	}

	// Perform at most one remarshal for the following ClientHello
	// modifications.
	needRemarshal := false

	// Either pre-TLS 1.3 ClientHellos or any randomized ClientHello is a
	// candidate for NoDefaultSessionID logic.
	if len(conn.HandshakeState.Hello.SessionTicket) == 0 &&
		(!isTLS13 || utlsClientHelloID.Client == "Randomized") {

		var noDefaultSessionID bool
		if config.NoDefaultTLSSessionID != nil {
			noDefaultSessionID = *config.NoDefaultTLSSessionID
		} else {
			noDefaultSessionID = config.Parameters.Get().WeightedCoinFlip(
				parameters.NoDefaultTLSSessionIDProbability)
		}

		if noDefaultSessionID {
			conn.HandshakeState.Hello.SessionId = nil
			needRemarshal = true
		}
	}

	// utls doesn't omit the server_name extension when the ServerName value is
	// empty or an IP address. To avoid a fingerprintable invalid/unusual
	// server_name extension, remove it in these cases.
	if tlsConfigServerName == "" || net.ParseIP(tlsConfigServerName) != nil {

		// Assumes only one SNIExtension.
		// TODO: use new UConn.RemoveSNIExtension function?
		deleteIndex := -1
		for index, extension := range conn.Extensions {
			if _, ok := extension.(*utls.SNIExtension); ok {
				deleteIndex = index
				break
			}
		}
		if deleteIndex != -1 {
			conn.Extensions = append(
				conn.Extensions[:deleteIndex], conn.Extensions[deleteIndex+1:]...)
		}
		needRemarshal = true
	}

	if config.TLSPadding > 0 {

		tlsPadding := config.TLSPadding

		// Maximum padding size per RFC 7685
		if tlsPadding > 65535 {
			tlsPadding = 65535
		}

		// Assumes only one PaddingExtension.
		deleteIndex := -1
		for index, extension := range conn.Extensions {
			if _, ok := extension.(*utls.UtlsPaddingExtension); ok {
				deleteIndex = index
				break
			}
		}
		if deleteIndex != -1 {
			conn.Extensions = append(
				conn.Extensions[:deleteIndex], conn.Extensions[deleteIndex+1:]...)
		}

		paddingExtension := &utls.UtlsPaddingExtension{
			PaddingLen: tlsPadding,
			WillPad:    true,
		}
		conn.Extensions = append([]utls.TLSExtension{paddingExtension}, conn.Extensions...)

		needRemarshal = true

	}

	if config.PassthroughMessage != nil {
		err := conn.SetClientRandom(config.PassthroughMessage)
		if err != nil {
			return nil, errors.Trace(err)
		}

		needRemarshal = true
	}

	if needRemarshal {
		// Apply changes to utls
		err = conn.MarshalClientHello()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	// Perform the TLS Handshake.

	resultChannel := make(chan error)

	go func() {
		resultChannel <- conn.Handshake()
	}()

	select {
	case err = <-resultChannel:
	case <-ctx.Done():
		err = ctx.Err()
		// Interrupt the goroutine
		rawConn.Close()
		<-resultChannel
	}

	if err != nil {
		rawConn.Close()
		return nil, errors.Trace(err)
	}

	return conn, nil
}

func verifyLegacyCertificate(rawCerts [][]byte, expectedCertificate *x509.Certificate) error {
	if len(rawCerts) < 1 {
		return errors.TraceNew("missing certificate")
	}
	if !bytes.Equal(rawCerts[0], expectedCertificate.Raw) {
		return errors.TraceNew("unexpected certificate")
	}
	return nil
}

func verifyServerCertificate(
	rootCAs *x509.CertPool, rawCerts [][]byte, verifyServerName string) ([][]*x509.Certificate, error) {

	// This duplicates the verification logic in utls (and standard crypto/tls).

	certs := make([]*x509.Certificate, len(rawCerts))
	for i, rawCert := range rawCerts {
		cert, err := x509.ParseCertificate(rawCert)
		if err != nil {
			return nil, errors.Trace(err)
		}
		certs[i] = cert
	}

	opts := x509.VerifyOptions{
		Roots:         rootCAs,
		DNSName:       verifyServerName,
		Intermediates: x509.NewCertPool(),
	}

	for i, cert := range certs {
		if i == 0 {
			continue
		}
		opts.Intermediates.AddCert(cert)
	}

	verifiedChains, err := certs[0].Verify(opts)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return verifiedChains, nil
}

func verifyCertificatePins(pins []string, verifiedChains [][]*x509.Certificate) error {
	for _, chain := range verifiedChains {
		for _, cert := range chain {
			publicKeyDigest := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
			expectedPin := base64.StdEncoding.EncodeToString(publicKeyDigest[:])
			if common.Contains(pins, expectedPin) {
				// Return success on the first match of any certificate public key to any
				// pin.
				return nil
			}
		}
	}
	return errors.TraceNew("no pin found")
}

func IsTLSConnUsingHTTP2(conn net.Conn) bool {
	if c, ok := conn.(*utls.UConn); ok {
		state := c.ConnectionState()
		return state.NegotiatedProtocolIsMutual &&
			state.NegotiatedProtocol == "h2"
	}
	return false
}

// SelectTLSProfile picks a TLS profile at random from the available candidates.
func SelectTLSProfile(
	requireTLS12SessionTickets bool,
	isFronted bool,
	frontingProviderID string,
	p parameters.ParametersAccessor) string {

	// Two TLS profile lists are constructed, subject to limit constraints:
	// stock, fixed parrots (non-randomized SupportedTLSProfiles) and custom
	// parrots (CustomTLSProfileNames); and randomized. If one list is empty, the
	// non-empty list is used. Otherwise SelectRandomizedTLSProfileProbability
	// determines which list is used.
	//
	// Note that LimitTLSProfiles is not applied to CustomTLSProfiles; the
	// presence of a candidate in CustomTLSProfiles is treated as explicit
	// enabling.
	//
	// UseOnlyCustomTLSProfiles may be used to disable all stock TLS profiles and
	// use only CustomTLSProfiles; UseOnlyCustomTLSProfiles is ignored if
	// CustomTLSProfiles is empty.
	//
	// For fronted servers, DisableFrontingProviderTLSProfiles may be used
	// to disable TLS profiles which are incompatible with the TLS stack used
	// by the front. For example, if a utls parrot doesn't fully support all
	// of the capabilities in the ClientHello. Unlike the LimitTLSProfiles case,
	// DisableFrontingProviderTLSProfiles may disable CustomTLSProfiles.

	limitTLSProfiles := p.TLSProfiles(parameters.LimitTLSProfiles)
	var disableTLSProfiles protocol.TLSProfiles

	if isFronted && frontingProviderID != "" {
		disableTLSProfiles = p.LabeledTLSProfiles(
			parameters.DisableFrontingProviderTLSProfiles, frontingProviderID)
	}

	randomizedTLSProfiles := make([]string, 0)
	parrotTLSProfiles := make([]string, 0)

	for _, tlsProfile := range p.CustomTLSProfileNames() {
		if !common.Contains(disableTLSProfiles, tlsProfile) {
			parrotTLSProfiles = append(parrotTLSProfiles, tlsProfile)
		}
	}

	useOnlyCustomTLSProfiles := p.Bool(parameters.UseOnlyCustomTLSProfiles)
	if useOnlyCustomTLSProfiles && len(parrotTLSProfiles) == 0 {
		useOnlyCustomTLSProfiles = false
	}

	if !useOnlyCustomTLSProfiles {
		for _, tlsProfile := range protocol.SupportedTLSProfiles {

			if len(limitTLSProfiles) > 0 &&
				!common.Contains(limitTLSProfiles, tlsProfile) {
				continue
			}

			if common.Contains(disableTLSProfiles, tlsProfile) {
				continue
			}

			// requireTLS12SessionTickets is specified for
			// UNFRONTED-MEEK-SESSION-TICKET-OSSH, a protocol which depends on using
			// obfuscated session tickets to ensure that the server doesn't send its
			// certificate in the TLS handshake. TLS 1.2 profiles which omit session
			// tickets should not be selected. As TLS 1.3 encrypts the server
			// certificate message, there's no exclusion for TLS 1.3.

			if requireTLS12SessionTickets &&
				protocol.TLS12ProfileOmitsSessionTickets(tlsProfile) {
				continue
			}

			if protocol.TLSProfileIsRandomized(tlsProfile) {
				randomizedTLSProfiles = append(randomizedTLSProfiles, tlsProfile)
			} else {
				parrotTLSProfiles = append(parrotTLSProfiles, tlsProfile)
			}
		}
	}

	if len(randomizedTLSProfiles) > 0 &&
		(len(parrotTLSProfiles) == 0 ||
			p.WeightedCoinFlip(parameters.SelectRandomizedTLSProfileProbability)) {

		return randomizedTLSProfiles[prng.Intn(len(randomizedTLSProfiles))]
	}

	if len(parrotTLSProfiles) == 0 {
		return ""
	}

	return parrotTLSProfiles[prng.Intn(len(parrotTLSProfiles))]
}

func getUTLSClientHelloID(
	p parameters.ParametersAccessor,
	tlsProfile string) (utls.ClientHelloID, *utls.ClientHelloSpec, error) {

	switch tlsProfile {
	case protocol.TLS_PROFILE_IOS_111:
		return utls.HelloIOS_11_1, nil, nil
	case protocol.TLS_PROFILE_IOS_121:
		return utls.HelloIOS_12_1, nil, nil
	case protocol.TLS_PROFILE_CHROME_58:
		return utls.HelloChrome_58, nil, nil
	case protocol.TLS_PROFILE_CHROME_62:
		return utls.HelloChrome_62, nil, nil
	case protocol.TLS_PROFILE_CHROME_70:
		return utls.HelloChrome_70, nil, nil
	case protocol.TLS_PROFILE_CHROME_72:
		return utls.HelloChrome_72, nil, nil
	case protocol.TLS_PROFILE_CHROME_83:
		return utls.HelloChrome_83, nil, nil
	case protocol.TLS_PROFILE_FIREFOX_55:
		return utls.HelloFirefox_55, nil, nil
	case protocol.TLS_PROFILE_FIREFOX_56:
		return utls.HelloFirefox_56, nil, nil
	case protocol.TLS_PROFILE_FIREFOX_65:
		return utls.HelloFirefox_65, nil, nil
	case protocol.TLS_PROFILE_RANDOMIZED:
		return utls.HelloRandomized, nil, nil
	}

	// utls.HelloCustom with a utls.ClientHelloSpec is used for
	// CustomTLSProfiles.

	customTLSProfile := p.CustomTLSProfile(tlsProfile)
	if customTLSProfile == nil {
		return utls.HelloCustom,
			nil,
			errors.Tracef("unknown TLS profile: %s", tlsProfile)
	}

	utlsClientHelloSpec, err := customTLSProfile.GetClientHelloSpec()
	if err != nil {
		return utls.ClientHelloID{}, nil, errors.Trace(err)
	}

	return utls.HelloCustom, utlsClientHelloSpec, nil
}

func getClientHelloVersion(
	utlsClientHelloID utls.ClientHelloID,
	utlsClientHelloSpec *utls.ClientHelloSpec) (string, error) {

	switch utlsClientHelloID {

	case utls.HelloIOS_11_1, utls.HelloIOS_12_1, utls.HelloChrome_58,
		utls.HelloChrome_62, utls.HelloFirefox_55, utls.HelloFirefox_56:
		return protocol.TLS_VERSION_12, nil

	case utls.HelloChrome_70, utls.HelloChrome_72, utls.HelloChrome_83,
		utls.HelloFirefox_65, utls.HelloGolang:
		return protocol.TLS_VERSION_13, nil
	}

	// As utls.HelloRandomized/Custom may be either TLS 1.2 or TLS 1.3, we cannot
	// perform a simple ClientHello ID check. BuildHandshakeState is run, which
	// constructs the entire ClientHello.
	//
	// Assumes utlsClientHelloID.Seed has been set; otherwise the result is
	// ephemeral.
	//
	// BenchmarkRandomizedGetClientHelloVersion indicates that this operation
	// takes on the order of 0.05ms and allocates ~8KB for randomized client
	// hellos.

	conn := utls.UClient(
		nil,
		&utls.Config{InsecureSkipVerify: true},
		utlsClientHelloID)

	if utlsClientHelloSpec != nil {
		err := conn.ApplyPreset(utlsClientHelloSpec)
		if err != nil {
			return "", errors.Trace(err)
		}
	}

	err := conn.BuildHandshakeState()
	if err != nil {
		return "", errors.Trace(err)
	}

	for _, v := range conn.HandshakeState.Hello.SupportedVersions {
		if v == utls.VersionTLS13 {
			return protocol.TLS_VERSION_13, nil
		}
	}

	return protocol.TLS_VERSION_12, nil
}

func init() {
	// Favor compatibility over security. CustomTLSDial is used as an obfuscation
	// layer; users of CustomTLSDial, including meek and remote server list
	// downloads, don't depend on this TLS for its security properties.
	utls.EnableWeakCiphers()
}
