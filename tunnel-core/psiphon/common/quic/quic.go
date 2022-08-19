//go:build !PSIPHON_DISABLE_QUIC
// +build !PSIPHON_DISABLE_QUIC

/*
 * Copyright (c) 2018, Psiphon Inc.
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

Package quic wraps github.com/lucas-clemente/quic-go with net.Listener and
net.Conn types that provide a drop-in replacement for net.TCPConn.

Each QUIC session has exactly one stream, which is the equivilent of a TCP
stream.

Conns returned from Accept will have an established QUIC session and are
configured to perform a deferred AcceptStream on the first Read or Write.

Conns returned from Dial have an established QUIC session and stream. Dial
accepts a Context input which may be used to cancel the dial.

Conns mask or translate qerr.PeerGoingAway to io.EOF as appropriate.

QUIC idle timeouts and keep alives are tuned to mitigate aggressive UDP NAT
timeouts on mobile data networks while accounting for the fact that mobile
devices in standby/sleep may not be able to initiate the keep alive.

*/
package quic

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ooni/psiphon/tunnel-core/psiphon/common"
	"github.com/ooni/psiphon/tunnel-core/psiphon/common/errors"
	"github.com/ooni/psiphon/tunnel-core/psiphon/common/obfuscator"
	"github.com/ooni/psiphon/tunnel-core/psiphon/common/prng"
	"github.com/ooni/psiphon/tunnel-core/psiphon/common/protocol"
	"github.com/ooni/psiphon/tunnel-core/psiphon/common/values"
	ietf_quic "github.com/ooni/psiphon/tunnel-core/oovendor/quic-go"
	"github.com/ooni/psiphon/tunnel-core/oovendor/quic-go/http3"
)

const (
	SERVER_HANDSHAKE_TIMEOUT = 30 * time.Second
	SERVER_IDLE_TIMEOUT      = 5 * time.Minute
	CLIENT_IDLE_TIMEOUT      = 30 * time.Second
	UDP_PACKET_WRITE_TIMEOUT = 1 * time.Second
)

// Enabled indicates if QUIC functionality is enabled.
func Enabled() bool {
	return true
}

const ietfQUIC1VersionNumber = 0x1

var supportedVersionNumbers = map[string]uint32{
	protocol.QUIC_VERSION_GQUIC39:       uint32(0x51303339),
	protocol.QUIC_VERSION_GQUIC43:       uint32(0x51303433),
	protocol.QUIC_VERSION_GQUIC44:       uint32(0x51303434),
	protocol.QUIC_VERSION_OBFUSCATED:    uint32(0x51303433),
	protocol.QUIC_VERSION_V1:            ietfQUIC1VersionNumber,
	protocol.QUIC_VERSION_RANDOMIZED_V1: ietfQUIC1VersionNumber,
	protocol.QUIC_VERSION_OBFUSCATED_V1: uint32(ietfQUIC1VersionNumber),
	protocol.QUIC_VERSION_DECOY_V1:      uint32(ietfQUIC1VersionNumber),
}

func isObfuscated(quicVersion string) bool {
	return quicVersion == protocol.QUIC_VERSION_OBFUSCATED ||
		quicVersion == protocol.QUIC_VERSION_OBFUSCATED_V1 ||
		quicVersion == protocol.QUIC_VERSION_DECOY_V1
}

func isDecoy(quicVersion string) bool {
	return quicVersion == protocol.QUIC_VERSION_DECOY_V1
}

func isClientHelloRandomized(quicVersion string) bool {
	return quicVersion == protocol.QUIC_VERSION_RANDOMIZED_V1
}

func isIETF(quicVersion string) bool {
	versionNumber, ok := supportedVersionNumbers[quicVersion]
	if !ok {
		return false
	}
	return isIETFVersionNumber(versionNumber)
}

func isIETFVersionNumber(versionNumber uint32) bool {
	return versionNumber == ietfQUIC1VersionNumber
}

func isGQUIC(quicVersion string) bool {
	return quicVersion == protocol.QUIC_VERSION_GQUIC39 ||
		quicVersion == protocol.QUIC_VERSION_GQUIC43 ||
		quicVersion == protocol.QUIC_VERSION_GQUIC44 ||
		quicVersion == protocol.QUIC_VERSION_OBFUSCATED
}

func getALPN(versionNumber uint32) string {
	return "h3"
}

// quic_test overrides the server idle timeout.
var serverIdleTimeout = SERVER_IDLE_TIMEOUT

// Listener is a net.Listener.
type Listener struct {
	quicListener
	obfuscatedPacketConn *ObfuscatedPacketConn
	clientRandomHistory  *obfuscator.SeedHistory
}

