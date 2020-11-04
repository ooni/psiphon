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

package psiphon

import (
	"syscall"

	"github.com/ooni/psiphon/oopsi/github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common/errors"
	"github.com/ooni/psiphon/oopsi/golang.org/x/net/bpf"
)

func ClientBPFEnabled() bool {
	return false
}

func setSocketBPF(_ []bpf.RawInstruction, _ int) error {
	return errors.TraceNew("BPF not supported")
}

func setAdditionalSocketOptions(socketFd int) {
	syscall.SetsockoptInt(socketFd, syscall.SOL_SOCKET, syscall.SO_NOSIGPIPE, 1)
}
