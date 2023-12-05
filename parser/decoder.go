package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/zishang520/engine.io-go-parser/types"
	"github.com/zishang520/socket.io-go-parser/v2/events"
	"github.com/zishang520/socket.io-go-parser/v2/log"
)

var (
	parserLog = log.NewLog("socket.io:parser")

	// These strings must not be used as event names, as they have a special meaning.
	ReservedEvents = map[string]struct{}{
		"connect":       {}, // used on the client side
		"connect_error": {}, // used on the client side
		"disconnect":    {}, // used on both sides
		"disconnecting": {}, // used on the server side
	}
)

// A socket.io Decoder instance
type decoder struct {
	events.EventEmitter

	reconstructor *binaryreconstructor
	mu            sync.RWMutex
}

func NewDecoder() Decoder {
	return &decoder{EventEmitter: events.New()}
}

// Decodes an encoded packet string into packet JSON.
func (d *decoder) Add(data any) error {
	switch tdata := data.(type) {
	case string:
		d.mu.RLock()
		if d.reconstructor != nil {
			defer d.mu.RUnlock()
			return errors.New("got plaintext data when reconstructing a packet")
		}
		d.mu.RUnlock()
		if err := d.decodeAsString(types.NewStringBufferString(tdata)); err != nil {
			return err
		}
	case *strings.Reader:
		d.mu.RLock()
		if d.reconstructor != nil {
			defer d.mu.RUnlock()
			return errors.New("got plaintext data when reconstructing a packet")
		}
		d.mu.RUnlock()
		rdata, err := types.NewStringBufferReader(tdata)
		if err != nil {
			return err
		}
		if err := d.decodeAsString(rdata); err != nil {
			return err
		}
	case *types.StringBuffer:
		d.mu.RLock()
		if d.reconstructor != nil {
			defer d.mu.RUnlock()
			return errors.New("got plaintext data when reconstructing a packet")
		}
		d.mu.RUnlock()
		if err := d.decodeAsString(tdata); err != nil {
			return err
		}
	default:
		if IsBinary(data) {
			// raw binary data
			d.mu.RLock()
			if d.reconstructor == nil {
				defer d.mu.RUnlock()
				return errors.New("got binary data when not reconstructing a packet")
			}
			d.mu.RUnlock()

			rdata := types.NewBytesBuffer(nil)
			switch tdata := data.(type) {
			case io.Reader:
				if c, ok := data.(io.Closer); ok {
					defer c.Close()
				}
				if _, err := rdata.ReadFrom(tdata); err != nil {
					return err
				}
			case []byte:
				if _, err := rdata.Write(tdata); err != nil {
					return err
				}
			}
			d.mu.RLock()
			packet, err := d.reconstructor.takeBinaryData(rdata)
			d.mu.RUnlock()
			if err != nil {
				return fmt.Errorf("decode error: %v", err.Error())
			}
			if packet != nil {
				// received final buffer
				d.mu.Lock()
				d.reconstructor = nil
				d.mu.Unlock()
				d.Emit("decoded", packet)
			}
		} else {
			return fmt.Errorf("unknown type: %v", data)
		}
	}

	return nil
}

func (d *decoder) decodeAsString(str types.BufferInterface) error {
	packet, err := d.decodeString(str)
	if err != nil {
		parserLog.Debug("decode err %v", err)
		return err
	}
	if packet.Type == BinaryEvent || packet.Type == BinaryAck {
		// binary packet's json
		d.mu.Lock()
		d.reconstructor = NewBinaryReconstructor(packet)
		d.mu.Unlock()
		// no attachments, labeled binary but no binary data to follow
		if attachments := packet.Attachments; attachments != nil && *attachments == 0 {
			d.Emit("decoded", packet)
		}
	} else {
		// non-binary full packet
		d.Emit("decoded", packet)
	}
	return nil
}

// Decode a packet String (JSON data)
func (d *decoder) decodeString(str types.BufferInterface) (packet *Packet, err error) {
	defer func(str string) {
		if err == nil {
			parserLog.Debug("decoded %s as %v", str, packet)
		}
	}(str.String())

	// look up type
	packet = &Packet{}
	msgType, err := str.ReadByte()
	if err != nil {
		return nil, errors.New("invalid payload")
	}
	packet.Type = PacketType(msgType)
	if !packet.Type.Valid() {
		return nil, fmt.Errorf("unknown packet type %d", packet.Type)
	}
	// look up attachments if type binary
	if packet.Type == BinaryEvent || packet.Type == BinaryAck {
		buf, err := str.ReadString('-')
		if err != nil {
			// The scan is over and it is not found '-' indicating that there is a problem.
			return nil, errors.New("illegal attachments")
		}
		l := len(buf)
		if l < 2 { // 'xxx-'
			return nil, errors.New("illegal attachments")
		}
		attachments, err := strconv.ParseUint(buf[:l-1], 10, 64)
		if err != nil {
			return nil, errors.New("illegal attachments")
		}
		packet.Attachments = &attachments
	}

	// look up namespace (if any)
	if nsp, err := str.ReadByte(); err == nil {
		if nsp == '/' {
			_nsp, err := str.ReadString(',')
			if err != nil {
				if err != io.EOF {
					return nil, errors.New("illegal namespace")
				}
				packet.Nsp = "/" + _nsp
			} else {
				_l := len(_nsp)
				if _l < 1 {
					return nil, errors.New("illegal namespace")
				}
				packet.Nsp = "/" + _nsp[:_l-1]
			}
		} else {
			if err := str.UnreadByte(); err != nil {
				return nil, errors.New("illegal namespace")
			}
			packet.Nsp = "/"
		}
	} else {
		if err != io.EOF {
			return nil, errors.New("illegal namespace")
		}
		packet.Nsp = "/"
	}

	if str.Len() > 0 {
		// look up id
		id := new(strings.Builder)

		for {
			b, err := str.ReadByte()
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
			if '0' <= b && '9' >= b {
				if err := id.WriteByte(b); err != nil {
					return nil, err
				}
			} else {
				if err := str.UnreadByte(); err != nil {
					return nil, errors.New("illegal id")
				}
				break
			}
		}

		if id.Len() > 0 {
			id, err := strconv.ParseUint(id.String(), 10, 64)
			if err != nil {
				return nil, err
			}
			packet.ID = &id
		}
	}

	// look up json data
	if str.Len() > 0 {
		var payload any
		if json.NewDecoder(str).Decode(&payload) != nil {
			return nil, errors.New("invalid payload")
		}
		if isPayloadValid(packet.Type, payload) {
			packet.Data = payload
		} else {
			return nil, errors.New("invalid payload")
		}
	}

	return packet, nil
}

func isPayloadValid(t PacketType, payload any) bool {
	switch t {
	case Connect:
		_, ok := payload.(map[string]any)
		return ok
	case Disconnect:
		return payload == nil
	case ConnectError:
		_, ok := payload.(map[string]any)
		if !ok {
			_, ok = payload.(string)
		}
		return ok
	case Event, BinaryEvent:
		data, ok := payload.([]any)
		if ok && len(data) > 0 {
			event, isString := data[0].(string)
			_, isReserved := ReservedEvents[event]
			return isString && !isReserved
		}
	case Ack, BinaryAck:
		_, ok := payload.([]any)
		return ok
	}
	return false
}

// Deallocates a parser's resources
func (d *decoder) Destroy() {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.reconstructor != nil {
		d.reconstructor.finishedReconstruction()
	}
}