// Listen creates a new Listener.
func Listen(
	logger common.Logger,
	irregularTunnelLogger func(string, error, common.LogFields),
	address string,
	obfuscationKey string,
	enableGQUIC bool) (net.Listener, error) {

	certificate, privateKey, err := common.GenerateWebServerCertificate(
		values.GetHostName())
	if err != nil {
		return nil, errors.Trace(err)
	}

	tlsCertificate, err := tls.X509KeyPair(
		[]byte(certificate), []byte(privateKey))
	if err != nil {
		return nil, errors.Trace(err)
	}

	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, errors.Trace(err)
	}

	udpConn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, errors.Trace(err)
	}

	seed, err := prng.NewSeed()
	if err != nil {
		udpConn.Close()
		return nil, errors.Trace(err)
	}

	obfuscatedPacketConn, err := NewObfuscatedPacketConn(
		udpConn, true, false, false, obfuscationKey, seed)
	if err != nil {
		udpConn.Close()
		return nil, errors.Trace(err)
	}

	// QUIC clients must prove knowledge of the obfuscated key via a message
	// sent in the TLS ClientHello random field, or receive no UDP packets
	// back from the server. This anti-probing mechanism is implemented using
	// the existing Passthrough message and SeedHistory replay detection
	// mechanisms. The replay history TTL is set to the validity period of
	// the passthrough message.
	//
	// Irregular events are logged for invalid client activity.

	clientRandomHistory := obfuscator.NewSeedHistory(
		&obfuscator.SeedHistoryConfig{SeedTTL: obfuscator.TLS_PASSTHROUGH_TIME_PERIOD})

	verifyClientHelloRandom := func(remoteAddr net.Addr, clientHelloRandom []byte) bool {

		ok := obfuscator.VerifyTLSPassthroughMessage(
			true, obfuscationKey, clientHelloRandom)
		if !ok {
			irregularTunnelLogger(
				common.IPAddressFromAddr(remoteAddr),
				errors.TraceNew("invalid client random message"),
				nil)
			return false
		}

		// Replay history is set to non-strict mode, allowing for a legitimate
		// client to resend its Initial packet, as may happen. Since the
		// source _port_ should be the same as the source IP in this case, we use
		// the full IP:port value as the client address from which a replay is
		// allowed.
		//
		// The non-strict case where ok is true and logFields is not nil is
		// ignored, and nothing is logged in that scenario.

		ok, logFields := clientRandomHistory.AddNew(
			false, remoteAddr.String(), "client-hello-random", clientHelloRandom)
		if !ok && logFields != nil {
			irregularTunnelLogger(
				common.IPAddressFromAddr(remoteAddr),
				errors.TraceNew("duplicate client random message"),
				*logFields)
		}

		return ok
	}

	var quicListener quicListener

	if !enableGQUIC {

		// When gQUIC is disabled, skip the muxListener entirely. This allows
		// quic-go to enable ECN operations as the packet conn is a
		// quic-goOOBCapablePacketConn; this provides some performance
		// optimizations and also generate packets that may be harder to
		// fingerprint, due to lack of ECN bits in IP packets otherwise.
		// Skipping muxListener also avoids the additional overhead of
		// pumping read packets though mux channels.

		tlsConfig, ietfQUICConfig := makeIETFConfig(
			obfuscatedPacketConn, verifyClientHelloRandom, tlsCertificate)

		listener, err := ietf_quic.Listen(
			obfuscatedPacketConn, tlsConfig, ietfQUICConfig)
		if err != nil {
			obfuscatedPacketConn.Close()
			return nil, errors.Trace(err)
		}

		quicListener = &ietfQUICListener{Listener: listener}

	} else {

		// Note that, due to nature of muxListener, full accepts may happen before
		// return and caller calls Accept.

		muxListener, err := newMuxListener(
			logger, verifyClientHelloRandom, obfuscatedPacketConn, tlsCertificate)
		if err != nil {
			obfuscatedPacketConn.Close()
			return nil, errors.Trace(err)
		}

		quicListener = muxListener
	}

	return &Listener{
		quicListener:         quicListener,
		obfuscatedPacketConn: obfuscatedPacketConn,
		clientRandomHistory:  clientRandomHistory,
	}, nil
}

func makeIETFConfig(
	conn *ObfuscatedPacketConn,
	verifyClientHelloRandom func(net.Addr, []byte) bool,
	tlsCertificate tls.Certificate) (*tls.Config, *ietf_quic.Config) {

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCertificate},
		NextProtos:   []string{getALPN(ietfQUIC1VersionNumber)},
	}

	ietfQUICConfig := &ietf_quic.Config{
		HandshakeIdleTimeout:  SERVER_HANDSHAKE_TIMEOUT,
		MaxIdleTimeout:        serverIdleTimeout,
		MaxIncomingStreams:    1,
		MaxIncomingUniStreams: -1,
		KeepAlive:             true,

		// The quic-go server may respond with a version negotiation packet
		// before reaching the Initial packet processing with its
		// anti-probing defense. This may happen even for a malformed packet.
		// To prevent all responses to probers, version negotiation is
		// disabled, which disables sending these packets. The fact that the
		// server does not issue version negotiation packets may be a
		// fingerprint itself, but, regardless, probers cannot ellicit any
		// reponse from the server without providing a well-formed Initial
		// packet with a valid Client Hello random value.
		//
		// Limitation: once version negotiate is required, the order of
		// quic-go operations may need to be changed in order to first check
		// the Initial/Client Hello, and then issue any required version
		// negotiation packet.
		DisableVersionNegotiationPackets: true,

		VerifyClientHelloRandom:       verifyClientHelloRandom,
		ServerMaxPacketSizeAdjustment: conn.serverMaxPacketSizeAdjustment,
	}

	return tlsConfig, ietfQUICConfig
}

