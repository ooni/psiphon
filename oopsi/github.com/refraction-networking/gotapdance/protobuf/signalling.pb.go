// Code generated by protoc-gen-go. DO NOT EDIT.
// source: signalling.proto

package tdproto

import (
	fmt "fmt"
	proto "github.com/ooni/psiphon/oopsi/github.com/golang/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type KeyType int32

const (
	KeyType_AES_GCM_128 KeyType = 90
	KeyType_AES_GCM_256 KeyType = 91
)

var KeyType_name = map[int32]string{
	90: "AES_GCM_128",
	91: "AES_GCM_256",
}

var KeyType_value = map[string]int32{
	"AES_GCM_128": 90,
	"AES_GCM_256": 91,
}

func (x KeyType) Enum() *KeyType {
	p := new(KeyType)
	*p = x
	return p
}

func (x KeyType) String() string {
	return proto.EnumName(KeyType_name, int32(x))
}

func (x *KeyType) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(KeyType_value, data, "KeyType")
	if err != nil {
		return err
	}
	*x = KeyType(value)
	return nil
}

func (KeyType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_39f66308029891ad, []int{0}
}

// State transitions of the client
type C2S_Transition int32

const (
	C2S_Transition_C2S_NO_CHANGE                C2S_Transition = 0
	C2S_Transition_C2S_SESSION_INIT             C2S_Transition = 1
	C2S_Transition_C2S_SESSION_COVERT_INIT      C2S_Transition = 11
	C2S_Transition_C2S_EXPECT_RECONNECT         C2S_Transition = 2
	C2S_Transition_C2S_SESSION_CLOSE            C2S_Transition = 3
	C2S_Transition_C2S_YIELD_UPLOAD             C2S_Transition = 4
	C2S_Transition_C2S_ACQUIRE_UPLOAD           C2S_Transition = 5
	C2S_Transition_C2S_EXPECT_UPLOADONLY_RECONN C2S_Transition = 6
	C2S_Transition_C2S_ERROR                    C2S_Transition = 255
)

var C2S_Transition_name = map[int32]string{
	0:   "C2S_NO_CHANGE",
	1:   "C2S_SESSION_INIT",
	11:  "C2S_SESSION_COVERT_INIT",
	2:   "C2S_EXPECT_RECONNECT",
	3:   "C2S_SESSION_CLOSE",
	4:   "C2S_YIELD_UPLOAD",
	5:   "C2S_ACQUIRE_UPLOAD",
	6:   "C2S_EXPECT_UPLOADONLY_RECONN",
	255: "C2S_ERROR",
}

var C2S_Transition_value = map[string]int32{
	"C2S_NO_CHANGE":                0,
	"C2S_SESSION_INIT":             1,
	"C2S_SESSION_COVERT_INIT":      11,
	"C2S_EXPECT_RECONNECT":         2,
	"C2S_SESSION_CLOSE":            3,
	"C2S_YIELD_UPLOAD":             4,
	"C2S_ACQUIRE_UPLOAD":           5,
	"C2S_EXPECT_UPLOADONLY_RECONN": 6,
	"C2S_ERROR":                    255,
}

func (x C2S_Transition) Enum() *C2S_Transition {
	p := new(C2S_Transition)
	*p = x
	return p
}

func (x C2S_Transition) String() string {
	return proto.EnumName(C2S_Transition_name, int32(x))
}

func (x *C2S_Transition) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(C2S_Transition_value, data, "C2S_Transition")
	if err != nil {
		return err
	}
	*x = C2S_Transition(value)
	return nil
}

func (C2S_Transition) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_39f66308029891ad, []int{1}
}

// State transitions of the server
type S2C_Transition int32

const (
	S2C_Transition_S2C_NO_CHANGE           S2C_Transition = 0
	S2C_Transition_S2C_SESSION_INIT        S2C_Transition = 1
	S2C_Transition_S2C_SESSION_COVERT_INIT S2C_Transition = 11
	S2C_Transition_S2C_CONFIRM_RECONNECT   S2C_Transition = 2
	S2C_Transition_S2C_SESSION_CLOSE       S2C_Transition = 3
	// TODO should probably also allow EXPECT_RECONNECT here, for DittoTap
	S2C_Transition_S2C_ERROR S2C_Transition = 255
)

var S2C_Transition_name = map[int32]string{
	0:   "S2C_NO_CHANGE",
	1:   "S2C_SESSION_INIT",
	11:  "S2C_SESSION_COVERT_INIT",
	2:   "S2C_CONFIRM_RECONNECT",
	3:   "S2C_SESSION_CLOSE",
	255: "S2C_ERROR",
}

var S2C_Transition_value = map[string]int32{
	"S2C_NO_CHANGE":           0,
	"S2C_SESSION_INIT":        1,
	"S2C_SESSION_COVERT_INIT": 11,
	"S2C_CONFIRM_RECONNECT":   2,
	"S2C_SESSION_CLOSE":       3,
	"S2C_ERROR":               255,
}

func (x S2C_Transition) Enum() *S2C_Transition {
	p := new(S2C_Transition)
	*p = x
	return p
}

func (x S2C_Transition) String() string {
	return proto.EnumName(S2C_Transition_name, int32(x))
}

func (x *S2C_Transition) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(S2C_Transition_value, data, "S2C_Transition")
	if err != nil {
		return err
	}
	*x = S2C_Transition(value)
	return nil
}

