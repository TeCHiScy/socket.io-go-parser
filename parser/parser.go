package parser

import (
	"github.com/zishang520/engine.io-go-parser/types"
	"github.com/zishang520/socket.io-go-parser/v2/events"
)

type (
	// Encoder is a socket.io Encoder instance
	Encoder interface {
		Encode(*Packet) []types.BufferInterface
	}

	// Decoder is a socket.io Decoder instance
	Decoder interface {
		events.EventEmitter

		Add(any) error
		Destroy()
	}

	Parser interface {
		// A socket.io Encoder instance
		NewEncoder() Encoder

		// A socket.io Decoder instance
		NewDecoder() Decoder
	}

	parser struct {
	}
)

func (p *parser) NewEncoder() Encoder {
	return NewEncoder()
}
func (p *parser) NewDecoder() Decoder {
	return NewDecoder()
}

func NewParser() Parser {
	return &parser{}
}