// Accept returns a net.Conn that wraps a single QUIC session and stream. The
// stream establishment is deferred until the first Read or Write, allowing
// Accept to be called in a fast loop while goroutines spawned to handle each
// net.Conn will perform the blocking AcceptStream.
func (listener *Listener) Accept() (net.Conn, error) {

	session, err := listener.quicListener.Accept()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &Conn{
		session:              session,
		deferredAcceptStream: true,
	}, nil
}

func (listener *Listener) Close() error {

	// First close the underlying packet conn to ensure all quic-go goroutines
	// as well as any blocking Accept call goroutine is interrupted. Note
	// that muxListener does this as well, so this is for the IETF-only case.
	_ = listener.obfuscatedPacketConn.Close()

	return listener.quicListener.Close()
}

// Dial establishes a new QUIC session and stream to the server specified by
// address.
//
// packetConn is used as the underlying packet connection for QUIC. The dial
// may be cancelled by ctx; packetConn will be closed if the dial is
// cancelled or fails.
//
// Keep alive and idle timeout functionality in QUIC is disabled as these
// aspects are expected to be handled at a higher level.
func Dial(
	ctx context.Context,
	packetConn net.PacketConn,
	remoteAddr *net.UDPAddr,
	quicSNIAddress string,
	quicVersion string,
	clientHelloSeed *prng.Seed,
	obfuscationKey string,
	obfuscationPaddingSeed *prng.Seed,
	disablePathMTUDiscovery bool) (net.Conn, error) {

	if quicVersion == "" {
		return nil, errors.TraceNew("missing version")
	}

	if isClientHelloRandomized(quicVersion) && clientHelloSeed == nil {
		return nil, errors.TraceNew("missing client hello randomization values")
	}

	// obfuscationKey is always required, as it is used for anti-probing even
	// when not obfuscating the QUIC payload.
	if (isObfuscated(quicVersion) && obfuscationPaddingSeed == nil) || obfuscationKey == "" {
		return nil, errors.TraceNew("missing obfuscation values")
	}

	versionNumber, ok := supportedVersionNumbers[quicVersion]
	if !ok {
		return nil, errors.Tracef("unsupported version: %s", quicVersion)
	}

	// Fail if the destination port is invalid. Network operations should fail
	// quickly in this case, but IETF quic-go has been observed to timeout,
	// instead of failing quickly, in the case of invalid destination port 0.
	if remoteAddr.Port <= 0 || remoteAddr.Port >= 65536 {
		return nil, errors.Tracef("invalid destination port: %d", remoteAddr.Port)
	}

	udpConn, ok := packetConn.(udpConn)
	if !ok {
		return nil, errors.TraceNew("packetConn must implement net.UDPConn functions")
	}

	// Ensure blocked packet writes eventually timeout.
	packetConn = &writeTimeoutUDPConn{
		udpConn: udpConn,
	}

	maxPacketSizeAdjustment := 0

	if isObfuscated(quicVersion) {
		obfuscatedPacketConn, err := NewObfuscatedPacketConn(
			packetConn,
			false,
			isIETFVersionNumber(versionNumber),
			isDecoy(quicVersion),
			obfuscationKey,
			obfuscationPaddingSeed)
		if err != nil {
			return nil, errors.Trace(err)
		}
		packetConn = obfuscatedPacketConn

		// Reserve space for packet obfuscation overhead so that quic-go will
		// continue to produce packets of max size 1280.
		maxPacketSizeAdjustment = OBFUSCATED_MAX_PACKET_SIZE_ADJUSTMENT
	}

	// As an anti-probing measure, QUIC clients must prove knowledge of the
	// server obfuscation key in the first client packet sent to the server. In
	// the case of QUIC, the first packet, the Initial packet, contains a TLS
	// Client Hello, and we set the client random field to a value that both
	// proves knowledge of the obfuscation key and is indistiguishable from
	// random. This is the same "passthrough" technique used for TLS, although
	// for QUIC the server simply doesn't respond to any packets instead of
	// passing traffic through to a different server.
	//
	// Limitation: the legacy gQUIC implementation does not support this
	// anti-probling measure; gQUIC must be disabled to ensure no response
	// from the server.

	var getClientHelloRandom func() ([]byte, error)
	if obfuscationKey != "" {
		getClientHelloRandom = func() ([]byte, error) {
			random, err := obfuscator.MakeTLSPassthroughMessage(true, obfuscationKey)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return random, nil
		}
	}

	session, err := dialQUIC(
		ctx,
		packetConn,
		remoteAddr,
		quicSNIAddress,
		versionNumber,
		clientHelloSeed,
		getClientHelloRandom,
		maxPacketSizeAdjustment,
		disablePathMTUDiscovery,
		false)
	if err != nil {
		packetConn.Close()
		return nil, errors.Trace(err)
	}

	type dialResult struct {
		conn *Conn
		err  error
	}

	resultChannel := make(chan dialResult)

	go func() {

		stream, err := session.OpenStream()
		if err != nil {
			session.Close()
			resultChannel <- dialResult{err: err}
			return
		}

		resultChannel <- dialResult{
			conn: &Conn{
				packetConn: packetConn,
				session:    session,
				stream:     stream,
			},
		}
	}()

	var conn *Conn

	select {
	case result := <-resultChannel:
		conn, err = result.conn, result.err
	case <-ctx.Done():
		err = ctx.Err()
		// Interrupt the goroutine
		session.Close()
		<-resultChannel
	}

	if err != nil {
		packetConn.Close()
		return nil, errors.Trace(err)
	}

	return conn, nil
}

