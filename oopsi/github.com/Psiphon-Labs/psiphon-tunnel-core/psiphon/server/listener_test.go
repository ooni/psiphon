/*
 * Copyright (c) 2020, Psiphon Inc.
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
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/ooni/psiphon/oopsi/github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common/fragmentor"
	"github.com/ooni/psiphon/oopsi/github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common/protocol"
	"github.com/ooni/psiphon/oopsi/github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common/tactics"
)

func TestListener(t *testing.T) {

	tunnelProtocol := protocol.TUNNEL_PROTOCOL_OBFUSCATED_SSH

	tacticsConfigJSONFormat := `
    {
      "RequestPublicKey" : "%s",
      "RequestPrivateKey" : "%s",
      "RequestObfuscatedKey" : "%s",
      "DefaultTactics" : {
        "TTL" : "60s",
        "Probability" : 1.0
      },
      "FilteredTactics" : [
        {
          "Filter" : {
            "Regions": ["R1"],
            "ISPs": ["I1"],
            "Cities": ["C1"]
          },
          "Tactics" : {
            "Parameters" : {
              "LimitTunnelProtocols" : ["%s"],
              "FragmentorDownstreamLimitProtocols" : ["%s"],
              "FragmentorDownstreamProbability" : 1.0,
              "FragmentorDownstreamMinTotalBytes" : 1,
              "FragmentorDownstreamMaxTotalBytes" : 1,
              "FragmentorDownstreamMinWriteBytes" : 1,
              "FragmentorDownstreamMaxWriteBytes" : 1,
              "FragmentorDownstreamLimitProtocols" : ["OSSH"]
            }
          }
        }
      ]
    }
    `

	tacticsRequestPublicKey, tacticsRequestPrivateKey, tacticsRequestObfuscatedKey, err :=
		tactics.GenerateKeys()
	if err != nil {
		t.Fatalf("error generating tactics keys: %s", err)
	}

	tacticsConfigJSON := fmt.Sprintf(
		tacticsConfigJSONFormat,
		tacticsRequestPublicKey, tacticsRequestPrivateKey, tacticsRequestObfuscatedKey,
		tunnelProtocol, tunnelProtocol)

	tacticsConfigFilename := filepath.Join(testDataDirName, "tactics_config.json")

	err = ioutil.WriteFile(tacticsConfigFilename, []byte(tacticsConfigJSON), 0600)
	if err != nil {
		t.Fatalf("error paving tactics config file: %s", err)
	}

	tacticsServer, err := tactics.NewServer(
		nil,
		nil,
		nil,
		tacticsConfigFilename)
	if err != nil {
		t.Fatalf("NewServer failed: %s", err)
	}

	listenerFragmentedGeoIP := func(string) GeoIPData {
		return GeoIPData{Country: "R1", ISP: "I1", City: "C1"}
	}
	listenerUnfragmentedGeoIPWrongRegion := func(string) GeoIPData {
		return GeoIPData{Country: "R2", ISP: "I1", City: "C1"}
	}
	listenerUnfragmentedGeoIPWrongISP := func(string) GeoIPData {
		return GeoIPData{Country: "R1", ISP: "I2", City: "C1"}
	}
	listenerUnfragmentedGeoIPWrongCity := func(string) GeoIPData {
		return GeoIPData{Country: "R1", ISP: "I1", City: "C2"}
	}

	listenerTestCases := []struct {
		description      string
		geoIPLookup      func(string) GeoIPData
		expectFragmentor bool
	}{
		{
			"fragmented",
			listenerFragmentedGeoIP,
			true,
		},
		{
			"unfragmented-region",
			listenerUnfragmentedGeoIPWrongRegion,
			false,
		},
		{
			"unfragmented-ISP",
			listenerUnfragmentedGeoIPWrongISP,
			false,
		},
		{
			"unfragmented-city",
			listenerUnfragmentedGeoIPWrongCity,
			false,
		},
	}

	for _, testCase := range listenerTestCases {
		t.Run(testCase.description, func(t *testing.T) {

			tcpListener, err := net.Listen("tcp", ":0")
			if err != nil {
				t.Fatalf(" net.Listen failed: %s", err)
			}

			support := &SupportServices{
				TacticsServer: tacticsServer,
			}
			support.ReplayCache = NewReplayCache(support)
			support.ServerTacticsParametersCache =
				NewServerTacticsParametersCache(support)

			tacticsListener := NewTacticsListener(
				support,
				tcpListener,
				tunnelProtocol,
				testCase.geoIPLookup)

			clientConn, err := net.Dial("tcp", tacticsListener.Addr().String())
			if err != nil {
				t.Fatalf(" net.Dial failed: %s", err)
				return
			}

			result := make(chan net.Conn, 1)

			go func() {
				serverConn, err := tacticsListener.Accept()
				if err == nil {
					result <- serverConn
				}
			}()

			timer := time.NewTimer(3 * time.Second)
			defer timer.Stop()

			select {
			case serverConn := <-result:
				_, isFragmentor := serverConn.(*fragmentor.Conn)
				if testCase.expectFragmentor && !isFragmentor {
					t.Fatalf("unexpected non-fragmentor: %T", serverConn)
				} else if !testCase.expectFragmentor && isFragmentor {
					t.Fatalf("unexpected fragmentor:  %T", serverConn)
				}
				serverConn.Close()
			case <-timer.C:
				t.Fatalf("timeout before expected accepted connection")
			}

			clientConn.Close()
			tacticsListener.Close()
		})
	}
}