func (S2C_Transition) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_39f66308029891ad, []int{2}
}

// Should accompany all S2C_ERROR messages.
type ErrorReasonS2C int32

const (
	ErrorReasonS2C_NO_ERROR         ErrorReasonS2C = 0
	ErrorReasonS2C_COVERT_STREAM    ErrorReasonS2C = 1
	ErrorReasonS2C_CLIENT_REPORTED  ErrorReasonS2C = 2
	ErrorReasonS2C_CLIENT_PROTOCOL  ErrorReasonS2C = 3
	ErrorReasonS2C_STATION_INTERNAL ErrorReasonS2C = 4
	ErrorReasonS2C_DECOY_OVERLOAD   ErrorReasonS2C = 5
	ErrorReasonS2C_CLIENT_STREAM    ErrorReasonS2C = 100
	ErrorReasonS2C_CLIENT_TIMEOUT   ErrorReasonS2C = 101
)

var ErrorReasonS2C_name = map[int32]string{
	0:   "NO_ERROR",
	1:   "COVERT_STREAM",
	2:   "CLIENT_REPORTED",
	3:   "CLIENT_PROTOCOL",
	4:   "STATION_INTERNAL",
	5:   "DECOY_OVERLOAD",
	100: "CLIENT_STREAM",
	101: "CLIENT_TIMEOUT",
}

var ErrorReasonS2C_value = map[string]int32{
	"NO_ERROR":         0,
	"COVERT_STREAM":    1,
	"CLIENT_REPORTED":  2,
	"CLIENT_PROTOCOL":  3,
	"STATION_INTERNAL": 4,
	"DECOY_OVERLOAD":   5,
	"CLIENT_STREAM":    100,
	"CLIENT_TIMEOUT":   101,
}

func (x ErrorReasonS2C) Enum() *ErrorReasonS2C {
	p := new(ErrorReasonS2C)
	*p = x
	return p
}

func (x ErrorReasonS2C) String() string {
	return proto.EnumName(ErrorReasonS2C_name, int32(x))
}

func (x *ErrorReasonS2C) UnmarshalJSON(data []byte) error {
	value, err := proto.UnmarshalJSONEnum(ErrorReasonS2C_value, data, "ErrorReasonS2C")
	if err != nil {
		return err
	}
	*x = ErrorReasonS2C(value)
	return nil
}

func (ErrorReasonS2C) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_39f66308029891ad, []int{3}
}

type PubKey struct {
	// A public key, as used by the station.
	Key                  []byte   `protobuf:"bytes,1,opt,name=key" json:"key,omitempty"`
	Type                 *KeyType `protobuf:"varint,2,opt,name=type,enum=tapdance.KeyType" json:"type,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *PubKey) Reset()         { *m = PubKey{} }
func (m *PubKey) String() string { return proto.CompactTextString(m) }
func (*PubKey) ProtoMessage()    {}
func (*PubKey) Descriptor() ([]byte, []int) {
	return fileDescriptor_39f66308029891ad, []int{0}
}

func (m *PubKey) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_PubKey.Unmarshal(m, b)
}
func (m *PubKey) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_PubKey.Marshal(b, m, deterministic)
}
func (m *PubKey) XXX_Merge(src proto.Message) {
	xxx_messageInfo_PubKey.Merge(m, src)
}
func (m *PubKey) XXX_Size() int {
	return xxx_messageInfo_PubKey.Size(m)
}
func (m *PubKey) XXX_DiscardUnknown() {
	xxx_messageInfo_PubKey.DiscardUnknown(m)
}

var xxx_messageInfo_PubKey proto.InternalMessageInfo

func (m *PubKey) GetKey() []byte {
	if m != nil {
		return m.Key
	}
	return nil
}

func (m *PubKey) GetType() KeyType {
	if m != nil && m.Type != nil {
		return *m.Type
	}
	return KeyType_AES_GCM_128
}

type TLSDecoySpec struct {
	// The hostname/SNI to use for this host
	//
	// The hostname is the only required field, although other
	// fields are expected to be present in most cases.
	Hostname *string `protobuf:"bytes,1,opt,name=hostname" json:"hostname,omitempty"`
	// The 32-bit ipv4 address, in network byte order
	//
	// If the IPv4 address is absent, then it may be resolved via
	// DNS by the client, or the client may discard this decoy spec
	// if local DNS is untrusted, or the service may be multihomed.
	Ipv4Addr *uint32 `protobuf:"fixed32,2,opt,name=ipv4addr" json:"ipv4addr,omitempty"`
	// The 128-bit ipv6 address, in network byte order
	Ipv6Addr []byte `protobuf:"bytes,6,opt,name=ipv6addr" json:"ipv6addr,omitempty"`
	// The Tapdance station public key to use when contacting this
	// decoy
	//
	// If omitted, the default station public key (if any) is used.
	Pubkey *PubKey `protobuf:"bytes,3,opt,name=pubkey" json:"pubkey,omitempty"`
	// The maximum duration, in milliseconds, to maintain an open
	// connection to this decoy (because the decoy may close the
	// connection itself after this length of time)
	//
	// If omitted, a default of 30,000 milliseconds is assumed.
	Timeout *uint32 `protobuf:"varint,4,opt,name=timeout" json:"timeout,omitempty"`
	// The maximum TCP window size to attempt to use for this decoy.
	//
	// If omitted, a default of 15360 is assumed.
	//
	// TODO: the default is based on the current heuristic of only
	// using decoys that permit windows of 15KB or larger.  If this
	// heuristic changes, then this default doesn't make sense.
	Tcpwin               *uint32  `protobuf:"varint,5,opt,name=tcpwin" json:"tcpwin,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *TLSDecoySpec) Reset()         { *m = TLSDecoySpec{} }
func (m *TLSDecoySpec) String() string { return proto.CompactTextString(m) }
func (*TLSDecoySpec) ProtoMessage()    {}
func (*TLSDecoySpec) Descriptor() ([]byte, []int) {
	return fileDescriptor_39f66308029891ad, []int{1}
}

func (m *TLSDecoySpec) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_TLSDecoySpec.Unmarshal(m, b)
}
func (m *TLSDecoySpec) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_TLSDecoySpec.Marshal(b, m, deterministic)
}
func (m *TLSDecoySpec) XXX_Merge(src proto.Message) {
	xxx_messageInfo_TLSDecoySpec.Merge(m, src)
}
func (m *TLSDecoySpec) XXX_Size() int {
	return xxx_messageInfo_TLSDecoySpec.Size(m)
}
func (m *TLSDecoySpec) XXX_DiscardUnknown() {
	xxx_messageInfo_TLSDecoySpec.DiscardUnknown(m)
}