// udpConn matches net.UDPConn, which implements both net.Conn and
// net.PacketConn. udpConn enables handling of Dial packetConn inputs that
// are not concrete *net.UDPConn types but which still implement all the
// required functions. A udpConn instance can be passed to quic-go; various
// quic-go code paths check that the input conn implements net.Conn and/or
// net.PacketConn.
//
// TODO: add *AddrPort functions introduced in Go 1.18
type udpConn interface {
	Close() error
	File() (f *os.File, err error)
	LocalAddr() net.Addr
	Read(b []byte) (int, error)
	ReadFrom(b []byte) (int, net.Addr, error)
	ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error)
	ReadMsgUDP(b, oob []byte) (n, oobn, flags int, addr *net.UDPAddr, err error)
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	SetReadBuffer(bytes int) error
	SetReadDeadline(t time.Time) error
	SetWriteBuffer(bytes int) error
	SetWriteDeadline(t time.Time) error
	SyscallConn() (syscall.RawConn, error)
	Write(b []byte) (int, error)
	WriteMsgUDP(b, oob []byte, addr *net.UDPAddr) (n, oobn int, err error)
	WriteTo(b []byte, addr net.Addr) (int, error)
	WriteToUDP(b []byte, addr *net.UDPAddr) (int, error)
}

// writeTimeoutUDPConn sets write deadlines before each UDP packet write.
//
// Generally, a UDP packet write doesn't block. However, Go's
// internal/poll.FD.WriteMsg continues to loop when syscall.SendmsgN fails
// with EAGAIN, which indicates that an OS socket buffer is currently full;
// in certain OS states this may cause WriteMsgUDP/etc. to block
// indefinitely. In this scenario, we want to instead behave as if the packet
// were dropped, so we set a write deadline which will eventually interrupt
// any EAGAIN loop.
//
// Note that quic-go manages read deadlines; we set only the write deadline
// here.
type writeTimeoutUDPConn struct {
	udpConn
}

func (conn *writeTimeoutUDPConn) Write(b []byte) (int, error) {

	err := conn.SetWriteDeadline(time.Now().Add(UDP_PACKET_WRITE_TIMEOUT))
	if err != nil {
		return 0, errors.Trace(err)
	}

	// Do not wrap any I/O err returned by udpConn
	return conn.udpConn.Write(b)
}

func (conn *writeTimeoutUDPConn) WriteMsgUDP(b, oob []byte, addr *net.UDPAddr) (int, int, error) {

	err := conn.SetWriteDeadline(time.Now().Add(UDP_PACKET_WRITE_TIMEOUT))
	if err != nil {
		return 0, 0, errors.Trace(err)
	}

	// Do not wrap any I/O err returned by udpConn
	return conn.udpConn.WriteMsgUDP(b, oob, addr)
}

func (conn *writeTimeoutUDPConn) WriteTo(b []byte, addr net.Addr) (int, error) {

	err := conn.SetWriteDeadline(time.Now().Add(UDP_PACKET_WRITE_TIMEOUT))
	if err != nil {
		return 0, errors.Trace(err)
	}

	// Do not wrap any I/O err returned by udpConn
	return conn.udpConn.WriteTo(b, addr)
}

func (conn *writeTimeoutUDPConn) WriteToUDP(b []byte, addr *net.UDPAddr) (int, error) {

	err := conn.SetWriteDeadline(time.Now().Add(UDP_PACKET_WRITE_TIMEOUT))
	if err != nil {
		return 0, errors.Trace(err)
	}

	// Do not wrap any I/O err returned by udpConn
	return conn.udpConn.WriteToUDP(b, addr)
}

// Conn is a net.Conn and psiphon/common.Closer.
type Conn struct {
	packetConn net.PacketConn
	session    quicSession

	deferredAcceptStream bool

	acceptMutex sync.Mutex
	acceptErr   error
	stream      quicStream

	readMutex  sync.Mutex
	writeMutex sync.Mutex

	isClosed int32
}

func (conn *Conn) doDeferredAcceptStream() error {
	conn.acceptMutex.Lock()
	defer conn.acceptMutex.Unlock()

	if conn.stream != nil {
		return nil
	}

	if conn.acceptErr != nil {
		return conn.acceptErr
	}

	stream, err := conn.session.AcceptStream()
	if err != nil {
		conn.session.Close()
		conn.acceptErr = errors.Trace(err)
		return conn.acceptErr
	}

	conn.stream = stream

	return nil
}

