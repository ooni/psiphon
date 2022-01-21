package utils

import "github.com/ooni/psiphon/tunnel-core/oovendor/quic-go/internal/protocol"

// PacketInterval is an interval from one PacketNumber to the other
type PacketInterval struct {
	Start protocol.PacketNumber
	End   protocol.PacketNumber
}