var xxx_messageInfo_TLSDecoySpec proto.InternalMessageInfo

func (m *TLSDecoySpec) GetHostname() string {
	if m != nil && m.Hostname != nil {
		return *m.Hostname
	}
	return ""
}

func (m *TLSDecoySpec) GetIpv4Addr() uint32 {
	if m != nil && m.Ipv4Addr != nil {
		return *m.Ipv4Addr
	}
	return 0
}

func (m *TLSDecoySpec) GetIpv6Addr() []byte {
	if m != nil {
		return m.Ipv6Addr
	}
	return nil
}

func (m *TLSDecoySpec) GetPubkey() *PubKey {
	if m != nil {
		return m.Pubkey
	}
	return nil
}

func (m *TLSDecoySpec) GetTimeout() uint32 {
	if m != nil && m.Timeout != nil {
		return *m.Timeout
	}
	return 0
}

func (m *TLSDecoySpec) GetTcpwin() uint32 {
	if m != nil && m.Tcpwin != nil {
		return *m.Tcpwin
	}
	return 0
}

type ClientConf struct {
	DecoyList            *DecoyList `protobuf:"bytes,1,opt,name=decoy_list,json=decoyList" json:"decoy_list,omitempty"`
	Generation           *uint32    `protobuf:"varint,2,opt,name=generation" json:"generation,omitempty"`
	DefaultPubkey        *PubKey    `protobuf:"bytes,3,opt,name=default_pubkey,json=defaultPubkey" json:"default_pubkey,omitempty"`
	XXX_NoUnkeyedLiteral struct{}   `json:"-"`
	XXX_unrecognized     []byte     `json:"-"`
	XXX_sizecache        int32      `json:"-"`
}

func (m *ClientConf) Reset()         { *m = ClientConf{} }
func (m *ClientConf) String() string { return proto.CompactTextString(m) }
func (*ClientConf) ProtoMessage()    {}
func (*ClientConf) Descriptor() ([]byte, []int) {
	return fileDescriptor_39f66308029891ad, []int{2}
}

func (m *ClientConf) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ClientConf.Unmarshal(m, b)
}
func (m *ClientConf) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ClientConf.Marshal(b, m, deterministic)
}
func (m *ClientConf) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ClientConf.Merge(m, src)
}
func (m *ClientConf) XXX_Size() int {
	return xxx_messageInfo_ClientConf.Size(m)
}
func (m *ClientConf) XXX_DiscardUnknown() {
	xxx_messageInfo_ClientConf.DiscardUnknown(m)
}

var xxx_messageInfo_ClientConf proto.InternalMessageInfo

func (m *ClientConf) GetDecoyList() *DecoyList {
	if m != nil {
		return m.DecoyList
	}
	return nil
}

func (m *ClientConf) GetGeneration() uint32 {
	if m != nil && m.Generation != nil {
		return *m.Generation
	}
	return 0
}

func (m *ClientConf) GetDefaultPubkey() *PubKey {
	if m != nil {
		return m.DefaultPubkey
	}
	return nil
}

type DecoyList struct {
	TlsDecoys            []*TLSDecoySpec `protobuf:"bytes,1,rep,name=tls_decoys,json=tlsDecoys" json:"tls_decoys,omitempty"`
	XXX_NoUnkeyedLiteral struct{}        `json:"-"`
	XXX_unrecognized     []byte          `json:"-"`
	XXX_sizecache        int32           `json:"-"`
}

func (m *DecoyList) Reset()         { *m = DecoyList{} }
func (m *DecoyList) String() string { return proto.CompactTextString(m) }
func (*DecoyList) ProtoMessage()    {}
func (*DecoyList) Descriptor() ([]byte, []int) {
	return fileDescriptor_39f66308029891ad, []int{3}
}

