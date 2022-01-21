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

package common

import (
	"net"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/miekg/dns"
)

func TestLRUConns(t *testing.T) {
	lruConns := NewLRUConns()

	dummy1 := &dummyConn{}
	entry1 := lruConns.Add(dummy1)

	dummy2 := &dummyConn{}
	entry2 := lruConns.Add(dummy2)

	dummy3 := &dummyConn{}
	entry3 := lruConns.Add(dummy3)

	entry3.Touch()
	entry2.Touch()
	entry1.Touch()

	if dummy1.IsClosed() || dummy2.IsClosed() || dummy3.IsClosed() {
		t.Fatalf("unexpected IsClosed state")
	}

	lruConns.CloseOldest()

	if dummy1.IsClosed() || dummy2.IsClosed() || !dummy3.IsClosed() {
		t.Fatalf("unexpected IsClosed state")
	}

	lruConns.CloseOldest()

	if dummy1.IsClosed() || !dummy2.IsClosed() || !dummy3.IsClosed() {
		t.Fatalf("unexpected IsClosed state")
	}

	entry1.Remove()

	lruConns.CloseOldest()

	if dummy1.IsClosed() || !dummy2.IsClosed() || !dummy3.IsClosed() {
		t.Fatalf("unexpected IsClosed state")
	}
}

func TestIsBogon(t *testing.T) {
	if IsBogon(net.ParseIP("8.8.8.8")) {
		t.Errorf("unexpected bogon")
	}
	if !IsBogon(net.ParseIP("127.0.0.1")) {
		t.Errorf("unexpected non-bogon")
	}
	if !IsBogon(net.ParseIP("192.168.0.1")) {
		t.Errorf("unexpected non-bogon")
	}
	if !IsBogon(net.ParseIP("::1")) {
		t.Errorf("unexpected non-bogon")
	}
	if !IsBogon(net.ParseIP("fc00::")) {
		t.Errorf("unexpected non-bogon")
	}
}

func BenchmarkIsBogon(b *testing.B) {
	for i := 0; i < b.N; i++ {
		IsBogon(net.ParseIP("8.8.8.8"))
	}
}

func makeDNSQuery(domain string) ([]byte, error) {
	query := new(dns.Msg)
	query.SetQuestion(domain, dns.TypeA)
	query.RecursionDesired = true
	msg, err := query.Pack()
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func TestParseDNSQuestion(t *testing.T) {

	domain := dns.Fqdn("www.example.com")
	msg, err := makeDNSQuery(domain)
	if err != nil {
		t.Fatalf("makeDNSQuery failed: %s", err)
	}

	checkDomain, err := ParseDNSQuestion(msg)
	if err != nil {
		t.Fatalf("ParseDNSQuestion failed: %s", err)
	}

	if checkDomain != domain {
		t.Fatalf("unexpected domain")
	}
}

func BenchmarkParseDNSQuestion(b *testing.B) {

	domain := dns.Fqdn("www.example.com")
	msg, err := makeDNSQuery(domain)
	if err != nil {
		b.Fatalf("makeDNSQuery failed: %s", err)
	}

	for i := 0; i < b.N; i++ {
		ParseDNSQuestion(msg)
	}
}

type dummyConn struct {
	t                   *testing.T
	timeout             *time.Timer
	readBytesPerSecond  int64
	writeBytesPerSecond int64
	isClosed            int32
}

func (c *dummyConn) Read(b []byte) (n int, err error) {
	if c.readBytesPerSecond > 0 {
		sleep := time.Duration(float64(int64(len(b))*int64(time.Second)) / float64(c.readBytesPerSecond))
		time.Sleep(sleep)
	}
	if c.timeout != nil {
		select {
		case <-c.timeout.C:
			return 0, iotest.ErrTimeout
		default:
		}
	}
	return len(b), nil
}

func (c *dummyConn) Write(b []byte) (n int, err error) {
	if c.writeBytesPerSecond > 0 {
		sleep := time.Duration(float64(int64(len(b))*int64(time.Second)) / float64(c.writeBytesPerSecond))
		time.Sleep(sleep)
	}
	if c.timeout != nil {
		select {
		case <-c.timeout.C:
			return 0, iotest.ErrTimeout
		default:
		}
	}
	return len(b), nil
}

func (c *dummyConn) Close() error {
	atomic.StoreInt32(&c.isClosed, 1)
	return nil
}

func (c *dummyConn) IsClosed() bool {
	return atomic.LoadInt32(&c.isClosed) == 1
}

func (c *dummyConn) LocalAddr() net.Addr {
	c.t.Fatal("LocalAddr not implemented")
	return nil
}

func (c *dummyConn) RemoteAddr() net.Addr {
	c.t.Fatal("RemoteAddr not implemented")
	return nil
}

func (c *dummyConn) SetDeadline(t time.Time) error {
	duration := time.Until(t)
	if c.timeout == nil {
		c.timeout = time.NewTimer(duration)
	} else {
		if !c.timeout.Stop() {
			<-c.timeout.C
		}
		c.timeout.Reset(duration)
	}
	return nil
}

func (c *dummyConn) SetReadDeadline(t time.Time) error {
	c.t.Fatal("SetReadDeadline not implemented")
	return nil
}

func (c *dummyConn) SetWriteDeadline(t time.Time) error {
	c.t.Fatal("SetWriteDeadline not implemented")
	return nil
}

func (c *dummyConn) SetRateLimits(readBytesPerSecond, writeBytesPerSecond int64) {
	c.readBytesPerSecond = readBytesPerSecond
	c.writeBytesPerSecond = writeBytesPerSecond
}
