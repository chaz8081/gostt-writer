// Package protocol implements protobuf encoding for the GOSTT-KBD BLE protocol.
package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ResponseType is the type field in a ResponsePacket.
type ResponseType uint32

const (
	ResponseTypeKeepalive  ResponseType = 0
	ResponseTypePeerStatus ResponseType = 1
)

// PeerStatus indicates whether the ESP32 recognizes us.
type PeerStatus uint32

const (
	PeerStatusUnknown PeerStatus = 0
	PeerStatusKnown   PeerStatus = 1
)

// ResponsePacket is the decoded response from the ESP32.
type ResponsePacket struct {
	Type       ResponseType
	PeerStatus PeerStatus
	Data       []byte // challenge data during pairing
}

// MarshalKeyboardPacket encodes a KeyboardPacket protobuf.
//
//	field 1 (string): message
//	field 2 (uint32): length of message
func MarshalKeyboardPacket(message string) []byte {
	var buf []byte
	// Field 1: tag = (1 << 3) | 2 = 0x0a, length-delimited
	buf = append(buf, 0x0a)
	buf = appendVarint(buf, uint64(len(message)))
	buf = append(buf, message...)
	// Field 2: tag = (2 << 3) | 0 = 0x10, varint
	buf = append(buf, 0x10)
	buf = appendVarint(buf, uint64(len(message)))
	return buf
}

// MarshalEncryptedData wraps a serialized KeyboardPacket in an EncryptedData envelope.
// For GOSTT-KBD, EncryptedData has a single field: KeyboardPacket (field 1, bytes).
func MarshalEncryptedData(keyboardPacket []byte) []byte {
	var buf []byte
	buf = append(buf, 0x0a)
	buf = appendVarint(buf, uint64(len(keyboardPacket)))
	buf = append(buf, keyboardPacket...)
	return buf
}

// MarshalDataPacket encodes a DataPacket protobuf (the outer encrypted wrapper).
//
//	field 1 (bytes): iv (12 bytes)
//	field 2 (bytes): tag (16 bytes)
//	field 3 (bytes): encrypted data
//	field 4 (uint32): packet_num
func MarshalDataPacket(iv, tag, encrypted []byte, packetNum uint32) ([]byte, error) {
	if len(iv) != 12 {
		return nil, fmt.Errorf("protocol: iv must be 12 bytes, got %d", len(iv))
	}
	if len(tag) != 16 {
		return nil, fmt.Errorf("protocol: tag must be 16 bytes, got %d", len(tag))
	}
	var buf []byte
	// Field 1: iv
	buf = append(buf, 0x0a)
	buf = appendVarint(buf, uint64(len(iv)))
	buf = append(buf, iv...)
	// Field 2: tag
	buf = append(buf, 0x12)
	buf = appendVarint(buf, uint64(len(tag)))
	buf = append(buf, tag...)
	// Field 3: encrypted
	buf = append(buf, 0x1a)
	buf = appendVarint(buf, uint64(len(encrypted)))
	buf = append(buf, encrypted...)
	// Field 4: packet_num
	buf = append(buf, 0x20)
	buf = appendVarint(buf, uint64(packetNum))
	return buf, nil
}

// UnmarshalResponsePacket decodes a ResponsePacket from raw protobuf bytes.
func UnmarshalResponsePacket(data []byte) (*ResponsePacket, error) {
	resp := &ResponsePacket{}
	for len(data) > 0 {
		tag, n, err := readVarint(data)
		if err != nil {
			return nil, fmt.Errorf("protocol: reading tag: %w", err)
		}
		data = data[n:]
		fieldNum := uint8(tag >> 3)
		wireType := uint8(tag & 0x07)

		switch wireType {
		case 0: // varint
			val, n, err := readVarint(data)
			if err != nil {
				return nil, fmt.Errorf("protocol: reading varint for field %d: %w", fieldNum, err)
			}
			data = data[n:]
			switch fieldNum {
			case 1:
				resp.Type = ResponseType(val)
			case 2:
				resp.PeerStatus = PeerStatus(val)
			}
		case 2: // length-delimited
			if len(data) < 1 {
				return nil, errors.New("protocol: truncated length in response packet")
			}
			length, n, err := readVarint(data)
			if err != nil {
				return nil, fmt.Errorf("protocol: reading length for field %d: %w", fieldNum, err)
			}
			data = data[n:]
			if uint64(len(data)) < length {
				return nil, fmt.Errorf("protocol: field %d length %d exceeds remaining %d bytes", fieldNum, length, len(data))
			}
			switch fieldNum {
			case 3:
				resp.Data = make([]byte, length)
				copy(resp.Data, data[:length])
			}
			data = data[length:]
		default:
			return nil, fmt.Errorf("protocol: unsupported wire type %d for field %d", wireType, fieldNum)
		}
	}
	return resp, nil
}

// appendVarint appends a protobuf varint to buf.
func appendVarint(buf []byte, v uint64) []byte {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(tmp[:], v)
	return append(buf, tmp[:n]...)
}

// readVarint reads a protobuf varint from data, returning value and bytes consumed.
func readVarint(data []byte) (uint64, int, error) {
	val, n := binary.Uvarint(data)
	if n <= 0 {
		return 0, 0, errors.New("protocol: invalid varint")
	}
	return val, n, nil
}