func (m *DecoyList) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_DecoyList.Unmarshal(m, b)
}
func (m *DecoyList) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_DecoyList.Marshal(b, m, deterministic)
}
func (m *DecoyList) XXX_Merge(src proto.Message) {
	xxx_messageInfo_DecoyList.Merge(m, src)
}
func (m *DecoyList) XXX_Size() int {
	return xxx_messageInfo_DecoyList.Size(m)
}
func (m *DecoyList) XXX_DiscardUnknown() {
	xxx_messageInfo_DecoyList.DiscardUnknown(m)
}

var xxx_messageInfo_DecoyList proto.InternalMessageInfo

func (m *DecoyList) GetTlsDecoys() []*TLSDecoySpec {
	if m != nil {
		return m.TlsDecoys
	}
	return nil
}

type StationToClient struct {
	// Should accompany (at least) SESSION_INIT and CONFIRM_RECONNECT.
	ProtocolVersion *uint32 `protobuf:"varint,1,opt,name=protocol_version,json=protocolVersion" json:"protocol_version,omitempty"`
	// There might be a state transition. May be absent; absence should be
	// treated identically to NO_CHANGE.
	StateTransition *S2C_Transition `protobuf:"varint,2,opt,name=state_transition,json=stateTransition,enum=tapdance.S2C_Transition" json:"state_transition,omitempty"`
	// The station can send client config info piggybacked
	// on any message, as it sees fit
	ConfigInfo *ClientConf `protobuf:"bytes,3,opt,name=config_info,json=configInfo" json:"config_info,omitempty"`
	// If state_transition == S2C_ERROR, this field is the explanation.
	ErrReason *ErrorReasonS2C `protobuf:"varint,4,opt,name=err_reason,json=errReason,enum=tapdance.ErrorReasonS2C" json:"err_reason,omitempty"`
	// Signals client to stop connecting for following amount of seconds
	TmpBackoff *uint32 `protobuf:"varint,5,opt,name=tmp_backoff,json=tmpBackoff" json:"tmp_backoff,omitempty"`
	// Sent in SESSION_INIT, identifies the station that picked up
	StationId *string `protobuf:"bytes,6,opt,name=station_id,json=stationId" json:"station_id,omitempty"`
	// Random-sized junk to defeat packet size fingerprinting.
	Padding              []byte   `protobuf:"bytes,100,opt,name=padding" json:"padding,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *StationToClient) Reset()         { *m = StationToClient{} }
func (m *StationToClient) String() string { return proto.CompactTextString(m) }
func (*StationToClient) ProtoMessage()    {}
func (*StationToClient) Descriptor() ([]byte, []int) {
	return fileDescriptor_39f66308029891ad, []int{4}
}

func (m *StationToClient) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_StationToClient.Unmarshal(m, b)
}
func (m *StationToClient) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_StationToClient.Marshal(b, m, deterministic)
}
func (m *StationToClient) XXX_Merge(src proto.Message) {
	xxx_messageInfo_StationToClient.Merge(m, src)
}
func (m *StationToClient) XXX_Size() int {
	return xxx_messageInfo_StationToClient.Size(m)
}
func (m *StationToClient) XXX_DiscardUnknown() {
	xxx_messageInfo_StationToClient.DiscardUnknown(m)
}

var xxx_messageInfo_StationToClient proto.InternalMessageInfo

func (m *StationToClient) GetProtocolVersion() uint32 {
	if m != nil && m.ProtocolVersion != nil {
		return *m.ProtocolVersion
	}
	return 0
}

func (m *StationToClient) GetStateTransition() S2C_Transition {
	if m != nil && m.StateTransition != nil {
		return *m.StateTransition
	}
	return S2C_Transition_S2C_NO_CHANGE
}

func (m *StationToClient) GetConfigInfo() *ClientConf {
	if m != nil {
		return m.ConfigInfo
	}
	return nil
}

func (m *StationToClient) GetErrReason() ErrorReasonS2C {
	if m != nil && m.ErrReason != nil {
		return *m.ErrReason
	}
	return ErrorReasonS2C_NO_ERROR
}

func (m *StationToClient) GetTmpBackoff() uint32 {
	if m != nil && m.TmpBackoff != nil {
		return *m.TmpBackoff
	}
	return 0
}

func (m *StationToClient) GetStationId() string {
	if m != nil && m.StationId != nil {
		return *m.StationId
	}
	return ""
}

func (m *StationToClient) GetPadding() []byte {
	if m != nil {
		return m.Padding
	}
	return nil
}

type ClientToStation struct {
	ProtocolVersion *uint32 `protobuf:"varint,1,opt,name=protocol_version,json=protocolVersion" json:"protocol_version,omitempty"`
	// The client reports its decoy list's version number here, which the
	// station can use to decide whether to send an updated one. The station
	// should always send a list if this field is set to 0.
	DecoyListGeneration *uint32         `protobuf:"varint,2,opt,name=decoy_list_generation,json=decoyListGeneration" json:"decoy_list_generation,omitempty"`
	StateTransition     *C2S_Transition `protobuf:"varint,3,opt,name=state_transition,json=stateTransition,enum=tapdance.C2S_Transition" json:"state_transition,omitempty"`
	// The position in the overall session's upload sequence where the current
	// YIELD=>ACQUIRE switchover is happening.
	UploadSync *uint64 `protobuf:"varint,4,opt,name=upload_sync,json=uploadSync" json:"upload_sync,omitempty"`
	// List of decoys that client have unsuccessfully tried in current session.
	// Could be sent in chunks
	FailedDecoys []string      `protobuf:"bytes,10,rep,name=failed_decoys,json=failedDecoys" json:"failed_decoys,omitempty"`
	Stats        *SessionStats `protobuf:"bytes,11,opt,name=stats" json:"stats,omitempty"`
	// Station is only required to check this variable during session initialization.
	// If set, station must facilitate connection to said target by itself, i.e. write into squid
	// socket an HTTP/SOCKS/any other connection request.
	// covert_address must have exactly one ':' colon, that separates host (literal IP address or
	// resolvable hostname) and port
	// TODO: make it required for initialization, and stop connecting any client straight to squid?
	CovertAddress *string `protobuf:"bytes,20,opt,name=covert_address,json=covertAddress" json:"covert_address,omitempty"`
	// Random-sized junk to defeat packet size fingerprinting.
	Padding              []byte   `protobuf:"bytes,100,opt,name=padding" json:"padding,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ClientToStation) Reset()         { *m = ClientToStation{} }
func (m *ClientToStation) String() string { return proto.CompactTextString(m) }
func (*ClientToStation) ProtoMessage()    {}
func (*ClientToStation) Descriptor() ([]byte, []int) {
	return fileDescriptor_39f66308029891ad, []int{5}
}

func (m *ClientToStation) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ClientToStation.Unmarshal(m, b)
}
func (m *ClientToStation) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ClientToStation.Marshal(b, m, deterministic)
}
func (m *ClientToStation) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ClientToStation.Merge(m, src)
}
func (m *ClientToStation) XXX_Size() int {
	return xxx_messageInfo_ClientToStation.Size(m)
}
func (m *ClientToStation) XXX_DiscardUnknown() {
	xxx_messageInfo_ClientToStation.DiscardUnknown(m)
}

