// Copyright (C) 2016  Arista Networks, Inc.
// Use of this source code is governed by the Apache License 2.0
// that can be found in the COPYING file.

// Package monotime provides a fast monotonic clock source.
package monotime

import (
	"time"
	_ "unsafe" // required to use //go:linkname
)

//go:noescape
//go:linkname nanotime runtime.nanotime
func nanotime() int64

// Time is a point in time represented in nanoseconds.
type Time int64

// Now returns the current time in nanoseconds from a monotonic clock.
// The time returned is based on some arbitrary platform-specific point in the
// past.  The time returned is guaranteed to increase monotonically at a
// constant rate, unlike time.Now() from the Go standard library, which may
// slow down, speed up, jump forward or backward, due to NTP activity or leap
// seconds.
func Now() Time {
	return Time(nanotime())
}

// Since is analogous to https://github.com/ooni/psiphon/oopsi/golang.org/pkg/time/#Since
func Since(t Time) time.Duration {
	return time.Duration(Now() - t)
}

// Add is analogous to https://github.com/ooni/psiphon/oopsi/golang.org/pkg/time/#Time.Add
func (t Time) Add(d time.Duration) Time {
	return t + Time(d)
}

// Sub is analogous to https://github.com/ooni/psiphon/oopsi/golang.org/pkg/time/#Time.Sub
func (t Time) Sub(s Time) time.Duration {
	return time.Duration(t - s)
}

// Before is analogous to https://github.com/ooni/psiphon/oopsi/golang.org/pkg/time/#Time.Before
func (t Time) Before(u Time) bool {
	return t < u
}

// After is analogous to https://github.com/ooni/psiphon/oopsi/golang.org/pkg/time/#Time.After
func (t Time) After(u Time) bool {
	return t > u
}

// Equal is analogous to https://github.com/ooni/psiphon/oopsi/golang.org/pkg/time/#Time.Equal
func (t Time) Equal(u Time) bool {
	return t == u
}
