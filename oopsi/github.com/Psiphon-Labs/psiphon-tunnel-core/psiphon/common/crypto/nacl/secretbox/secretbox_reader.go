/*
 * Copyright (c) 2017, Psiphon Inc.
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

// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package secretbox // import "github.com/ooni/psiphon/oopsi/github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common/crypto/nacl/secretbox"

import (
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/ooni/psiphon/oopsi/golang.org/x/crypto/poly1305"
	"github.com/ooni/psiphon/oopsi/golang.org/x/crypto/salsa20/salsa"
)

// NewOpenReadSeeker is a streaming variant of Open.
//
// NewOpenReadSeeker is intended only for use in Psiphon with a payload that is
// independently authenticated; and consideration has been given only for client-side
// operation. Non-optimized reference implementation poly1305 and salsa20 code is used.
//
// The box is accessed through an io.ReadSeeker, which allows for an initial
// poly1305 verification pass followed by a payload decryption pass, both
// without loading the entire box into memory. As such, this implementation
// should not be subject to the use-before-authentication or truncation attacks
// discussed here:
// https://github.com/ooni/psiphon/oopsi/github.com/golang/crypto/commit/9ba3862cf6a5452ae579de98f9364dd2e544844c#diff-9a969aca62172940631ad143523794ee
// https://github.com/ooni/psiphon/oopsi/github.com/golang/go/issues/17673#issuecomment-275732868
func NewOpenReadSeeker(box io.ReadSeeker, nonce *[24]byte, key *[32]byte) (io.ReadSeeker, error) {

	r := &salsa20ReadSeeker{
		box:   box,
		nonce: *nonce,
		key:   *key,
	}

	err := r.reset()
	if err != nil {
		return nil, err
	}

	return r, nil
}

type salsa20ReadSeeker struct {
	box         io.ReadSeeker
	nonce       [24]byte
	key         [32]byte
	subKey      [32]byte
	counter     [16]byte
	block       [64]byte
	blockOffset int
}

// Open x/crypto/nacl/secretbox/secretbox.go, adapted to streaming and rewinding.
func (r *salsa20ReadSeeker) reset() error {

	// See comments in Open

	_, err := r.box.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("initial seek failed: %s", err)
	}

	var tag [poly1305.TagSize]byte

	_, err = io.ReadFull(r.box, tag[:])
	if err != nil {
		return fmt.Errorf("read tag failed: %s", err)
	}

	var subKey [32]byte
	var counter [16]byte
	setup(&subKey, &counter, &r.nonce, &r.key)

	// The Poly1305 key is generated by encrypting 32 bytes of zeros. Since
	// Salsa20 works with 64-byte blocks, we also generate 32 bytes of
	// keystream as a side effect.
	var firstBlock [64]byte
	salsa.XORKeyStream(firstBlock[:], firstBlock[:], &counter, &subKey)

	var poly1305Key [32]byte
	copy(poly1305Key[:], firstBlock[:])

	err = poly1305VerifyReader(&tag, r.box, &poly1305Key)
	if err != nil {
		return err
	}

	_, err = r.box.Seek(int64(len(tag)), io.SeekStart)
	if err != nil {
		return fmt.Errorf("rewind seek failed: %s", err)
	}

	counter[8] = 1

	r.subKey = subKey
	r.counter = counter

	// We XOR up to 32 bytes of box with the keystream generated from
	// the first block.

	r.block = firstBlock
	r.blockOffset = 32

	return nil
}

func (r *salsa20ReadSeeker) Read(p []byte) (int, error) {

	n, err := r.box.Read(p)

	for i := 0; i < n; i++ {
		if r.blockOffset == 64 {
			salsa20Core(&r.block, &r.counter, &r.subKey, &salsa.Sigma)

			u := uint32(1)
			for i := 8; i < 16; i++ {
				u += uint32(r.counter[i])
				r.counter[i] = byte(u)
				u >>= 8
			}
			r.blockOffset = 0
		}
		p[i] = p[i] ^ r.block[r.blockOffset]
		r.blockOffset++
	}

	return n, err
}

func (r *salsa20ReadSeeker) Seek(offset int64, whence int) (int64, error) {

	// Currently only supports Seek(0, io.SeekStart) as required for Psiphon.

	if offset != 0 || whence != io.SeekStart {
		return -1, fmt.Errorf("unsupported")
	}

	// TODO: could skip poly1305 verify after 1st reset.

	err := r.reset()
	if err != nil {
		return -1, err
	}

	return 0, nil
}

// Verify from crypto/poly1305/poly1305.go, modifed to use an io.Reader.
func poly1305VerifyReader(mac *[16]byte, m io.Reader, key *[32]byte) error {
	var tmp [16]byte
	err := poly1305SumReader(&tmp, m, key)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(tmp[:], mac[:]) != 1 {
		return fmt.Errorf("verify failed")
	}
	return nil
}

// Sum from crypto/poly1305/sum_ref.go, modifed to use an io.Reader.
func poly1305SumReader(out *[poly1305.TagSize]byte, msg io.Reader, key *[32]byte) error {
	var (
		h0, h1, h2, h3, h4 uint32 // the hash accumulators
		r0, r1, r2, r3, r4 uint64 // the r part of the key
	)

	r0 = uint64(binary.LittleEndian.Uint32(key[0:]) & 0x3ffffff)
	r1 = uint64((binary.LittleEndian.Uint32(key[3:]) >> 2) & 0x3ffff03)
	r2 = uint64((binary.LittleEndian.Uint32(key[6:]) >> 4) & 0x3ffc0ff)
	r3 = uint64((binary.LittleEndian.Uint32(key[9:]) >> 6) & 0x3f03fff)
	r4 = uint64((binary.LittleEndian.Uint32(key[12:]) >> 8) & 0x00fffff)

	R1, R2, R3, R4 := r1*5, r2*5, r3*5, r4*5

	var in [poly1305.TagSize]byte

	for {
		n, err := msg.Read(in[:])

		if n == poly1305.TagSize {

			// h += msg
			h0 += binary.LittleEndian.Uint32(in[0:]) & 0x3ffffff
			h1 += (binary.LittleEndian.Uint32(in[3:]) >> 2) & 0x3ffffff
			h2 += (binary.LittleEndian.Uint32(in[6:]) >> 4) & 0x3ffffff
			h3 += (binary.LittleEndian.Uint32(in[9:]) >> 6) & 0x3ffffff
			h4 += (binary.LittleEndian.Uint32(in[12:]) >> 8) | (1 << 24)

		} else if n > 0 {

			in[n] = 0x01
			for i := n + 1; i < poly1305.TagSize; i++ {
				in[i] = 0
			}

			// h += msg
			h0 += binary.LittleEndian.Uint32(in[0:]) & 0x3ffffff
			h1 += (binary.LittleEndian.Uint32(in[3:]) >> 2) & 0x3ffffff
			h2 += (binary.LittleEndian.Uint32(in[6:]) >> 4) & 0x3ffffff
			h3 += (binary.LittleEndian.Uint32(in[9:]) >> 6) & 0x3ffffff
			h4 += (binary.LittleEndian.Uint32(in[12:]) >> 8)
		}

		if n > 0 {

			// h *= r
			d0 := (uint64(h0) * r0) + (uint64(h1) * R4) + (uint64(h2) * R3) + (uint64(h3) * R2) + (uint64(h4) * R1)
			d1 := (d0 >> 26) + (uint64(h0) * r1) + (uint64(h1) * r0) + (uint64(h2) * R4) + (uint64(h3) * R3) + (uint64(h4) * R2)
			d2 := (d1 >> 26) + (uint64(h0) * r2) + (uint64(h1) * r1) + (uint64(h2) * r0) + (uint64(h3) * R4) + (uint64(h4) * R3)
			d3 := (d2 >> 26) + (uint64(h0) * r3) + (uint64(h1) * r2) + (uint64(h2) * r1) + (uint64(h3) * r0) + (uint64(h4) * R4)
			d4 := (d3 >> 26) + (uint64(h0) * r4) + (uint64(h1) * r3) + (uint64(h2) * r2) + (uint64(h3) * r1) + (uint64(h4) * r0)

			// h %= p
			h0 = uint32(d0) & 0x3ffffff
			h1 = uint32(d1) & 0x3ffffff
			h2 = uint32(d2) & 0x3ffffff
			h3 = uint32(d3) & 0x3ffffff
			h4 = uint32(d4) & 0x3ffffff

			h0 += uint32(d4>>26) * 5
			h1 += h0 >> 26
			h0 = h0 & 0x3ffffff
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}
	}

	// h %= p reduction
	h2 += h1 >> 26
	h1 &= 0x3ffffff
	h3 += h2 >> 26
	h2 &= 0x3ffffff
	h4 += h3 >> 26
	h3 &= 0x3ffffff
	h0 += 5 * (h4 >> 26)
	h4 &= 0x3ffffff
	h1 += h0 >> 26
	h0 &= 0x3ffffff

	// h - p
	t0 := h0 + 5
	t1 := h1 + (t0 >> 26)
	t2 := h2 + (t1 >> 26)
	t3 := h3 + (t2 >> 26)
	t4 := h4 + (t3 >> 26) - (1 << 26)
	t0 &= 0x3ffffff
	t1 &= 0x3ffffff
	t2 &= 0x3ffffff
	t3 &= 0x3ffffff

	// select h if h < p else h - p
	t_mask := (t4 >> 31) - 1
	h_mask := ^t_mask
	h0 = (h0 & h_mask) | (t0 & t_mask)
	h1 = (h1 & h_mask) | (t1 & t_mask)
	h2 = (h2 & h_mask) | (t2 & t_mask)
	h3 = (h3 & h_mask) | (t3 & t_mask)
	h4 = (h4 & h_mask) | (t4 & t_mask)

	// h %= 2^128
	h0 |= h1 << 26
	h1 = ((h1 >> 6) | (h2 << 20))
	h2 = ((h2 >> 12) | (h3 << 14))
	h3 = ((h3 >> 18) | (h4 << 8))

	// s: the s part of the key
	// tag = (h + s) % (2^128)
	t := uint64(h0) + uint64(binary.LittleEndian.Uint32(key[16:]))
	h0 = uint32(t)
	t = uint64(h1) + uint64(binary.LittleEndian.Uint32(key[20:])) + (t >> 32)
	h1 = uint32(t)
	t = uint64(h2) + uint64(binary.LittleEndian.Uint32(key[24:])) + (t >> 32)
	h2 = uint32(t)
	t = uint64(h3) + uint64(binary.LittleEndian.Uint32(key[28:])) + (t >> 32)
	h3 = uint32(t)

	binary.LittleEndian.PutUint32(out[0:], h0)
	binary.LittleEndian.PutUint32(out[4:], h1)
	binary.LittleEndian.PutUint32(out[8:], h2)
	binary.LittleEndian.PutUint32(out[12:], h3)

	return nil
}

// core from x/crypto/salsa20/salsa/salsa20_ref.go.
func salsa20Core(out *[64]byte, in *[16]byte, k *[32]byte, c *[16]byte) {
	j0 := uint32(c[0]) | uint32(c[1])<<8 | uint32(c[2])<<16 | uint32(c[3])<<24
	j1 := uint32(k[0]) | uint32(k[1])<<8 | uint32(k[2])<<16 | uint32(k[3])<<24
	j2 := uint32(k[4]) | uint32(k[5])<<8 | uint32(k[6])<<16 | uint32(k[7])<<24
	j3 := uint32(k[8]) | uint32(k[9])<<8 | uint32(k[10])<<16 | uint32(k[11])<<24
	j4 := uint32(k[12]) | uint32(k[13])<<8 | uint32(k[14])<<16 | uint32(k[15])<<24
	j5 := uint32(c[4]) | uint32(c[5])<<8 | uint32(c[6])<<16 | uint32(c[7])<<24
	j6 := uint32(in[0]) | uint32(in[1])<<8 | uint32(in[2])<<16 | uint32(in[3])<<24
	j7 := uint32(in[4]) | uint32(in[5])<<8 | uint32(in[6])<<16 | uint32(in[7])<<24
	j8 := uint32(in[8]) | uint32(in[9])<<8 | uint32(in[10])<<16 | uint32(in[11])<<24
	j9 := uint32(in[12]) | uint32(in[13])<<8 | uint32(in[14])<<16 | uint32(in[15])<<24
	j10 := uint32(c[8]) | uint32(c[9])<<8 | uint32(c[10])<<16 | uint32(c[11])<<24
	j11 := uint32(k[16]) | uint32(k[17])<<8 | uint32(k[18])<<16 | uint32(k[19])<<24
	j12 := uint32(k[20]) | uint32(k[21])<<8 | uint32(k[22])<<16 | uint32(k[23])<<24
	j13 := uint32(k[24]) | uint32(k[25])<<8 | uint32(k[26])<<16 | uint32(k[27])<<24
	j14 := uint32(k[28]) | uint32(k[29])<<8 | uint32(k[30])<<16 | uint32(k[31])<<24
	j15 := uint32(c[12]) | uint32(c[13])<<8 | uint32(c[14])<<16 | uint32(c[15])<<24

	x0, x1, x2, x3, x4, x5, x6, x7, x8 := j0, j1, j2, j3, j4, j5, j6, j7, j8
	x9, x10, x11, x12, x13, x14, x15 := j9, j10, j11, j12, j13, j14, j15

	const rounds = 20

	for i := 0; i < rounds; i += 2 {
		u := x0 + x12
		x4 ^= u<<7 | u>>(32-7)
		u = x4 + x0
		x8 ^= u<<9 | u>>(32-9)
		u = x8 + x4
		x12 ^= u<<13 | u>>(32-13)
		u = x12 + x8
		x0 ^= u<<18 | u>>(32-18)

		u = x5 + x1
		x9 ^= u<<7 | u>>(32-7)
		u = x9 + x5
		x13 ^= u<<9 | u>>(32-9)
		u = x13 + x9
		x1 ^= u<<13 | u>>(32-13)
		u = x1 + x13
		x5 ^= u<<18 | u>>(32-18)

		u = x10 + x6
		x14 ^= u<<7 | u>>(32-7)
		u = x14 + x10
		x2 ^= u<<9 | u>>(32-9)
		u = x2 + x14
		x6 ^= u<<13 | u>>(32-13)
		u = x6 + x2
		x10 ^= u<<18 | u>>(32-18)

		u = x15 + x11
		x3 ^= u<<7 | u>>(32-7)
		u = x3 + x15
		x7 ^= u<<9 | u>>(32-9)
		u = x7 + x3
		x11 ^= u<<13 | u>>(32-13)
		u = x11 + x7
		x15 ^= u<<18 | u>>(32-18)

		u = x0 + x3
		x1 ^= u<<7 | u>>(32-7)
		u = x1 + x0
		x2 ^= u<<9 | u>>(32-9)
		u = x2 + x1
		x3 ^= u<<13 | u>>(32-13)
		u = x3 + x2
		x0 ^= u<<18 | u>>(32-18)

		u = x5 + x4
		x6 ^= u<<7 | u>>(32-7)
		u = x6 + x5
		x7 ^= u<<9 | u>>(32-9)
		u = x7 + x6
		x4 ^= u<<13 | u>>(32-13)
		u = x4 + x7
		x5 ^= u<<18 | u>>(32-18)

		u = x10 + x9
		x11 ^= u<<7 | u>>(32-7)
		u = x11 + x10
		x8 ^= u<<9 | u>>(32-9)
		u = x8 + x11
		x9 ^= u<<13 | u>>(32-13)
		u = x9 + x8
		x10 ^= u<<18 | u>>(32-18)

		u = x15 + x14
		x12 ^= u<<7 | u>>(32-7)
		u = x12 + x15
		x13 ^= u<<9 | u>>(32-9)
		u = x13 + x12
		x14 ^= u<<13 | u>>(32-13)
		u = x14 + x13
		x15 ^= u<<18 | u>>(32-18)
	}
	x0 += j0
	x1 += j1
	x2 += j2
	x3 += j3
	x4 += j4
	x5 += j5
	x6 += j6
	x7 += j7
	x8 += j8
	x9 += j9
	x10 += j10
	x11 += j11
	x12 += j12
	x13 += j13
	x14 += j14
	x15 += j15

	out[0] = byte(x0)
	out[1] = byte(x0 >> 8)
	out[2] = byte(x0 >> 16)
	out[3] = byte(x0 >> 24)

	out[4] = byte(x1)
	out[5] = byte(x1 >> 8)
	out[6] = byte(x1 >> 16)
	out[7] = byte(x1 >> 24)

	out[8] = byte(x2)
	out[9] = byte(x2 >> 8)
	out[10] = byte(x2 >> 16)
	out[11] = byte(x2 >> 24)

	out[12] = byte(x3)
	out[13] = byte(x3 >> 8)
	out[14] = byte(x3 >> 16)
	out[15] = byte(x3 >> 24)

	out[16] = byte(x4)
	out[17] = byte(x4 >> 8)
	out[18] = byte(x4 >> 16)
	out[19] = byte(x4 >> 24)

	out[20] = byte(x5)
	out[21] = byte(x5 >> 8)
	out[22] = byte(x5 >> 16)
	out[23] = byte(x5 >> 24)

	out[24] = byte(x6)
	out[25] = byte(x6 >> 8)
	out[26] = byte(x6 >> 16)
	out[27] = byte(x6 >> 24)

	out[28] = byte(x7)
	out[29] = byte(x7 >> 8)
	out[30] = byte(x7 >> 16)
	out[31] = byte(x7 >> 24)

	out[32] = byte(x8)
	out[33] = byte(x8 >> 8)
	out[34] = byte(x8 >> 16)
	out[35] = byte(x8 >> 24)

	out[36] = byte(x9)
	out[37] = byte(x9 >> 8)
	out[38] = byte(x9 >> 16)
	out[39] = byte(x9 >> 24)

	out[40] = byte(x10)
	out[41] = byte(x10 >> 8)
	out[42] = byte(x10 >> 16)
	out[43] = byte(x10 >> 24)

	out[44] = byte(x11)
	out[45] = byte(x11 >> 8)
	out[46] = byte(x11 >> 16)
	out[47] = byte(x11 >> 24)

	out[48] = byte(x12)
	out[49] = byte(x12 >> 8)
	out[50] = byte(x12 >> 16)
	out[51] = byte(x12 >> 24)

	out[52] = byte(x13)
	out[53] = byte(x13 >> 8)
	out[54] = byte(x13 >> 16)
	out[55] = byte(x13 >> 24)

	out[56] = byte(x14)
	out[57] = byte(x14 >> 8)
	out[58] = byte(x14 >> 16)
	out[59] = byte(x14 >> 24)

	out[60] = byte(x15)
	out[61] = byte(x15 >> 8)
	out[62] = byte(x15 >> 16)
	out[63] = byte(x15 >> 24)
}