func (conn *Conn) Read(b []byte) (int, error) {

	if conn.deferredAcceptStream {
		err := conn.doDeferredAcceptStream()
		if err != nil {
			return 0, errors.Trace(err)
		}
	}

	// Add mutex to provide full net.Conn concurrency semantics.
	// https://github.com/lucas-clemente/quic-go/blob/9cc23135d0477baf83aa4715de39ae7070039cb2/stream.go#L64
	// "Read() and Write() may be called concurrently, but multiple calls to
	// "Read() or Write() individually must be synchronized manually."
	conn.readMutex.Lock()
	defer conn.readMutex.Unlock()

	n, err := conn.stream.Read(b)
	if conn.session.isErrorIndicatingClosed(err) {
		_ = conn.Close()
		err = io.EOF
	}
	return n, err
}

func (conn *Conn) Write(b []byte) (int, error) {

	if conn.deferredAcceptStream {
		err := conn.doDeferredAcceptStream()
		if err != nil {
			return 0, errors.Trace(err)
		}
	}

	conn.writeMutex.Lock()
	defer conn.writeMutex.Unlock()

	n, err := conn.stream.Write(b)
	if conn.session.isErrorIndicatingClosed(err) {
		_ = conn.Close()
		if n == len(b) {
			err = nil
		}
	}
	return n, err
}

func (conn *Conn) Close() error {
	err := conn.session.Close()
	if conn.packetConn != nil {
		err1 := conn.packetConn.Close()
		if err == nil {
			err = err1
		}
	}
	atomic.StoreInt32(&conn.isClosed, 1)
	return err
}

func (conn *Conn) IsClosed() bool {
	return atomic.LoadInt32(&conn.isClosed) == 1
}

func (conn *Conn) LocalAddr() net.Addr {
	return conn.session.LocalAddr()
}

func (conn *Conn) RemoteAddr() net.Addr {
	return conn.session.RemoteAddr()
}

func (conn *Conn) SetDeadline(t time.Time) error {

	if conn.deferredAcceptStream {
		err := conn.doDeferredAcceptStream()
		if err != nil {
			return errors.Trace(err)
		}
	}

	return conn.stream.SetDeadline(t)
}

func (conn *Conn) SetReadDeadline(t time.Time) error {

	if conn.deferredAcceptStream {
		err := conn.doDeferredAcceptStream()
		if err != nil {
			return errors.Trace(err)
		}
	}

	return conn.stream.SetReadDeadline(t)
}

func (conn *Conn) SetWriteDeadline(t time.Time) error {

	if conn.deferredAcceptStream {
		err := conn.doDeferredAcceptStream()
		if err != nil {
			return errors.Trace(err)
		}
	}

	return conn.stream.SetWriteDeadline(t)
}

// QUICTransporter implements the psiphon.transporter interface, used in
// psiphon.MeekConn for HTTP requests, which requires a RoundTripper and
// CloseIdleConnections.
type QUICTransporter struct {
	quicRoundTripper
	noticeEmitter           func(string)
	udpDialer               func(ctx context.Context) (net.PacketConn, *net.UDPAddr, error)
	quicSNIAddress          string
	quicVersion             string
	clientHelloSeed         *prng.Seed
	disablePathMTUDiscovery bool
	packetConn              atomic.Value

	mutex sync.Mutex
	ctx   context.Context
}

