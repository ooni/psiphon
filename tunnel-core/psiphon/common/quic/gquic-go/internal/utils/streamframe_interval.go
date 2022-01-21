package utils

import "github.com/ooni/psiphon/tunnel-core/psiphon/common/quic/gquic-go/internal/protocol"

// ByteInterval is an interval from one ByteCount to the other
type ByteInterval struct {
	Start protocol.ByteCount
	End   protocol.ByteCount
}