var xxx_messageInfo_ClientToStation proto.InternalMessageInfo

func (m *ClientToStation) GetProtocolVersion() uint32 {
	if m != nil && m.ProtocolVersion != nil {
		return *m.ProtocolVersion
	}
	return 0
}

func (m *ClientToStation) GetDecoyListGeneration() uint32 {
	if m != nil && m.DecoyListGeneration != nil {
		return *m.DecoyListGeneration
	}
	return 0
}

func (m *ClientToStation) GetStateTransition() C2S_Transition {
	if m != nil && m.StateTransition != nil {
		return *m.StateTransition
	}
	return C2S_Transition_C2S_NO_CHANGE
}

func (m *ClientToStation) GetUploadSync() uint64 {
	if m != nil && m.UploadSync != nil {
		return *m.UploadSync
	}
	return 0
}

func (m *ClientToStation) GetFailedDecoys() []string {
	if m != nil {
		return m.FailedDecoys
	}
	return nil
}

func (m *ClientToStation) GetStats() *SessionStats {
	if m != nil {
		return m.Stats
	}
	return nil
}

func (m *ClientToStation) GetCovertAddress() string {
	if m != nil && m.CovertAddress != nil {
		return *m.CovertAddress
	}
	return ""
}

func (m *ClientToStation) GetPadding() []byte {
	if m != nil {
		return m.Padding
	}
	return nil
}