// NewQUICTransporter creates a new QUICTransporter.
func NewQUICTransporter(
	ctx context.Context,
	noticeEmitter func(string),
	udpDialer func(ctx context.Context) (net.PacketConn, *net.UDPAddr, error),
	quicSNIAddress string,
	quicVersion string,
	clientHelloSeed *prng.Seed,
	disablePathMTUDiscovery bool) (*QUICTransporter, error) {

	if quicVersion == "" {
		return nil, errors.TraceNew("missing version")
	}

	versionNumber, ok := supportedVersionNumbers[quicVersion]
	if !ok {
		return nil, errors.Tracef("unsupported version: %s", quicVersion)
	}

	if isClientHelloRandomized(quicVersion) && clientHelloSeed == nil {
		return nil, errors.TraceNew("missing client hello randomization values")
	}

	t := &QUICTransporter{
		noticeEmitter:           noticeEmitter,
		udpDialer:               udpDialer,
		quicSNIAddress:          quicSNIAddress,
		quicVersion:             quicVersion,
		clientHelloSeed:         clientHelloSeed,
		disablePathMTUDiscovery: disablePathMTUDiscovery,
		ctx:                     ctx,
	}

	if isIETFVersionNumber(versionNumber) {
		t.quicRoundTripper = &http3.RoundTripper{Dial: t.dialIETFQUIC}
	} else {
		var err error
		t.quicRoundTripper, err = gQUICRoundTripper(t)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	return t, nil
}

func (t *QUICTransporter) SetRequestContext(ctx context.Context) {
	// Note: can't use sync.Value since underlying type of ctx changes.
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.ctx = ctx
}

// CloseIdleConnections wraps QUIC RoundTripper.Close, which provides the
// necessary functionality for psiphon.transporter as used by
// psiphon.MeekConn. Note that, unlike http.Transport.CloseIdleConnections,
// the connections are closed regardless of idle status.
func (t *QUICTransporter) CloseIdleConnections() {

	// This operation doesn't prevent a concurrent http3.client.dial from
	// establishing a new packet conn; we also rely on the request context to
	// fully interrupt and stop a http3.RoundTripper.

	t.closePacketConn()
	t.quicRoundTripper.Close()
}

func (t *QUICTransporter) closePacketConn() {
	packetConn := t.packetConn.Load()
	if p, ok := packetConn.(net.PacketConn); ok {
		p.Close()
	}
}

func (t *QUICTransporter) dialIETFQUIC(
	_, _ string, _ *tls.Config, _ *ietf_quic.Config) (ietf_quic.EarlySession, error) {
	session, err := t.dialQUIC()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return session.(*ietfQUICSession).Session.(ietf_quic.EarlySession), nil
}

func (t *QUICTransporter) dialQUIC() (retSession quicSession, retErr error) {

	defer func() {
		if retErr != nil && t.noticeEmitter != nil {
			t.noticeEmitter(fmt.Sprintf("QUICTransporter.dialQUIC failed: %s", retErr))
		}
	}()

	versionNumber, ok := supportedVersionNumbers[t.quicVersion]
	if !ok {
		return nil, errors.Tracef("unsupported version: %s", t.quicVersion)
	}

	t.mutex.Lock()
	ctx := t.ctx
	t.mutex.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}

	packetConn, remoteAddr, err := t.udpDialer(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	session, err := dialQUIC(
		ctx,
		packetConn,
		remoteAddr,
		t.quicSNIAddress,
		versionNumber,
		t.clientHelloSeed,
		nil,
		0,
		t.disablePathMTUDiscovery,
		true)
	if err != nil {
		packetConn.Close()
		return nil, errors.Trace(err)
	}

	// dialQUIC uses quic-go.DialContext as we must create our own UDP sockets to
	// set properties such as BIND_TO_DEVICE. However, when DialContext is used,
	// quic-go does not take responsibiity for closing the underlying packetConn
	// when the QUIC session is closed.
	//
	// We track the most recent packetConn in QUICTransporter and close it:
	// - when CloseIdleConnections is called, as it is by psiphon.MeekConn when
	//   it is closing;
	// - here in dialFunc, with the assumption that only one concurrent QUIC
	//   session is used per h2quic.RoundTripper.
	//
	// This code also assume no concurrent calls to dialFunc, as otherwise a race
	// condition exists between closePacketConn and Store.

	t.closePacketConn()
	t.packetConn.Store(packetConn)

	return session, nil
}

// The following code provides support for using both gQUIC and IETF QUIC,
// which are implemented in two different branches (now forks) of quic-go.
// (The gQUIC functions are now located in gquic.go and the entire gQUIC
// quic-go stack may be conditionally excluded from builds).
//
// dialQUIC uses the appropriate quic-go and returns quicSession which wraps
// either a ietf_quic.Session or gquic.Session.
//
// muxPacketConn provides a multiplexing listener that directs packets to
// either a ietf_quic.Listener or a gquic.Listener based on the content of the
// packet.

type quicListener interface {
	Close() error
	Addr() net.Addr
	Accept() (quicSession, error)
}

type quicSession interface {
	io.Closer
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	AcceptStream() (quicStream, error)
	OpenStream() (quicStream, error)
	isErrorIndicatingClosed(err error) bool
}

type quicStream interface {
	io.Reader
	io.Writer
	io.Closer
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	SetDeadline(t time.Time) error
}

type quicRoundTripper interface {
	http.RoundTripper
	Close() error
}

type ietfQUICListener struct {
	ietf_quic.Listener
}

func (l *ietfQUICListener) Accept() (quicSession, error) {
	// A specific context is not provided since the interface needs to match the
	// gquic-go API, which lacks context support.
	session, err := l.Listener.Accept(context.Background())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ietfQUICSession{Session: session}, nil
}

type ietfQUICSession struct {
	ietf_quic.Session
}

func (s *ietfQUICSession) AcceptStream() (quicStream, error) {
	// A specific context is not provided since the interface needs to match the
	// gquic-go API, which lacks context support.
	//
	// TODO: once gQUIC support is retired, this context may be used in place
	// of the deferredAcceptStream mechanism.
	stream, err := s.Session.AcceptStream(context.Background())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return stream, nil
}

func (s *ietfQUICSession) OpenStream() (quicStream, error) {
	return s.Session.OpenStream()
}

func (s *ietfQUICSession) Close() error {
	return s.Session.CloseWithError(0, "")
}

func (s *ietfQUICSession) isErrorIndicatingClosed(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// The target errors are of type qerr.ApplicationError and
	// qerr.IdleTimeoutError, but these are not exported by quic-go.
	return errStr == "Application error 0x0" ||
		errStr == "timeout: no recent network activity"
}

func dialQUIC(
	ctx context.Context,
	packetConn net.PacketConn,
	remoteAddr *net.UDPAddr,
	quicSNIAddress string,
	versionNumber uint32,
	clientHelloSeed *prng.Seed,
	getClientHelloRandom func() ([]byte, error),
	clientMaxPacketSizeAdjustment int,
	disablePathMTUDiscovery bool,
	dialEarly bool) (quicSession, error) {

	if isIETFVersionNumber(versionNumber) {
		quicConfig := &ietf_quic.Config{
			HandshakeIdleTimeout: time.Duration(1<<63 - 1),
			MaxIdleTimeout:       CLIENT_IDLE_TIMEOUT,
			KeepAlive:            true,
			Versions: []ietf_quic.VersionNumber{
				ietf_quic.VersionNumber(versionNumber)},
			ClientHelloSeed:               clientHelloSeed,
			GetClientHelloRandom:          getClientHelloRandom,
			ClientMaxPacketSizeAdjustment: clientMaxPacketSizeAdjustment,
			DisablePathMTUDiscovery:       disablePathMTUDiscovery,
		}

		deadline, ok := ctx.Deadline()
		if ok {
			quicConfig.HandshakeIdleTimeout = time.Until(deadline)
		}

		var dialSession ietf_quic.Session
		var err error
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{getALPN(versionNumber)},
		}

		if dialEarly {
			dialSession, err = ietf_quic.DialEarlyContext(
				ctx,
				packetConn,
				remoteAddr,
				quicSNIAddress,
				tlsConfig,
				quicConfig)
		} else {
			dialSession, err = ietf_quic.DialContext(
				ctx,
				packetConn,
				remoteAddr,
				quicSNIAddress,
				tlsConfig,
				quicConfig)
		}
		if err != nil {
			return nil, errors.Trace(err)
		}

		return &ietfQUICSession{Session: dialSession}, nil

	} else {

		quicSession, err := gQUICDialContext(
			ctx,
			packetConn,
			remoteAddr,
			quicSNIAddress,
			versionNumber)
		if err != nil {
			return nil, errors.Trace(err)
		}

		return quicSession, nil
	}
}

