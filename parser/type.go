package parser

type (
	PacketType byte

	Packet struct {
		Type        PacketType `json:"type" mapstructure:"type" msgpack:"type"`
		Nsp         string     `json:"nsp" mapstructure:"nsp" msgpack:"nsp"`
		Data        any        `json:"data,omitempty" mapstructure:"data,omitempty" msgpack:"data,omitempty"`
		ID          *uint64    `json:"id,omitempty" mapstructure:"id,omitempty" msgpack:"id,omitempty"`
		Attachments *uint64    `json:"attachments,omitempty" mapstructure:"attachments,omitempty" msgpack:"attachments,omitempty"`
	}
)

const (
	Connect      PacketType = '0'
	Disconnect   PacketType = '1'
	Event        PacketType = '2'
	Ack          PacketType = '3'
	ConnectError PacketType = '4'
	BinaryEvent  PacketType = '5'
	BinaryAck    PacketType = '6'
)

func (t PacketType) Valid() bool {
	return t >= '0' && t <= '6'
}

func (t PacketType) String() string {
	switch t {
	case Connect:
		return "CONNECT"
	case Disconnect:
		return "DISCONNECT"
	case Event:
		return "EVENT"
	case Ack:
		return "ACK"
	case ConnectError:
		return "CONNECT_ERROR"
	case BinaryEvent:
		return "BINARY_EVENT"
	case BinaryAck:
		return "BINARY_ACK"
	}
	return "UNKNOWN"
}
