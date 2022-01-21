//go:build !PSIPHON_DISABLE_QUIC && !PSIPHON_DISABLE_GQUIC
// +build !PSIPHON_DISABLE_QUIC,!PSIPHON_DISABLE_GQUIC

/*
 * Copyright (c) 2021, Psiphon Inc.
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

package quic

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/ooni/psiphon/tunnel-core/psiphon/common/errors"
	"github.com/ooni/psiphon/tunnel-core/psiphon/common/quic/gquic-go"
	"github.com/ooni/psiphon/tunnel-core/psiphon/common/quic/gquic-go/h2quic"
	"github.com/ooni/psiphon/tunnel-core/psiphon/common/quic/gquic-go/qerr"
)

func GQUICEnabled() bool {
	return true
}

type gQUICListener struct {
	gquic.Listener
}

func (l *gQUICListener) Accept() (quicSession, error) {
	session, err := l.Listener.Accept()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &gQUICSession{Session: session}, nil
}

func gQUICListen(
	conn net.PacketConn,
	tlsCertificate tls.Certificate,
	serverIdleTimeout time.Duration) (quicListener, error) {

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCertificate},
	}

	gQUICConfig := &gquic.Config{
		HandshakeTimeout:      SERVER_HANDSHAKE_TIMEOUT,
		IdleTimeout:           serverIdleTimeout,
		MaxIncomingStreams:    1,
		MaxIncomingUniStreams: -1,
		KeepAlive:             true,
	}

	gl, err := gquic.Listen(conn, tlsConfig, gQUICConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &gQUICListener{Listener: gl}, nil
}

type gQUICSession struct {
	gquic.Session
}

func (s *gQUICSession) AcceptStream() (quicStream, error) {
	stream, err := s.Session.AcceptStream()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return stream, nil
}

func (s *gQUICSession) OpenStream() (quicStream, error) {
	return s.Session.OpenStream()
}

func (s *gQUICSession) isErrorIndicatingClosed(err error) bool {
	if err == nil {
		return false
	}
	if quicErr, ok := err.(*qerr.QuicError); ok {
		switch quicErr.ErrorCode {
		case qerr.PeerGoingAway, qerr.NetworkIdleTimeout:
			return true
		}
	}
	return false
}

func gQUICDialContext(
	ctx context.Context,
	packetConn net.PacketConn,
	remoteAddr *net.UDPAddr,
	quicSNIAddress string,
	versionNumber uint32) (quicSession, error) {

	quicConfig := &gquic.Config{
		HandshakeTimeout: time.Duration(1<<63 - 1),
		IdleTimeout:      CLIENT_IDLE_TIMEOUT,
		KeepAlive:        true,
		Versions: []gquic.VersionNumber{
			gquic.VersionNumber(versionNumber)},
	}

	deadline, ok := ctx.Deadline()
	if ok {
		quicConfig.HandshakeTimeout = time.Until(deadline)
	}

	dialSession, err := gquic.DialContext(
		ctx,
		packetConn,
		remoteAddr,
		quicSNIAddress,
		&tls.Config{
			InsecureSkipVerify: true,
		},
		quicConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &gQUICSession{Session: dialSession}, nil
}

func gQUICRoundTripper(t *QUICTransporter) (quicRoundTripper, error) {
	return &h2quic.RoundTripper{Dial: t.dialgQUIC}, nil
}

func (t *QUICTransporter) dialgQUIC(
	_, _ string, _ *tls.Config, _ *gquic.Config) (gquic.Session, error) {
	session, err := t.dialQUIC()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return session.(*gQUICSession).Session, nil
}