const (
	muxPacketQueueSize  = 128
	muxPacketBufferSize = 1452 // quic-go.MaxReceivePacketSize
)

type packet struct {
	addr net.Addr
	size int
	data []byte
}

// muxPacketConn delivers packets to a specific quic-go listener.
type muxPacketConn struct {
	localAddr net.Addr
	listener  *muxListener
	packets   chan *packet
}

func newMuxPacketConn(localAddr net.Addr, listener *muxListener) *muxPacketConn {
	return &muxPacketConn{
		localAddr: localAddr,
		listener:  listener,
		packets:   make(chan *packet, muxPacketQueueSize),
	}
}

func (conn *muxPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {

	select {
	case p := <-conn.packets:

		// If b is too short, the packet is truncated. This won't happen as long as
		// muxPacketBufferSize matches quic-go.MaxReceivePacketSize.
		copy(b, p.data[0:p.size])
		n := p.size
		addr := p.addr

		// Clear and replace packet buffer.
		p.size = 0
		conn.listener.packets <- p

		return n, addr, nil
	case <-conn.listener.stopBroadcast:
		return 0, nil, io.EOF
	}
}

func (conn *muxPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return conn.listener.conn.WriteTo(b, addr)
}

func (conn *muxPacketConn) Close() error {
	// This Close won't unblock Read/Write operations or propagate the Close
	// signal up to muxListener.  The correct way to shutdown is to call
	// muxListener.Close.
	return nil
}

func (conn *muxPacketConn) LocalAddr() net.Addr {
	return conn.localAddr
}

func (conn *muxPacketConn) SetDeadline(t time.Time) error {
	return errors.TraceNew("not supported")
}

func (conn *muxPacketConn) SetReadDeadline(t time.Time) error {
	return errors.TraceNew("not supported")
}

func (conn *muxPacketConn) SetWriteDeadline(t time.Time) error {
	return errors.TraceNew("not supported")
}

// SetReadBuffer and SyscallConn provide passthroughs to the underlying
// net.UDPConn implementations, used to optimize the UDP receive buffer size.
// See https://github.com/lucas-clemente/quic-go/wiki/UDP-Receive-Buffer-Size
// and ietf_quic.setReceiveBuffer. Only the IETF stack will access these
// functions.
//
// Limitation: due to the relayPackets/ReadFrom scheme, this simple
// passthrough does not suffice to provide access to ReadMsgUDP for
// https://godoc.org/github.com/lucas-clemente/quic-go#ECNCapablePacketConn.

func (conn *muxPacketConn) SetReadBuffer(bytes int) error {
	c, ok := conn.listener.conn.OOBCapablePacketConn.(interface {
		SetReadBuffer(int) error
	})
	if !ok {
		return errors.TraceNew("not supported")
	}
	return c.SetReadBuffer(bytes)
}

func (conn *muxPacketConn) SyscallConn() (syscall.RawConn, error) {
	c, ok := conn.listener.conn.OOBCapablePacketConn.(interface {
		SyscallConn() (syscall.RawConn, error)
	})
	if !ok {
		return nil, errors.TraceNew("not supported")
	}
	return c.SyscallConn()
}

// muxListener is a multiplexing packet conn listener which relays packets to
// multiple quic-go listeners.
type muxListener struct {
	logger           common.Logger
	isClosed         int32
	runWaitGroup     *sync.WaitGroup
	stopBroadcast    chan struct{}
	conn             *ObfuscatedPacketConn
	packets          chan *packet
	acceptedSessions chan quicSession
	ietfQUICConn     *muxPacketConn
	ietfQUICListener quicListener
	gQUICConn        *muxPacketConn
	gQUICListener    quicListener
}

func newMuxListener(
	logger common.Logger,
	verifyClientHelloRandom func(net.Addr, []byte) bool,
	conn *ObfuscatedPacketConn,
	tlsCertificate tls.Certificate) (*muxListener, error) {

	listener := &muxListener{
		logger:           logger,
		runWaitGroup:     new(sync.WaitGroup),
		stopBroadcast:    make(chan struct{}),
		conn:             conn,
		packets:          make(chan *packet, muxPacketQueueSize),
		acceptedSessions: make(chan quicSession, 2), // 1 per listener
	}

	// All packet relay buffers are allocated in advance.
	for i := 0; i < muxPacketQueueSize; i++ {
		listener.packets <- &packet{data: make([]byte, muxPacketBufferSize)}
	}

	listener.ietfQUICConn = newMuxPacketConn(conn.LocalAddr(), listener)

	tlsConfig, ietfQUICConfig := makeIETFConfig(
		conn, verifyClientHelloRandom, tlsCertificate)

	il, err := ietf_quic.Listen(listener.ietfQUICConn, tlsConfig, ietfQUICConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	listener.ietfQUICListener = &ietfQUICListener{Listener: il}

	listener.gQUICConn = newMuxPacketConn(conn.LocalAddr(), listener)

	gl, err := gQUICListen(listener.gQUICConn, tlsCertificate, serverIdleTimeout)
	if err != nil {
		listener.ietfQUICListener.Close()
		return nil, errors.Trace(err)
	}
	listener.gQUICListener = gl

	listener.runWaitGroup.Add(3)
	go listener.relayPackets()
	go listener.relayAcceptedSessions(listener.gQUICListener)
	go listener.relayAcceptedSessions(listener.ietfQUICListener)

	return listener, nil
}

func (listener *muxListener) relayPackets() {
	defer listener.runWaitGroup.Done()

	for {

		var p *packet
		select {
		case p = <-listener.packets:
		case <-listener.stopBroadcast:
			return
		}

		// Read network packets. The DPI functionality of the obfuscation layer
		// identifies the type of QUIC, gQUIC or IETF, in addition to identifying
		// and processing obfuscation. This type information determines which
		// quic-go receives the packet.
		//
		// Network errors are not relayed to quic-go, as it will shut down the
		// server on any error returned from ReadFrom, even net.Error.Temporary()
		// errors.

		var isIETF bool
		var err error
		p.size, _, _, p.addr, isIETF, err = listener.conn.readPacketWithType(p.data, nil)
		if err != nil {
			if listener.logger != nil {
				message := "readFromWithType failed"
				if e, ok := err.(net.Error); ok && e.Temporary() {
					listener.logger.WithTraceFields(
						common.LogFields{"error": err}).Debug(message)
				} else {
					listener.logger.WithTraceFields(
						common.LogFields{"error": err}).Warning(message)
				}
			}
			// TODO: propagate non-temporary errors to Accept?
			listener.packets <- p
			continue
		}

		// Send the packet to the correct quic-go. The packet is dropped if the
		// target quic-go packet queue is full.

		if isIETF {
			select {
			case listener.ietfQUICConn.packets <- p:
			default:
				listener.packets <- p
			}
		} else {
			select {
			case listener.gQUICConn.packets <- p:
			default:
				listener.packets <- p
			}
		}
	}
}

func (listener *muxListener) relayAcceptedSessions(l quicListener) {
	defer listener.runWaitGroup.Done()
	for {
		session, err := l.Accept()
		if err != nil {
			if listener.logger != nil {
				message := "Accept failed"
				if e, ok := err.(net.Error); ok && e.Temporary() {
					listener.logger.WithTraceFields(
						common.LogFields{"error": err}).Debug(message)
				} else {
					listener.logger.WithTraceFields(
						common.LogFields{"error": err}).Warning(message)
				}
			}
			// TODO: propagate non-temporary errors to Accept?
			select {
			case <-listener.stopBroadcast:
				return
			default:
			}
			continue
		}
		select {
		case listener.acceptedSessions <- session:
		case <-listener.stopBroadcast:
			return
		}
	}
}

func (listener *muxListener) Accept() (quicSession, error) {
	select {
	case conn := <-listener.acceptedSessions:
		return conn, nil
	case <-listener.stopBroadcast:
		return nil, errors.TraceNew("closed")
	}
}

func (listener *muxListener) Close() error {

	// Ensure close channel only called once.
	if !atomic.CompareAndSwapInt32(&listener.isClosed, 0, 1) {
		return nil
	}

	close(listener.stopBroadcast)

	var retErr error

	err := listener.gQUICListener.Close()
	if err != nil && retErr == nil {
		retErr = errors.Trace(err)
	}

	err = listener.ietfQUICListener.Close()
	if err != nil && retErr == nil {
		retErr = errors.Trace(err)
	}

	err = listener.conn.Close()
	if err != nil && retErr == nil {
		retErr = errors.Trace(err)
	}

	listener.runWaitGroup.Wait()

	return retErr
}

func (listener *muxListener) Addr() net.Addr {
	return listener.conn.LocalAddr()
}