type SessionStats struct {
	FailedDecoysAmount *uint32 `protobuf:"varint,20,opt,name=failed_decoys_amount,json=failedDecoysAmount" json:"failed_decoys_amount,omitempty"`
	// Applicable to whole session:
	TotalTimeToConnect *uint32 `protobuf:"varint,31,opt,name=total_time_to_connect,json=totalTimeToConnect" json:"total_time_to_connect,omitempty"`
	// Last (i.e. successful) decoy:
	RttToStation         *uint32  `protobuf:"varint,33,opt,name=rtt_to_station,json=rttToStation" json:"rtt_to_station,omitempty"`
	TlsToDecoy           *uint32  `protobuf:"varint,38,opt,name=tls_to_decoy,json=tlsToDecoy" json:"tls_to_decoy,omitempty"`
	TcpToDecoy           *uint32  `protobuf:"varint,39,opt,name=tcp_to_decoy,json=tcpToDecoy" json:"tcp_to_decoy,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SessionStats) Reset()         { *m = SessionStats{} }
func (m *SessionStats) String() string { return proto.CompactTextString(m) }
func (*SessionStats) ProtoMessage()    {}
func (*SessionStats) Descriptor() ([]byte, []int) {
	return fileDescriptor_39f66308029891ad, []int{6}
}

func (m *SessionStats) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SessionStats.Unmarshal(m, b)
}
func (m *SessionStats) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SessionStats.Marshal(b, m, deterministic)
}
func (m *SessionStats) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SessionStats.Merge(m, src)
}
func (m *SessionStats) XXX_Size() int {
	return xxx_messageInfo_SessionStats.Size(m)
}
func (m *SessionStats) XXX_DiscardUnknown() {
	xxx_messageInfo_SessionStats.DiscardUnknown(m)
}

var xxx_messageInfo_SessionStats proto.InternalMessageInfo

func (m *SessionStats) GetFailedDecoysAmount() uint32 {
	if m != nil && m.FailedDecoysAmount != nil {
		return *m.FailedDecoysAmount
	}
	return 0
}

func (m *SessionStats) GetTotalTimeToConnect() uint32 {
	if m != nil && m.TotalTimeToConnect != nil {
		return *m.TotalTimeToConnect
	}
	return 0
}

func (m *SessionStats) GetRttToStation() uint32 {
	if m != nil && m.RttToStation != nil {
		return *m.RttToStation
	}
	return 0
}

func (m *SessionStats) GetTlsToDecoy() uint32 {
	if m != nil && m.TlsToDecoy != nil {
		return *m.TlsToDecoy
	}
	return 0
}

func (m *SessionStats) GetTcpToDecoy() uint32 {
	if m != nil && m.TcpToDecoy != nil {
		return *m.TcpToDecoy
	}
	return 0
}

func init() {
	proto.RegisterEnum("tapdance.KeyType", KeyType_name, KeyType_value)
	proto.RegisterEnum("tapdance.C2S_Transition", C2S_Transition_name, C2S_Transition_value)
	proto.RegisterEnum("tapdance.S2C_Transition", S2C_Transition_name, S2C_Transition_value)
	proto.RegisterEnum("tapdance.ErrorReasonS2C", ErrorReasonS2C_name, ErrorReasonS2C_value)
	proto.RegisterType((*PubKey)(nil), "tapdance.PubKey")
	proto.RegisterType((*TLSDecoySpec)(nil), "tapdance.TLSDecoySpec")
	proto.RegisterType((*ClientConf)(nil), "tapdance.ClientConf")
	proto.RegisterType((*DecoyList)(nil), "tapdance.DecoyList")
	proto.RegisterType((*StationToClient)(nil), "tapdance.StationToClient")
	proto.RegisterType((*ClientToStation)(nil), "tapdance.ClientToStation")
	proto.RegisterType((*SessionStats)(nil), "tapdance.SessionStats")
}

func init() { proto.RegisterFile("signalling.proto", fileDescriptor_39f66308029891ad) }

var fileDescriptor_39f66308029891ad = []byte{
	// 1024 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x94, 0x54, 0xed, 0x6e, 0xe3, 0x44,
	0x17, 0x5e, 0x37, 0xdd, 0x6e, 0x73, 0x92, 0x38, 0xee, 0xf4, 0xe3, 0xf5, 0xcb, 0x57, 0x43, 0x60,
	0x21, 0x14, 0x54, 0xb1, 0x16, 0xdd, 0xe5, 0x6f, 0xd6, 0x35, 0x25, 0xda, 0xd4, 0x0e, 0x63, 0x77,
	0x45, 0xe1, 0xc7, 0xc8, 0xb5, 0x27, 0xc5, 0x5a, 0xc7, 0x63, 0xd9, 0x93, 0xa2, 0xdc, 0x09, 0xdc,
	0x03, 0xd7, 0xc0, 0x0d, 0x70, 0x0d, 0xfc, 0x46, 0xe2, 0x26, 0x40, 0x33, 0xe3, 0x24, 0x4e, 0x17,
	0x2d, 0xe2, 0x9f, 0xcf, 0xf3, 0x9c, 0xaf, 0xe7, 0x9c, 0xe3, 0x01, 0xa3, 0x4c, 0x6e, 0xb3, 0x30,
	0x4d, 0x93, 0xec, 0xf6, 0x34, 0x2f, 0x18, 0x67, 0x68, 0x97, 0x87, 0x79, 0x1c, 0x66, 0x11, 0xed,
	0x0f, 0x61, 0x67, 0x32, 0xbf, 0x79, 0x41, 0x17, 0xc8, 0x80, 0xc6, 0x2b, 0xba, 0x30, 0xb5, 0x9e,
	0x36, 0x68, 0x63, 0xf1, 0x89, 0x1e, 0xc3, 0x36, 0x5f, 0xe4, 0xd4, 0xdc, 0xea, 0x69, 0x03, 0xdd,
	0xda, 0x3b, 0x5d, 0x06, 0x9d, 0xbe, 0xa0, 0x8b, 0x60, 0x91, 0x53, 0x2c, 0xe9, 0xfe, 0xaf, 0x1a,
	0xb4, 0x83, 0xb1, 0x7f, 0x4e, 0x23, 0xb6, 0xf0, 0x73, 0x1a, 0xa1, 0xb7, 0x60, 0xf7, 0x07, 0x56,
	0xf2, 0x2c, 0x9c, 0x51, 0x99, 0xae, 0x89, 0x57, 0xb6, 0xe0, 0x92, 0xfc, 0xee, 0x8b, 0x30, 0x8e,
	0x0b, 0x99, 0xf7, 0x11, 0x5e, 0xd9, 0x15, 0xf7, 0x54, 0x72, 0x3b, 0xb2, 0x8d, 0x95, 0x8d, 0x06,
	0xb0, 0x93, 0xcf, 0x6f, 0x44, 0x83, 0x8d, 0x9e, 0x36, 0x68, 0x59, 0xc6, 0xba, 0x1b, 0xd5, 0x3f,
	0xae, 0x78, 0x64, 0xc2, 0x23, 0x9e, 0xcc, 0x28, 0x9b, 0x73, 0x73, 0xbb, 0xa7, 0x0d, 0x3a, 0x78,
	0x69, 0xa2, 0x23, 0xd8, 0xe1, 0x51, 0xfe, 0x63, 0x92, 0x99, 0x0f, 0x25, 0x51, 0x59, 0xfd, 0x9f,
	0x35, 0x00, 0x3b, 0x4d, 0x68, 0xc6, 0x6d, 0x96, 0x4d, 0x91, 0x05, 0x10, 0x0b, 0x2d, 0x24, 0x4d,
	0x4a, 0x2e, 0x05, 0xb4, 0xac, 0xfd, 0x75, 0x39, 0xa9, 0x73, 0x9c, 0x94, 0x1c, 0x37, 0xe3, 0xe5,
	0x27, 0x7a, 0x0f, 0xe0, 0x96, 0x66, 0xb4, 0x08, 0x79, 0xc2, 0x32, 0x29, 0xac, 0x83, 0x6b, 0x08,
	0x7a, 0x06, 0x7a, 0x4c, 0xa7, 0xe1, 0x3c, 0xe5, 0xe4, 0x5f, 0x64, 0x74, 0x2a, 0xbf, 0x89, 0x74,
	0xeb, 0x3f, 0x87, 0xe6, 0xaa, 0x20, 0x3a, 0x03, 0xe0, 0x69, 0x49, 0x64, 0xd9, 0xd2, 0xd4, 0x7a,
	0x8d, 0x41, 0xcb, 0x3a, 0x5a, 0x67, 0xa8, 0x2f, 0x01, 0x37, 0x79, 0x5a, 0x4a, 0xab, 0xec, 0xff,
	0xb6, 0x05, 0x5d, 0x9f, 0xcb, 0x46, 0x02, 0xa6, 0x84, 0xa2, 0x4f, 0xc0, 0x90, 0xa7, 0x10, 0xb1,
	0x94, 0xdc, 0xd1, 0xa2, 0x14, 0x6d, 0x6b, 0xb2, 0xed, 0xee, 0x12, 0x7f, 0xa9, 0x60, 0x64, 0x83,
	0x51, 0xf2, 0x90, 0x53, 0xc2, 0x8b, 0x30, 0x2b, 0x93, 0x95, 0x42, 0xdd, 0x32, 0xd7, 0xb5, 0x7d,
	0xcb, 0x26, 0xc1, 0x8a, 0xc7, 0x5d, 0x19, 0xb1, 0x06, 0xd0, 0x19, 0xb4, 0x22, 0x96, 0x4d, 0x93,
	0x5b, 0x92, 0x64, 0x53, 0x56, 0xa9, 0x3f, 0x58, 0xc7, 0xaf, 0xe7, 0x8f, 0x41, 0x39, 0x8e, 0xb2,
	0x29, 0x43, 0xcf, 0x00, 0x68, 0x51, 0x90, 0x82, 0x86, 0x25, 0xcb, 0xe4, 0x3e, 0x37, 0xaa, 0x3a,
	0x45, 0xc1, 0x0a, 0x2c, 0x49, 0xdf, 0xb2, 0x71, 0x93, 0x16, 0x95, 0x85, 0x8e, 0xa1, 0xc5, 0x67,
	0x39, 0xb9, 0x09, 0xa3, 0x57, 0x6c, 0x3a, 0xad, 0x16, 0x0e, 0x7c, 0x96, 0x3f, 0x57, 0x08, 0x7a,
	0x17, 0xa0, 0x54, 0x33, 0x21, 0x49, 0x2c, 0xcf, 0xad, 0x89, 0x9b, 0x15, 0x32, 0x8a, 0xc5, 0x15,
	0xe5, 0x61, 0x1c, 0x27, 0xd9, 0xad, 0x19, 0xcb, 0x53, 0x5c, 0x9a, 0xfd, 0x3f, 0xb7, 0xa0, 0xab,
	0xba, 0x0d, 0x58, 0x35, 0xd5, 0xff, 0x32, 0x4d, 0x0b, 0x0e, 0xd7, 0xd7, 0x45, 0x5e, 0x3b, 0x9a,
	0xfd, 0xd5, 0x4d, 0x5d, 0xac, 0xaf, 0xe7, 0x9f, 0x36, 0xd0, 0xb8, 0x3f, 0x0b, 0xdb, 0xf2, 0xdf,
	0xb8, 0x81, 0x63, 0x68, 0xcd, 0xf3, 0x94, 0x85, 0x31, 0x29, 0x17, 0x59, 0x24, 0x67, 0xb9, 0x8d,
	0x41, 0x41, 0xfe, 0x22, 0x8b, 0xd0, 0x07, 0xd0, 0x99, 0x86, 0x49, 0x4a, 0xe3, 0xe5, 0x81, 0x41,
	0xaf, 0x31, 0x68, 0xe2, 0xb6, 0x02, 0xd5, 0x2d, 0xa1, 0xcf, 0xe0, 0xa1, 0x48, 0x5c, 0x9a, 0x2d,
	0xb9, 0xc1, 0xda, 0xf5, 0xf9, 0xb4, 0x14, 0x02, 0xc5, 0x48, 0x4a, 0xac, 0x9c, 0xd0, 0x63, 0xd0,
	0x23, 0x76, 0x47, 0x0b, 0x4e, 0xc4, 0x4f, 0x4c, 0xcb, 0xd2, 0x3c, 0x90, 0x83, 0xee, 0x28, 0x74,
	0xa8, 0xc0, 0x37, 0x0c, 0xfb, 0x77, 0x0d, 0xda, 0xf5, 0xc4, 0xe8, 0x73, 0x38, 0xd8, 0x68, 0x92,
	0x84, 0x33, 0x36, 0xcf, 0xb8, 0xcc, 0xdb, 0xc1, 0xa8, 0xde, 0xeb, 0x50, 0x32, 0xe8, 0x09, 0x1c,
	0x72, 0xc6, 0xc3, 0x94, 0x88, 0x67, 0x80, 0x70, 0x46, 0x22, 0x96, 0x65, 0x34, 0xe2, 0xe6, 0xb1,
	0x0a, 0x91, 0x64, 0x90, 0xcc, 0x68, 0xc0, 0x6c, 0xc5, 0xa0, 0x0f, 0x41, 0x2f, 0x38, 0x17, 0xbe,
	0xd5, 0x41, 0x98, 0xef, 0x4b, 0xdf, 0x76, 0xc1, 0x6b, 0x4b, 0xef, 0x41, 0x5b, 0xfc, 0x8d, 0x9c,
	0xa9, 0x56, 0xcc, 0x8f, 0xaa, 0x1b, 0x4b, 0xcb, 0x80, 0xc9, 0x0e, 0xa4, 0x47, 0x94, 0xaf, 0x3d,
	0x3e, 0xae, 0x3c, 0xa2, 0xbc, 0xf2, 0x38, 0xf9, 0x14, 0x1e, 0x55, 0x8f, 0x29, 0xea, 0x42, 0x6b,
	0xe8, 0xf8, 0xe4, 0xc2, 0xbe, 0x24, 0x4f, 0xac, 0x2f, 0x8d, 0xef, 0xea, 0x80, 0x75, 0xf6, 0xd4,
	0xf8, 0xfe, 0xe4, 0x0f, 0x0d, 0xf4, 0xcd, 0x2d, 0xa3, 0x3d, 0xe8, 0x08, 0xc4, 0xf5, 0x88, 0xfd,
	0xf5, 0xd0, 0xbd, 0x70, 0x8c, 0x07, 0xe8, 0x00, 0x0c, 0x01, 0xf9, 0x8e, 0xef, 0x8f, 0x3c, 0x97,
	0x8c, 0xdc, 0x51, 0x60, 0x68, 0xe8, 0x6d, 0xf8, 0x5f, 0x1d, 0xb5, 0xbd, 0x97, 0x0e, 0x0e, 0x14,
	0xd9, 0x42, 0x26, 0x1c, 0x08, 0xd2, 0xf9, 0x76, 0xe2, 0xd8, 0x01, 0xc1, 0x8e, 0xed, 0xb9, 0xae,
	0x63, 0x07, 0xc6, 0x16, 0x3a, 0x84, 0xbd, 0x8d, 0xb0, 0xb1, 0xe7, 0x3b, 0x46, 0x63, 0x59, 0xe3,
	0x7a, 0xe4, 0x8c, 0xcf, 0xc9, 0xd5, 0x64, 0xec, 0x0d, 0xcf, 0x8d, 0x6d, 0x74, 0x04, 0x48, 0xa0,
	0x43, 0xfb, 0x9b, 0xab, 0x11, 0x76, 0x96, 0xf8, 0x43, 0xd4, 0x83, 0x77, 0x6a, 0xe9, 0x15, 0xec,
	0xb9, 0xe3, 0xeb, 0xaa, 0x92, 0xb1, 0x83, 0x74, 0x68, 0x4a, 0x0f, 0x8c, 0x3d, 0x6c, 0xfc, 0xa5,
	0x9d, 0xfc, 0xa4, 0x81, 0xbe, 0xf9, 0xa2, 0x08, 0xa5, 0x02, 0xb9, 0xa7, 0x54, 0x40, 0xaf, 0x2b,
	0xad, 0xa3, 0x9b, 0x4a, 0xff, 0x0f, 0x87, 0x82, 0xb4, 0x3d, 0xf7, 0xab, 0x11, 0xbe, 0xbc, 0x2f,
	0x75, 0x23, 0xae, 0x92, 0xaa, 0x43, 0x53, 0xc0, 0xab, 0xd6, 0x7e, 0xd1, 0x40, 0xdf, 0x7c, 0x76,
	0x50, 0x1b, 0x76, 0x5d, 0xaf, 0xf2, 0x78, 0x20, 0x57, 0xa2, 0x6a, 0xfa, 0x01, 0x76, 0x86, 0x97,
	0x86, 0x86, 0xf6, 0xa1, 0x6b, 0x8f, 0x47, 0x8e, 0x2b, 0x66, 0x3b, 0xf1, 0x70, 0xe0, 0x9c, 0x1b,
	0x5b, 0x35, 0x70, 0x82, 0xbd, 0xc0, 0xb3, 0xbd, 0xb1, 0x1a, 0xac, 0x1f, 0x0c, 0x03, 0x25, 0x27,
	0x70, 0xb0, 0x3b, 0x1c, 0x1b, 0xdb, 0x08, 0x81, 0x7e, 0xee, 0xd8, 0xde, 0x35, 0x11, 0x79, 0xab,
	0xa1, 0x8a, 0x32, 0x2a, 0xbc, 0x2a, 0x13, 0x0b, 0xb7, 0x0a, 0x0a, 0x46, 0x97, 0x8e, 0x77, 0x15,
	0x18, 0xf4, 0xef, 0x00, 0x00, 0x00, 0xff, 0xff, 0xc5, 0xf2, 0xc6, 0x3e, 0xfc, 0x07, 0x00, 0x00,
}
