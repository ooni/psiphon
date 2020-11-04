// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// TLS low level connection and record layer

package tls

import (
	"bytes"
	"crypto/cipher"
	"crypto/subtle"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// A Conn represents a secured connection.
// It implements the net.Conn interface.
type Conn struct {
	// constant
	conn     net.Conn
	isClient bool

	phase handshakeStatus // protected by in.Mutex
	// handshakeConfirmed is an atomic bool for phase == handshakeConfirmed
	handshakeConfirmed int32
	// confirmMutex is held by any read operation before handshakeConfirmed
	confirmMutex sync.Mutex

	// [Psiphon]
	// https://github.com/ooni/psiphon/oopsi/github.com/golang/go/commit/e5b13401c6b19f58a8439f1019a80fe540c0c687
	//
	// handshakeStatus is 1 if the connection is currently transferring
	// application data (i.e. is not currently processing a handshake).
	// This field is only to be accessed with sync/atomic.
	handshakeStatus uint32

	// constant after handshake; protected by handshakeMutex
	handshakeMutex sync.Mutex // handshakeMutex < in.Mutex, out.Mutex, errMutex
	handshakeErr   error      // error resulting from handshake
	connID         []byte     // Random connection id
	clientHello    []byte     // ClientHello packet contents
	vers           uint16     // TLS version
	haveVers       bool       // version has been negotiated
	config         *Config    // configuration passed to constructor
	// handshakes counts the number of handshakes performed on the
	// connection so far. If renegotiation is disabled then this is either
	// zero or one.
	handshakes       int
	didResume        bool // whether this connection was a session resumption
	cipherSuite      uint16
	ocspResponse     []byte   // stapled OCSP response
	scts             [][]byte // Signed certificate timestamps from server
	peerCertificates []*x509.Certificate
	// verifiedChains contains the certificate chains that we built, as
	// opposed to the ones presented by the server.
	verifiedChains [][]*x509.Certificate
	// verifiedDc is set by a client who negotiates the use of a valid delegated
	// credential.
	verifiedDc *delegatedCredential
	// serverName contains the server name indicated by the client, if any.
	serverName string
	// secureRenegotiation is true if the server echoed the secure
	// renegotiation extension. (This is meaningless as a server because
	// renegotiation is not supported in that case.)
	secureRenegotiation bool
	// indicates wether extended MasterSecret extension is used (see RFC7627)
	useEMS bool

	// clientFinishedIsFirst is true if the client sent the first Finished
	// message during the most recent handshake. This is recorded because
	// the first transmitted Finished message is the tls-unique
	// channel-binding value.
	clientFinishedIsFirst bool

	// closeNotifyErr is any error from sending the alertCloseNotify record.
	closeNotifyErr error
	// closeNotifySent is true if the Conn attempted to send an
	// alertCloseNotify record.
	closeNotifySent bool

	// clientFinished and serverFinished contain the Finished message sent
	// by the client or server in the most recent handshake. This is
	// retained to support the renegotiation extension and tls-unique
	// channel-binding.
	clientFinished [12]byte
	serverFinished [12]byte

	clientProtocol         string
	clientProtocolFallback bool

	// ticketMaxEarlyData is the maximum bytes of 0-RTT application data
	// that the client is allowed to send on the ticket it used.
	ticketMaxEarlyData int64

	// input/output
	in, out   halfConn     // in.Mutex < out.Mutex
	rawInput  *block       // raw input, right off the wire
	input     *block       // application data waiting to be read
	hand      bytes.Buffer // handshake data waiting to be read
	buffering bool         // whether records are buffered in sendBuf
	sendBuf   []byte       // a buffer of records waiting to be sent

	// bytesSent counts the bytes of application data sent.
	// packetsSent counts packets.
	bytesSent   int64
	packetsSent int64

	// warnCount counts the number of consecutive warning alerts received
	// by Conn.readRecord. Protected by in.Mutex.
	warnCount int

	// activeCall is an atomic int32; the low bit is whether Close has
	// been called. the rest of the bits are the number of goroutines
	// in Conn.Write.
	activeCall int32

	// TLS 1.3 needs the server state until it reaches the Client Finished
	hs *serverHandshakeState

	// earlyDataBytes is the number of bytes of early data received so
	// far. Tracked to enforce max_early_data_size.
	// We don't keep track of rejected 0-RTT data since there's no need
	// to ever buffer it. in.Mutex.
	earlyDataBytes int64

	// binder is the value of the PSK binder that was validated to
	// accept the 0-RTT data. Exposed as ConnectionState.Unique0RTTToken.
	binder []byte

	tmp [16]byte
}

type handshakeStatus int

const (
	handshakeRunning handshakeStatus = iota
	discardingEarlyData
	readingEarlyData
	waitingClientFinished
	readingClientFinished
	handshakeConfirmed
)

// Access to net.Conn methods.
// Cannot just embed net.Conn because that would
// export the struct field too.

// LocalAddr returns the local network address.
func (c *Conn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr returns the remote network address.
func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// SetDeadline sets the read and write deadlines associated with the connection.
// A zero value for t means Read and Write will not time out.
// After a Write has timed out, the TLS state is corrupt and all future writes will return the same error.
func (c *Conn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

// SetReadDeadline sets the read deadline on the underlying connection.
// A zero value for t means Read will not time out.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline on the underlying connection.
// A zero value for t means Write will not time out.
// After a Write has timed out, the TLS state is corrupt and all future writes will return the same error.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// A halfConn represents one direction of the record layer
// connection, either sending or receiving.
type halfConn struct {
	sync.Mutex

	err            error       // first permanent error
	version        uint16      // protocol version
	cipher         interface{} // cipher algorithm
	mac            macFunction
	seq            [8]byte  // 64-bit sequence number
	bfree          *block   // list of free blocks
	additionalData [13]byte // to avoid allocs; interface method args escape

	nextCipher interface{} // next encryption state
	nextMac    macFunction // next MAC algorithm

	// used to save allocating a new buffer for each MAC.
	inDigestBuf, outDigestBuf []byte

	traceErr func(error)
}

func (hc *halfConn) setErrorLocked(err error) error {
	hc.err = err
	if hc.traceErr != nil {
		hc.traceErr(err)
	}
	return err
}

// prepareCipherSpec sets the encryption and MAC states
// that a subsequent changeCipherSpec will use.
func (hc *halfConn) prepareCipherSpec(version uint16, cipher interface{}, mac macFunction) {
	hc.version = version
	hc.nextCipher = cipher
	hc.nextMac = mac
}

// changeCipherSpec changes the encryption and MAC states
// to the ones previously passed to prepareCipherSpec.
func (hc *halfConn) changeCipherSpec() error {
	if hc.nextCipher == nil {
		return alertInternalError
	}
	hc.cipher = hc.nextCipher
	hc.mac = hc.nextMac
	hc.nextCipher = nil
	hc.nextMac = nil
	for i := range hc.seq {
		hc.seq[i] = 0
	}
	return nil
}

func (hc *halfConn) setCipher(version uint16, cipher interface{}) {
	hc.version = version
	hc.cipher = cipher
	for i := range hc.seq {
		hc.seq[i] = 0
	}
}

// incSeq increments the sequence number.
func (hc *halfConn) incSeq() {
	for i := 7; i >= 0; i-- {
		hc.seq[i]++
		if hc.seq[i] != 0 {
			return
		}
	}

	// Not allowed to let sequence number wrap.
	// Instead, must renegotiate before it does.
	// Not likely enough to bother.
	panic("TLS: sequence number wraparound")
}

// extractPadding returns, in constant time, the length of the padding to remove
// from the end of payload. It also returns a byte which is equal to 255 if the
// padding was valid and 0 otherwise. See RFC 2246, section 6.2.3.2
func extractPadding(payload []byte) (toRemove int, good byte) {
	if len(payload) < 1 {
		return 0, 0
	}

	paddingLen := payload[len(payload)-1]
	t := uint(len(payload)-1) - uint(paddingLen)
	// if len(payload) >= (paddingLen - 1) then the MSB of t is zero
	good = byte(int32(^t) >> 31)

	// The maximum possible padding length plus the actual length field
	toCheck := 256
	// The length of the padded data is public, so we can use an if here
	if toCheck > len(payload) {
		toCheck = len(payload)
	}

	for i := 0; i < toCheck; i++ {
		t := uint(paddingLen) - uint(i)
		// if i <= paddingLen then the MSB of t is zero
		mask := byte(int32(^t) >> 31)
		b := payload[len(payload)-1-i]
		good &^= mask&paddingLen ^ mask&b
	}

	// We AND together the bits of good and replicate the result across
	// all the bits.
	good &= good << 4
	good &= good << 2
	good &= good << 1
	good = uint8(int8(good) >> 7)

	toRemove = int(paddingLen) + 1
	return
}

// extractPaddingSSL30 is a replacement for extractPadding in the case that the
// protocol version is SSLv3. In this version, the contents of the padding
// are random and cannot be checked.
func extractPaddingSSL30(payload []byte) (toRemove int, good byte) {
	if len(payload) < 1 {
		return 0, 0
	}

	paddingLen := int(payload[len(payload)-1]) + 1
	if paddingLen > len(payload) {
		return 0, 0
	}

	return paddingLen, 255
}

func roundUp(a, b int) int {
	return a + (b-a%b)%b
}

// cbcMode is an interface for block ciphers using cipher block chaining.
type cbcMode interface {
	cipher.BlockMode
	SetIV([]byte)
}

// decrypt checks and strips the mac and decrypts the data in b. Returns a
// success boolean, the number of bytes to skip from the start of the record in
// order to get the application payload, and an optional alert value.
func (hc *halfConn) decrypt(b *block) (ok bool, prefixLen int, alertValue alert) {
	// pull out payload
	payload := b.data[recordHeaderLen:]

	macSize := 0
	if hc.mac != nil {
		macSize = hc.mac.Size()
	}

	paddingGood := byte(255)
	paddingLen := 0
	explicitIVLen := 0

	// decrypt
	if hc.cipher != nil {
		switch c := hc.cipher.(type) {
		case cipher.Stream:
			c.XORKeyStream(payload, payload)
		case aead:
			explicitIVLen = c.explicitNonceLen()
			if len(payload) < explicitIVLen {
				return false, 0, alertBadRecordMAC
			}
			nonce := payload[:explicitIVLen]
			payload = payload[explicitIVLen:]

			if len(nonce) == 0 {
				nonce = hc.seq[:]
			}

			var additionalData []byte
			if hc.version < VersionTLS13 {
				copy(hc.additionalData[:], hc.seq[:])
				copy(hc.additionalData[8:], b.data[:3])
				n := len(payload) - c.Overhead()
				hc.additionalData[11] = byte(n >> 8)
				hc.additionalData[12] = byte(n)
				additionalData = hc.additionalData[:]
			} else {
				if len(payload) > int((1<<14)+256) {
					return false, 0, alertRecordOverflow
				}
				// Check AD header, see 5.2 of RFC8446
				additionalData = make([]byte, 5)
				additionalData[0] = byte(recordTypeApplicationData)
				binary.BigEndian.PutUint16(additionalData[1:], VersionTLS12)
				binary.BigEndian.PutUint16(additionalData[3:], uint16(len(payload)))
			}
			var err error
			payload, err = c.Open(payload[:0], nonce, payload, additionalData)
			if err != nil {
				return false, 0, alertBadRecordMAC
			}
			b.resize(recordHeaderLen + explicitIVLen + len(payload))
		case cbcMode:
			blockSize := c.BlockSize()
			if hc.version >= VersionTLS11 {
				explicitIVLen = blockSize
			}

			if len(payload)%blockSize != 0 || len(payload) < roundUp(explicitIVLen+macSize+1, blockSize) {
				return false, 0, alertBadRecordMAC
			}

			if explicitIVLen > 0 {
				c.SetIV(payload[:explicitIVLen])
				payload = payload[explicitIVLen:]
			}
			c.CryptBlocks(payload, payload)
			if hc.version == VersionSSL30 {
				paddingLen, paddingGood = extractPaddingSSL30(payload)
			} else {
				paddingLen, paddingGood = extractPadding(payload)

				// To protect against CBC padding oracles like Lucky13, the data
				// past paddingLen (which is secret) is passed to the MAC
				// function as extra data, to be fed into the HMAC after
				// computing the digest. This makes the MAC constant time as
				// long as the digest computation is constant time and does not
				// affect the subsequent write.
			}
		default:
			panic("unknown cipher type")
		}
	}

	// check, strip mac
	if hc.mac != nil {
		if len(payload) < macSize {
			return false, 0, alertBadRecordMAC
		}

		// strip mac off payload, b.data
		n := len(payload) - macSize - paddingLen
		n = subtle.ConstantTimeSelect(int(uint32(n)>>31), 0, n) // if n < 0 { n = 0 }
		b.data[3] = byte(n >> 8)
		b.data[4] = byte(n)
		remoteMAC := payload[n : n+macSize]
		localMAC := hc.mac.MAC(hc.inDigestBuf, hc.seq[0:], b.data[:recordHeaderLen], payload[:n], payload[n+macSize:])

		if subtle.ConstantTimeCompare(localMAC, remoteMAC) != 1 || paddingGood != 255 {
			return false, 0, alertBadRecordMAC
		}
		hc.inDigestBuf = localMAC

		b.resize(recordHeaderLen + explicitIVLen + n)
	}
	hc.incSeq()

	return true, recordHeaderLen + explicitIVLen, 0
}

// padToBlockSize calculates the needed padding block, if any, for a payload.
// On exit, prefix aliases payload and extends to the end of the last full
// block of payload. finalBlock is a fresh slice which contains the contents of
// any suffix of payload as well as the needed padding to make finalBlock a
// full block.
func padToBlockSize(payload []byte, blockSize int) (prefix, finalBlock []byte) {
	overrun := len(payload) % blockSize
	paddingLen := blockSize - overrun
	prefix = payload[:len(payload)-overrun]
	finalBlock = make([]byte, blockSize)
	copy(finalBlock, payload[len(payload)-overrun:])
	for i := overrun; i < blockSize; i++ {
		finalBlock[i] = byte(paddingLen - 1)
	}
	return
}

// encrypt encrypts and macs the data in b.
func (hc *halfConn) encrypt(b *block, explicitIVLen int) (bool, alert) {
	// mac
	if hc.mac != nil {
		mac := hc.mac.MAC(hc.outDigestBuf, hc.seq[0:], b.data[:recordHeaderLen], b.data[recordHeaderLen+explicitIVLen:], nil)

		n := len(b.data)
		b.resize(n + len(mac))
		copy(b.data[n:], mac)
		hc.outDigestBuf = mac
	}

	payload := b.data[recordHeaderLen:]

	// encrypt
	if hc.cipher != nil {
		switch c := hc.cipher.(type) {
		case cipher.Stream:
			c.XORKeyStream(payload, payload)
		case aead:
			// explicitIVLen is always 0 for TLS1.3
			payloadLen := len(b.data) - recordHeaderLen - explicitIVLen
			payloadOffset := recordHeaderLen + explicitIVLen
			nonce := b.data[recordHeaderLen : recordHeaderLen+explicitIVLen]
			if len(nonce) == 0 {
				nonce = hc.seq[:]
			}

			var additionalData []byte
			if hc.version < VersionTLS13 {
				// make room in a buffer for payload + MAC
				b.resize(len(b.data) + c.Overhead())

				payload = b.data[payloadOffset : payloadOffset+payloadLen]
				copy(hc.additionalData[:], hc.seq[:])
				copy(hc.additionalData[8:], b.data[:3])
				binary.BigEndian.PutUint16(hc.additionalData[11:], uint16(payloadLen))
				additionalData = hc.additionalData[:]
			} else {
				// make room in a buffer for TLSCiphertext.encrypted_record:
				// payload + MAC + extra data if needed
				b.resize(len(b.data) + c.Overhead() + 1)

				payload = b.data[payloadOffset : payloadOffset+payloadLen+1]
				// 1 byte of content type is appended to payload and encrypted
				payload[len(payload)-1] = b.data[0]

				// opaque_type
				b.data[0] = byte(recordTypeApplicationData)

				// Add AD header, see 5.2 of RFC8446
				additionalData = make([]byte, 5)
				additionalData[0] = b.data[0]
				binary.BigEndian.PutUint16(additionalData[1:], VersionTLS12)
				binary.BigEndian.PutUint16(additionalData[3:], uint16(len(payload)+c.Overhead()))
			}
			c.Seal(payload[:0], nonce, payload, additionalData)
		case cbcMode:
			blockSize := c.BlockSize()
			if explicitIVLen > 0 {
				c.SetIV(payload[:explicitIVLen])
				payload = payload[explicitIVLen:]
			}
			prefix, finalBlock := padToBlockSize(payload, blockSize)
			b.resize(recordHeaderLen + explicitIVLen + len(prefix) + len(finalBlock))
			c.CryptBlocks(b.data[recordHeaderLen+explicitIVLen:], prefix)
			c.CryptBlocks(b.data[recordHeaderLen+explicitIVLen+len(prefix):], finalBlock)
		default:
			panic("unknown cipher type")
		}
	}

	// update length to include MAC and any block padding needed.
	n := len(b.data) - recordHeaderLen
	b.data[3] = byte(n >> 8)
	b.data[4] = byte(n)
	hc.incSeq()

	return true, 0
}

// A block is a simple data buffer.
type block struct {
	data []byte
	off  int // index for Read
	link *block
}

// resize resizes block to be n bytes, growing if necessary.
func (b *block) resize(n int) {
	if n > cap(b.data) {
		b.reserve(n)
	}
	b.data = b.data[0:n]
}

// reserve makes sure that block contains a capacity of at least n bytes.
func (b *block) reserve(n int) {
	if cap(b.data) >= n {
		return
	}
	m := cap(b.data)
	if m == 0 {
		m = 1024
	}
	for m < n {
		m *= 2
	}
	data := make([]byte, len(b.data), m)
	copy(data, b.data)
	b.data = data
}

// readFromUntil reads from r into b until b contains at least n bytes
// or else returns an error.
func (b *block) readFromUntil(r io.Reader, n int) error {
	// quick case
	if len(b.data) >= n {
		return nil
	}

	// read until have enough.
	b.reserve(n)
	for {
		m, err := r.Read(b.data[len(b.data):cap(b.data)])
		b.data = b.data[0 : len(b.data)+m]
		if len(b.data) >= n {
			// TODO(bradfitz,agl): slightly suspicious
			// that we're throwing away r.Read's err here.
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *block) Read(p []byte) (n int, err error) {
	n = copy(p, b.data[b.off:])
	b.off += n
	if b.off >= len(b.data) {
		err = io.EOF
	}
	return
}

// newBlock allocates a new block, from hc's free list if possible.
func (hc *halfConn) newBlock() *block {
	b := hc.bfree
	if b == nil {
		return new(block)
	}
	hc.bfree = b.link
	b.link = nil
	b.resize(0)
	return b
}

// freeBlock returns a block to hc's free list.
// The protocol is such that each side only has a block or two on
// its free list at a time, so there's no need to worry about
// trimming the list, etc.
func (hc *halfConn) freeBlock(b *block) {
	b.link = hc.bfree
	hc.bfree = b
}

// splitBlock splits a block after the first n bytes,
// returning a block with those n bytes and a
// block with the remainder.  the latter may be nil.
func (hc *halfConn) splitBlock(b *block, n int) (*block, *block) {
	if len(b.data) <= n {
		return b, nil
	}
	bb := hc.newBlock()
	bb.resize(len(b.data) - n)
	copy(bb.data, b.data[n:])
	b.data = b.data[0:n]
	return b, bb
}

// RecordHeaderError results when a TLS record header is invalid.
type RecordHeaderError struct {
	// Msg contains a human readable string that describes the error.
	Msg string
	// RecordHeader contains the five bytes of TLS record header that
	// triggered the error.
	RecordHeader [5]byte
}

func (e RecordHeaderError) Error() string { return "tls: " + e.Msg }

func (c *Conn) newRecordHeaderError(msg string) (err RecordHeaderError) {
	err.Msg = msg
	copy(err.RecordHeader[:], c.rawInput.data)
	return err
}

// readRecord reads the next TLS record from the connection
// and updates the record layer state.
// c.in.Mutex <= L; c.input == nil.
// c.input can still be nil after a call, retry if so.
func (c *Conn) readRecord(want recordType) error {
	// Caller must be in sync with connection:
	// handshake data if handshake not yet completed,
	// else application data.
	switch want {
	default:
		c.sendAlert(alertInternalError)
		return c.in.setErrorLocked(errors.New("tls: unknown record type requested"))
	case recordTypeHandshake, recordTypeChangeCipherSpec:
		if c.phase != handshakeRunning && c.phase != readingClientFinished {
			c.sendAlert(alertInternalError)
			return c.in.setErrorLocked(errors.New("tls: handshake or ChangeCipherSpec requested while not in handshake"))
		}
	case recordTypeApplicationData:
		if c.phase == handshakeRunning || c.phase == readingClientFinished {
			c.sendAlert(alertInternalError)
			return c.in.setErrorLocked(errors.New("tls: application data record requested while in handshake"))
		}
	}

Again:
	if c.rawInput == nil {
		c.rawInput = c.in.newBlock()
	}
	b := c.rawInput

	// Read header, payload.
	if err := b.readFromUntil(c.conn, recordHeaderLen); err != nil {
		// RFC suggests that EOF without an alertCloseNotify is
		// an error, but popular web sites seem to do this,
		// so we can't make it an error.
		// if err == io.EOF {
		// 	err = io.ErrUnexpectedEOF
		// }
		if e, ok := err.(net.Error); !ok || !e.Temporary() {
			c.in.setErrorLocked(err)
		}
		return err
	}
	typ := recordType(b.data[0])

	// No valid TLS record has a type of 0x80, however SSLv2 handshakes
	// start with a uint16 length where the MSB is set and the first record
	// is always < 256 bytes long. Therefore typ == 0x80 strongly suggests
	// an SSLv2 client.
	if want == recordTypeHandshake && typ == 0x80 {
		c.sendAlert(alertProtocolVersion)
		return c.in.setErrorLocked(c.newRecordHeaderError("unsupported SSLv2 handshake received"))
	}

	vers := uint16(b.data[1])<<8 | uint16(b.data[2])
	n := int(b.data[3])<<8 | int(b.data[4])
	if n > maxCiphertext {
		c.sendAlert(alertRecordOverflow)
		msg := fmt.Sprintf("oversized record received with length %d", n)
		return c.in.setErrorLocked(c.newRecordHeaderError(msg))
	}
	if !c.haveVers {
		// First message, be extra suspicious: this might not be a TLS
		// client. Bail out before reading a full 'body', if possible.
		// The current max version is 3.3 so if the version is >= 16.0,
		// it's probably not real.
		if (typ != recordTypeAlert && typ != want) || vers >= 0x1000 {
			c.sendAlert(alertUnexpectedMessage)
			return c.in.setErrorLocked(c.newRecordHeaderError("first record does not look like a TLS handshake"))
		}
	}
	if err := b.readFromUntil(c.conn, recordHeaderLen+n); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		if e, ok := err.(net.Error); !ok || !e.Temporary() {
			c.in.setErrorLocked(err)
		}
		return err
	}

	// Process message.
	b, c.rawInput = c.in.splitBlock(b, recordHeaderLen+n)

	// TLS 1.3 middlebox compatibility: skip over unencrypted CCS.
	if c.vers >= VersionTLS13 && typ == recordTypeChangeCipherSpec && c.phase != handshakeConfirmed {
		if len(b.data) != 6 || b.data[5] != 1 {
			c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
		}
		c.in.freeBlock(b)
		return c.in.err
	}

	peekedAlert := peekAlert(b) // peek at a possible alert before decryption
	ok, off, alertValue := c.in.decrypt(b)
	switch {
	case !ok && c.phase == discardingEarlyData:
		// If the client said that it's sending early data and we did not
		// accept it, we are expected to fail decryption.
		c.in.freeBlock(b)
		return nil
	case ok && c.phase == discardingEarlyData:
		c.phase = waitingClientFinished
	case !ok:
		c.in.traceErr, c.out.traceErr = nil, nil // not that interesting
		c.in.freeBlock(b)
		err := c.sendAlert(alertValue)
		// If decryption failed because the message is an unencrypted
		// alert, return a more meaningful error message
		if alertValue == alertBadRecordMAC && peekedAlert != nil {
			err = peekedAlert
		}
		return c.in.setErrorLocked(err)
	}
	b.off = off
	data := b.data[b.off:]
	if (c.vers < VersionTLS13 && len(data) > maxPlaintext) || len(data) > maxPlaintext+1 {
		c.in.freeBlock(b)
		return c.in.setErrorLocked(c.sendAlert(alertRecordOverflow))
	}

	// After checking the plaintext length, remove 1.3 padding and
	// extract the real content type.
	// See https://tools.ietf.org/html/draft-ietf-tls-tls13-18#section-5.4.
	if c.vers >= VersionTLS13 {
		i := len(data) - 1
		for i >= 0 {
			if data[i] != 0 {
				break
			}
			i--
		}
		if i < 0 {
			c.in.freeBlock(b)
			return c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
		}
		typ = recordType(data[i])
		data = data[:i]
		b.resize(b.off + i) // shrinks, guaranteed not to reallocate
	}

	if typ != recordTypeAlert && len(data) > 0 {
		// this is a valid non-alert message: reset the count of alerts
		c.warnCount = 0
	}

	switch typ {
	default:
		c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))

	case recordTypeAlert:
		if len(data) != 2 {
			c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
			break
		}
		if alert(data[1]) == alertCloseNotify {
			c.in.setErrorLocked(io.EOF)
			break
		}
		switch data[0] {
		case alertLevelWarning:
			// drop on the floor
			c.in.freeBlock(b)

			c.warnCount++
			if c.warnCount > maxWarnAlertCount {
				c.sendAlert(alertUnexpectedMessage)
				return c.in.setErrorLocked(errors.New("tls: too many warn alerts"))
			}

			goto Again
		case alertLevelError:
			c.in.setErrorLocked(&net.OpError{Op: "remote error", Err: alert(data[1])})
		default:
			c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
		}

	case recordTypeChangeCipherSpec:
		if typ != want || len(data) != 1 || data[0] != 1 || c.vers >= VersionTLS13 {
			c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
			break
		}
		// Handshake messages are not allowed to fragment across the CCS
		if c.hand.Len() > 0 {
			c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
			break
		}
		// Handshake messages are not allowed to fragment across the CCS
		if c.hand.Len() > 0 {
			c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
			break
		}
		err := c.in.changeCipherSpec()
		if err != nil {
			c.in.setErrorLocked(c.sendAlert(err.(alert)))
		}

	case recordTypeApplicationData:
		if typ != want || c.phase == waitingClientFinished {
			c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
			break
		}
		if c.phase == readingEarlyData {
			c.earlyDataBytes += int64(len(b.data) - b.off)
			if c.earlyDataBytes > c.ticketMaxEarlyData {
				return c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
			}
		}
		c.input = b
		b = nil

	case recordTypeHandshake:
		// TODO(rsc): Should at least pick off connection close.
		// If early data was being read, a Finished message is expected
		// instead of (early) application data. Other post-handshake
		// messages include HelloRequest and NewSessionTicket.
		if typ != want && want != recordTypeApplicationData {
			return c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
		}
		c.hand.Write(data)
	}

	if b != nil {
		c.in.freeBlock(b)
	}
	return c.in.err
}

// peekAlert looks at a message to spot an unencrypted alert. It must be
// called before decryption to avoid a side channel, and its result must
// only be used if decryption fails, to avoid false positives.
func peekAlert(b *block) error {
	if len(b.data) < 7 {
		return nil
	}
	if recordType(b.data[0]) != recordTypeAlert {
		return nil
	}
	return &net.OpError{Op: "remote error", Err: alert(b.data[6])}
}

// sendAlert sends a TLS alert message.
// c.out.Mutex <= L.
func (c *Conn) sendAlertLocked(err alert) error {

	// [Psiphon]
	// Do not send TLS alerts before the passthrough state is determined.
	// Otherwise, an invalid client would receive non-passthrough traffic.
	//
	// Limitation: ClientHello-related alerts to legitimate clients are not sent.
	// This changes the nature of errors that such clients may report when their
	// TLS handshake fails. This change in behavior is only visible to legitimate
	// clients.
	if c.config.PassthroughAddress != "" &&
		c.conn.(*recorderConn).IsRecording() {
		return nil
	}

	switch err {
	case alertNoRenegotiation, alertCloseNotify:
		c.tmp[0] = alertLevelWarning
	default:
		c.tmp[0] = alertLevelError
	}
	c.tmp[1] = byte(err)

	_, writeErr := c.writeRecordLocked(recordTypeAlert, c.tmp[0:2])
	if err == alertCloseNotify {
		// closeNotify is a special case in that it isn't an error.
		return writeErr
	}

	return c.out.setErrorLocked(&net.OpError{Op: "local error", Err: err})
}

// sendAlert sends a TLS alert message.
// L < c.out.Mutex.
func (c *Conn) sendAlert(err alert) error {
	c.out.Lock()
	defer c.out.Unlock()
	return c.sendAlertLocked(err)
}

const (
	// tcpMSSEstimate is a conservative estimate of the TCP maximum segment
	// size (MSS). A constant is used, rather than querying the kernel for
	// the actual MSS, to avoid complexity. The value here is the IPv6
	// minimum MTU (1280 bytes) minus the overhead of an IPv6 header (40
	// bytes) and a TCP header with timestamps (32 bytes).
	tcpMSSEstimate = 1208

	// recordSizeBoostThreshold is the number of bytes of application data
	// sent after which the TLS record size will be increased to the
	// maximum.
	recordSizeBoostThreshold = 128 * 1024
)

// maxPayloadSizeForWrite returns the maximum TLS payload size to use for the
// next application data record. There is the following trade-off:
//
//   - For latency-sensitive applications, such as web browsing, each TLS
//     record should fit in one TCP segment.
//   - For throughput-sensitive applications, such as large file transfers,
//     larger TLS records better amortize framing and encryption overheads.
//
// A simple heuristic that works well in practice is to use small records for
// the first 1MB of data, then use larger records for subsequent data, and
// reset back to smaller records after the connection becomes idle. See "High
// Performance Web Networking", Chapter 4, or:
// https://www.igvita.com/2013/10/24/optimizing-tls-record-size-and-buffering-latency/
//
// In the interests of simplicity and determinism, this code does not attempt
// to reset the record size once the connection is idle, however.
//
// c.out.Mutex <= L.
func (c *Conn) maxPayloadSizeForWrite(typ recordType, explicitIVLen int) int {
	if c.config.DynamicRecordSizingDisabled || typ != recordTypeApplicationData {
		return maxPlaintext
	}

	if c.bytesSent >= recordSizeBoostThreshold {
		return maxPlaintext
	}

	// Subtract TLS overheads to get the maximum payload size.
	macSize := 0
	if c.out.mac != nil {
		macSize = c.out.mac.Size()
	}

	payloadBytes := tcpMSSEstimate - recordHeaderLen - explicitIVLen
	if c.out.cipher != nil {
		switch ciph := c.out.cipher.(type) {
		case cipher.Stream:
			payloadBytes -= macSize
		case cipher.AEAD:
			payloadBytes -= ciph.Overhead()
			if c.vers >= VersionTLS13 {
				payloadBytes -= 1 // ContentType
			}
		case cbcMode:
			blockSize := ciph.BlockSize()
			// The payload must fit in a multiple of blockSize, with
			// room for at least one padding byte.
			payloadBytes = (payloadBytes & ^(blockSize - 1)) - 1
			// The MAC is appended before padding so affects the
			// payload size directly.
			payloadBytes -= macSize
		default:
			panic("unknown cipher type")
		}
	}

	// Allow packet growth in arithmetic progression up to max.
	pkt := c.packetsSent
	c.packetsSent++
	if pkt > 1000 {
		return maxPlaintext // avoid overflow in multiply below
	}

	n := payloadBytes * int(pkt+1)
	if n > maxPlaintext {
		n = maxPlaintext
	}
	return n
}

// c.out.Mutex <= L.
func (c *Conn) write(data []byte) (int, error) {
	if c.buffering {
		c.sendBuf = append(c.sendBuf, data...)
		return len(data), nil
	}

	n, err := c.conn.Write(data)
	c.bytesSent += int64(n)
	return n, err
}

func (c *Conn) flush() (int, error) {
	if len(c.sendBuf) == 0 {
		return 0, nil
	}

	n, err := c.conn.Write(c.sendBuf)
	c.bytesSent += int64(n)
	c.sendBuf = nil
	c.buffering = false
	return n, err
}

// writeRecordLocked writes a TLS record with the given type and payload to the
// connection and updates the record layer state.
// c.out.Mutex <= L.
func (c *Conn) writeRecordLocked(typ recordType, data []byte) (int, error) {
	b := c.out.newBlock()
	defer c.out.freeBlock(b)

	var n int
	for len(data) > 0 {
		explicitIVLen := 0
		explicitIVIsSeq := false

		var cbc cbcMode
		if c.out.version >= VersionTLS11 {
			var ok bool
			if cbc, ok = c.out.cipher.(cbcMode); ok {
				explicitIVLen = cbc.BlockSize()
			}
		}
		if explicitIVLen == 0 {
			if c, ok := c.out.cipher.(aead); ok {
				explicitIVLen = c.explicitNonceLen()

				// The AES-GCM construction in TLS has an
				// explicit nonce so that the nonce can be
				// random. However, the nonce is only 8 bytes
				// which is too small for a secure, random
				// nonce. Therefore we use the sequence number
				// as the nonce.
				explicitIVIsSeq = explicitIVLen > 0
			}
		}
		m := len(data)
		if maxPayload := c.maxPayloadSizeForWrite(typ, explicitIVLen); m > maxPayload {
			m = maxPayload
		}
		b.resize(recordHeaderLen + explicitIVLen + m)
		b.data[0] = byte(typ)
		vers := c.vers
		if vers == 0 {
			// Some TLS servers fail if the record version is
			// greater than TLS 1.0 for the initial ClientHello.
			vers = VersionTLS10
		}
		if c.vers >= VersionTLS13 {
			// TLS 1.3 froze the record layer version at { 3, 1 }.
			// See https://tools.ietf.org/html/draft-ietf-tls-tls13-18#section-5.1.
			// But for draft 22, this was changed to { 3, 3 }.
			vers = VersionTLS12
		}
		b.data[1] = byte(vers >> 8)
		b.data[2] = byte(vers)
		b.data[3] = byte(m >> 8)
		b.data[4] = byte(m)
		if explicitIVLen > 0 {
			explicitIV := b.data[recordHeaderLen : recordHeaderLen+explicitIVLen]
			if explicitIVIsSeq {
				copy(explicitIV, c.out.seq[:])
			} else {
				if _, err := io.ReadFull(c.config.rand(), explicitIV); err != nil {
					return n, err
				}
			}
		}
		copy(b.data[recordHeaderLen+explicitIVLen:], data)
		c.out.encrypt(b, explicitIVLen)
		if _, err := c.write(b.data); err != nil {
			return n, err
		}
		n += m
		data = data[m:]
	}

	if typ == recordTypeChangeCipherSpec && c.vers < VersionTLS13 {
		if err := c.out.changeCipherSpec(); err != nil {
			return n, c.sendAlertLocked(err.(alert))
		}
	}

	return n, nil
}

// writeRecord writes a TLS record with the given type and payload to the
// connection and updates the record layer state.
// L < c.out.Mutex.
func (c *Conn) writeRecord(typ recordType, data []byte) (int, error) {
	c.out.Lock()
	defer c.out.Unlock()

	return c.writeRecordLocked(typ, data)
}

// readHandshake reads the next handshake message from
// the record layer.
// c.in.Mutex < L; c.out.Mutex < L.
func (c *Conn) readHandshake() (interface{}, error) {
	for c.hand.Len() < 4 {
		if err := c.in.err; err != nil {
			return nil, err
		}
		if err := c.readRecord(recordTypeHandshake); err != nil {
			return nil, err
		}
	}

	data := c.hand.Bytes()
	n := int(data[1])<<16 | int(data[2])<<8 | int(data[3])
	if n > maxHandshake {
		c.sendAlertLocked(alertInternalError)
		return nil, c.in.setErrorLocked(fmt.Errorf("tls: handshake message of length %d bytes exceeds maximum of %d bytes", n, maxHandshake))
	}
	for c.hand.Len() < 4+n {
		if err := c.in.err; err != nil {
			return nil, err
		}
		if err := c.readRecord(recordTypeHandshake); err != nil {
			return nil, err
		}
	}
	data = c.hand.Next(4 + n)
	var m handshakeMessage
	switch data[0] {
	case typeHelloRequest:
		m = new(helloRequestMsg)
	case typeClientHello:
		m = new(clientHelloMsg)
	case typeServerHello:
		m = new(serverHelloMsg)
	case typeEncryptedExtensions:
		m = new(encryptedExtensionsMsg)
	case typeNewSessionTicket:
		if c.vers >= VersionTLS13 {
			m = new(newSessionTicketMsg13)
		} else {
			m = new(newSessionTicketMsg)
		}
	case typeEndOfEarlyData:
		m = new(endOfEarlyDataMsg)
	case typeCertificate:
		if c.vers >= VersionTLS13 {
			m = new(certificateMsg13)
		} else {
			m = new(certificateMsg)
		}
	case typeCertificateRequest:
		if c.vers >= VersionTLS13 {
			m = new(certificateRequestMsg13)
		} else {
			m = &certificateRequestMsg{
				hasSignatureAndHash: c.vers >= VersionTLS12,
			}
		}
	case typeCertificateStatus:
		m = new(certificateStatusMsg)
	case typeServerKeyExchange:
		m = new(serverKeyExchangeMsg)
	case typeServerHelloDone:
		m = new(serverHelloDoneMsg)
	case typeClientKeyExchange:
		m = new(clientKeyExchangeMsg)
	case typeCertificateVerify:
		m = &certificateVerifyMsg{
			hasSignatureAndHash: c.vers >= VersionTLS12,
		}
	case typeNextProtocol:
		m = new(nextProtoMsg)
	case typeFinished:
		m = new(finishedMsg)
	default:
		return nil, c.in.setErrorLocked(c.sendAlert(alertUnexpectedMessage))
	}

	// The handshake message unmarshalers
	// expect to be able to keep references to data,
	// so pass in a fresh copy that won't be overwritten.
	data = append([]byte(nil), data...)

	if unmarshalAlert := m.unmarshal(data); unmarshalAlert != alertSuccess {
		return nil, c.in.setErrorLocked(c.sendAlert(unmarshalAlert))
	}
	return m, nil
}

var (
	errClosed   = errors.New("tls: use of closed connection")
	errShutdown = errors.New("tls: protocol is shutdown")
)

// Write writes data to the connection.
func (c *Conn) Write(b []byte) (int, error) {
	// interlock with Close below
	for {
		x := atomic.LoadInt32(&c.activeCall)
		if x&1 != 0 {
			return 0, errClosed
		}
		if atomic.CompareAndSwapInt32(&c.activeCall, x, x+2) {
			defer atomic.AddInt32(&c.activeCall, -2)
			break
		}
	}

	if err := c.Handshake(); err != nil {
		return 0, err
	}

	c.out.Lock()
	defer c.out.Unlock()

	if err := c.out.err; err != nil {
		return 0, err
	}

	if !c.handshakeComplete() {
		return 0, alertInternalError
	}

	if c.closeNotifySent {
		return 0, errShutdown
	}

	// SSL 3.0 and TLS 1.0 are susceptible to a chosen-plaintext
	// attack when using block mode ciphers due to predictable IVs.
	// This can be prevented by splitting each Application Data
	// record into two records, effectively randomizing the IV.
	//
	// http://www.openssl.org/~bodo/tls-cbc.txt
	// https://bugzilla.mozilla.org/show_bug.cgi?id=665814
	// http://www.imperialviolet.org/2012/01/15/beastfollowup.html

	var m int
	if len(b) > 1 && c.vers <= VersionTLS10 {
		if _, ok := c.out.cipher.(cipher.BlockMode); ok {
			n, err := c.writeRecordLocked(recordTypeApplicationData, b[:1])
			if err != nil {
				return n, c.out.setErrorLocked(err)
			}
			m, b = 1, b[1:]
		}
	}

	n, err := c.writeRecordLocked(recordTypeApplicationData, b)
	return n + m, c.out.setErrorLocked(err)
}

// Process Handshake messages after the handshake has completed.
// c.in.Mutex <= L
func (c *Conn) handlePostHandshake() error {
	msg, err := c.readHandshake()
	if err != nil {
		return err
	}

	switch hm := msg.(type) {
	case *helloRequestMsg:
		return c.handleRenegotiation(hm)
	case *newSessionTicketMsg13:
		if !c.isClient {
			c.sendAlert(alertUnexpectedMessage)
			return alertUnexpectedMessage
		}
		return nil // TODO implement session tickets
	default:
		c.sendAlert(alertUnexpectedMessage)
		return alertUnexpectedMessage
	}
}

// handleRenegotiation processes a HelloRequest handshake message.
// c.in.Mutex <= L
func (c *Conn) handleRenegotiation(*helloRequestMsg) error {
	if !c.isClient {
		return c.sendAlert(alertNoRenegotiation)
	}

	if c.vers >= VersionTLS13 {
		return c.sendAlert(alertNoRenegotiation)
	}

	switch c.config.Renegotiation {
	case RenegotiateNever:
		return c.sendAlert(alertNoRenegotiation)
	case RenegotiateOnceAsClient:
		if c.handshakes > 1 {
			return c.sendAlert(alertNoRenegotiation)
		}
	case RenegotiateFreelyAsClient:
		// Ok.
	default:
		c.sendAlert(alertInternalError)
		return errors.New("tls: unknown Renegotiation value")
	}

	c.handshakeMutex.Lock()
	defer c.handshakeMutex.Unlock()

	c.phase = handshakeRunning
	atomic.StoreUint32(&c.handshakeStatus, 0)
	if c.handshakeErr = c.clientHandshake(); c.handshakeErr == nil {
		c.handshakes++
	}
	return c.handshakeErr
}

// ConfirmHandshake waits for the handshake to reach a point at which
// the connection is certainly not replayed. That is, after receiving
// the Client Finished.
//
// If ConfirmHandshake returns an error and until ConfirmHandshake
// returns, the 0-RTT data should not be trusted not to be replayed.
//
// This is only meaningful in TLS 1.3 when Accept0RTTData is true and the
// client sent valid 0-RTT data. In any other case it's equivalent to
// calling Handshake.
func (c *Conn) ConfirmHandshake() error {
	if c.isClient {
		panic("ConfirmHandshake should only be called for servers")
	}

	if err := c.Handshake(); err != nil {
		return err
	}

	if c.vers < VersionTLS13 {
		return nil
	}

	c.confirmMutex.Lock()
	if atomic.LoadInt32(&c.handshakeConfirmed) == 1 { // c.phase == handshakeConfirmed
		c.confirmMutex.Unlock()
		return nil
	} else {
		defer func() {
			// If we transitioned to handshakeConfirmed we already released the lock,
			// otherwise do it here.
			if c.phase != handshakeConfirmed {
				c.confirmMutex.Unlock()
			}
		}()
	}

	c.in.Lock()
	defer c.in.Unlock()

	var input *block
	// Try to read all data (if phase==readingEarlyData) or extract the
	// remaining data from the previous read that could not fit in the read
	// buffer (if c.input != nil).
	if c.phase == readingEarlyData || c.input != nil {
		buf := &bytes.Buffer{}
		if _, err := buf.ReadFrom(earlyDataReader{c}); err != nil {
			c.in.setErrorLocked(err)
			return err
		}
		input = &block{data: buf.Bytes()}
	}

	// At this point, earlyDataReader has read all early data and received
	// the end_of_early_data signal. Expect a Finished message.
	// Locks held so far: c.confirmMutex, c.in
	// not confirmed implies c.phase == discardingEarlyData || c.phase == waitingClientFinished
	for c.phase != handshakeConfirmed {
		if err := c.hs.readClientFinished13(true); err != nil {
			c.in.setErrorLocked(err)
			return err
		}
	}

	if c.phase != handshakeConfirmed {
		panic("should have reached handshakeConfirmed state")
	}
	if c.input != nil {
		panic("should not have read past the Client Finished")
	}

	c.input = input

	return nil
}

// earlyDataReader wraps a Conn and reads only early data, both buffered
// and still on the wire.
type earlyDataReader struct {
	c *Conn
}

// c.in.Mutex <= L
func (r earlyDataReader) Read(b []byte) (n int, err error) {
	c := r.c

	if c.phase == handshakeConfirmed {
		// c.input might not be early data
		panic("earlyDataReader called at handshakeConfirmed")
	}

	for c.input == nil && c.in.err == nil && c.phase == readingEarlyData {
		if err := c.readRecord(recordTypeApplicationData); err != nil {
			return 0, err
		}
		if c.hand.Len() > 0 {
			if err := c.handleEndOfEarlyData(); err != nil {
				return 0, err
			}
		}
	}
	if err := c.in.err; err != nil {
		return 0, err
	}

	if c.input != nil {
		n, err = c.input.Read(b)
		if err == io.EOF {
			err = nil
			c.in.freeBlock(c.input)
			c.input = nil
		}
	}

	// Following early application data, an end_of_early_data is expected.
	if err == nil && c.phase != readingEarlyData && c.input == nil {
		err = io.EOF
	}
	return
}

// Read can be made to time out and return a net.Error with Timeout() == true
// after a fixed time limit; see SetDeadline and SetReadDeadline.
func (c *Conn) Read(b []byte) (n int, err error) {
	if err = c.Handshake(); err != nil {
		return
	}
	if len(b) == 0 {
		// Put this after Handshake, in case people were calling
		// Read(nil) for the side effect of the Handshake.
		return
	}

	c.confirmMutex.Lock()
	if atomic.LoadInt32(&c.handshakeConfirmed) == 1 { // c.phase == handshakeConfirmed
		c.confirmMutex.Unlock()
	} else {
		defer func() {
			// If we transitioned to handshakeConfirmed we already released the lock,
			// otherwise do it here.
			if c.phase != handshakeConfirmed {
				c.confirmMutex.Unlock()
			}
		}()
	}

	c.in.Lock()
	defer c.in.Unlock()

	// Some OpenSSL servers send empty records in order to randomize the
	// CBC IV. So this loop ignores a limited number of empty records.
	const maxConsecutiveEmptyRecords = 100
	for emptyRecordCount := 0; emptyRecordCount <= maxConsecutiveEmptyRecords; emptyRecordCount++ {
		for c.input == nil && c.in.err == nil {
			if err := c.readRecord(recordTypeApplicationData); err != nil {
				// Soft error, like EAGAIN
				return 0, err
			}
			if c.hand.Len() > 0 {
				if c.phase == readingEarlyData || c.phase == waitingClientFinished {
					if c.phase == readingEarlyData {
						if err := c.handleEndOfEarlyData(); err != nil {
							return 0, err
						}
					}
					// Server has received all early data, confirm
					// by reading the Client Finished message.
					if err := c.hs.readClientFinished13(true); err != nil {
						c.in.setErrorLocked(err)
						return 0, err
					}
					continue
				}
				if err := c.handlePostHandshake(); err != nil {
					return 0, err
				}
			}
		}
		if err := c.in.err; err != nil {
			return 0, err
		}

		n, err = c.input.Read(b)
		if err == io.EOF {
			err = nil
			c.in.freeBlock(c.input)
			c.input = nil
		}

		// If a close-notify alert is waiting, read it so that
		// we can return (n, EOF) instead of (n, nil), to signal
		// to the HTTP response reading goroutine that the
		// connection is now closed. This eliminates a race
		// where the HTTP response reading goroutine would
		// otherwise not observe the EOF until its next read,
		// by which time a client goroutine might have already
		// tried to reuse the HTTP connection for a new
		// request.
		// See https://codereview.appspot.com/76400046
		// and https://github.com/ooni/psiphon/oopsi/golang.org/issue/3514
		if ri := c.rawInput; ri != nil &&
			n != 0 && err == nil &&
			c.input == nil && len(ri.data) > 0 && recordType(ri.data[0]) == recordTypeAlert {
			if recErr := c.readRecord(recordTypeApplicationData); recErr != nil {
				err = recErr // will be io.EOF on closeNotify
			}
		}

		if n != 0 || err != nil {
			return n, err
		}
	}

	return 0, io.ErrNoProgress
}

// Close closes the connection.
func (c *Conn) Close() error {
	// Interlock with Conn.Write above.
	var x int32
	for {
		x = atomic.LoadInt32(&c.activeCall)
		if x&1 != 0 {
			return errClosed
		}
		if atomic.CompareAndSwapInt32(&c.activeCall, x, x|1) {
			break
		}
	}
	if x != 0 {
		// io.Writer and io.Closer should not be used concurrently.
		// If Close is called while a Write is currently in-flight,
		// interpret that as a sign that this Close is really just
		// being used to break the Write and/or clean up resources and
		// avoid sending the alertCloseNotify, which may block
		// waiting on handshakeMutex or the c.out mutex.
		return c.conn.Close()
	}

	var alertErr error

	if c.handshakeComplete() {
		alertErr = c.closeNotify()
	}

	if err := c.conn.Close(); err != nil {
		return err
	}
	return alertErr
}

var errEarlyCloseWrite = errors.New("tls: CloseWrite called before handshake complete")

// CloseWrite shuts down the writing side of the connection. It should only be
// called once the handshake has completed and does not call CloseWrite on the
// underlying connection. Most callers should just use Close.
func (c *Conn) CloseWrite() error {
	if !c.handshakeComplete() {
		return errEarlyCloseWrite
	}

	return c.closeNotify()
}

func (c *Conn) closeNotify() error {
	c.out.Lock()
	defer c.out.Unlock()

	if !c.closeNotifySent {
		c.closeNotifyErr = c.sendAlertLocked(alertCloseNotify)
		c.closeNotifySent = true
	}
	return c.closeNotifyErr
}

// Handshake runs the client or server handshake
// protocol if it has not yet been run.
// Most uses of this package need not call Handshake
// explicitly: the first Read or Write will call it automatically.
//
// In TLS 1.3 Handshake returns after the client and server first flights,
// without waiting for the Client Finished.
func (c *Conn) Handshake() error {
	c.handshakeMutex.Lock()
	defer c.handshakeMutex.Unlock()

	if err := c.handshakeErr; err != nil {
		return err
	}
	if c.handshakeComplete() {
		return nil
	}

	c.in.Lock()
	defer c.in.Unlock()

	// The handshake cannot have completed when handshakeMutex was unlocked
	// because this goroutine set handshakeCond.
	if c.handshakeErr != nil || c.handshakeComplete() {
		panic("handshake should not have been able to complete after handshakeCond was set")
	}

	c.connID = make([]byte, 8)
	if _, err := io.ReadFull(c.config.rand(), c.connID); err != nil {
		return err
	}

	if c.isClient {
		c.handshakeErr = c.clientHandshake()
	} else {
		c.handshakeErr = c.serverHandshake()
	}
	if c.handshakeErr == nil {
		c.handshakes++
	} else {
		// If an error occurred during the hadshake try to flush the
		// alert that might be left in the buffer.
		c.flush()
	}

	if c.handshakeErr == nil && !c.handshakeComplete() {
		panic("handshake should have had a result.")
	}

	return c.handshakeErr
}

// ConnectionState returns basic TLS details about the connection.
func (c *Conn) ConnectionState() ConnectionState {
	c.handshakeMutex.Lock()
	defer c.handshakeMutex.Unlock()

	var state ConnectionState
	state.HandshakeComplete = c.handshakeComplete()
	state.ServerName = c.serverName

	if state.HandshakeComplete {
		state.ConnectionID = c.connID
		state.ClientHello = c.clientHello
		state.Version = c.vers
		state.NegotiatedProtocol = c.clientProtocol
		state.DidResume = c.didResume
		state.NegotiatedProtocolIsMutual = !c.clientProtocolFallback
		state.CipherSuite = c.cipherSuite
		state.PeerCertificates = c.peerCertificates
		state.VerifiedChains = c.verifiedChains
		state.SignedCertificateTimestamps = c.scts
		state.OCSPResponse = c.ocspResponse
		if c.verifiedDc != nil {
			state.DelegatedCredential = c.verifiedDc.raw
		}
		state.HandshakeConfirmed = atomic.LoadInt32(&c.handshakeConfirmed) == 1
		if !state.HandshakeConfirmed {
			state.Unique0RTTToken = c.binder
		}
		if !c.didResume {
			if c.clientFinishedIsFirst {
				state.TLSUnique = c.clientFinished[:]
			} else {
				state.TLSUnique = c.serverFinished[:]
			}
		}
	}

	return state
}

// OCSPResponse returns the stapled OCSP response from the TLS server, if
// any. (Only valid for client connections.)
func (c *Conn) OCSPResponse() []byte {
	c.handshakeMutex.Lock()
	defer c.handshakeMutex.Unlock()

	return c.ocspResponse
}

// VerifyHostname checks that the peer certificate chain is valid for
// connecting to host. If so, it returns nil; if not, it returns an error
// describing the problem.
func (c *Conn) VerifyHostname(host string) error {
	c.handshakeMutex.Lock()
	defer c.handshakeMutex.Unlock()
	if !c.isClient {
		return errors.New("tls: VerifyHostname called on TLS server connection")
	}
	if !c.handshakeComplete() {
		return errors.New("tls: handshake has not yet been performed")
	}
	if len(c.verifiedChains) == 0 {
		return errors.New("tls: handshake did not verify certificate chain")
	}
	return c.peerCertificates[0].VerifyHostname(host)
}

func (c *Conn) handshakeComplete() bool {
	return atomic.LoadUint32(&c.handshakeStatus) == 1
}
