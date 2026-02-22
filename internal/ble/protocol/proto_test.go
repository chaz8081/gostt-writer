package protocol

import (
	"bytes"
	"testing"
)

func TestMarshalKeyboardPacket(t *testing.T) {
	msg := "hello"
	got := MarshalKeyboardPacket(msg)
	// Field 1 (string): tag=0x0a, len=5, "hello"
	// Field 2 (uint32): tag=0x10, varint=5
	want := []byte{0x0a, 0x05, 'h', 'e', 'l', 'l', 'o', 0x10, 0x05}
	if !bytes.Equal(got, want) {
		t.Errorf("MarshalKeyboardPacket(%q) = %x, want %x", msg, got, want)
	}
}

func TestMarshalKeyboardPacketEmpty(t *testing.T) {
	got := MarshalKeyboardPacket("")
	// Empty string: field 1 tag=0x0a, len=0
	// Field 2: tag=0x10, varint=0
	want := []byte{0x0a, 0x00, 0x10, 0x00}
	if !bytes.Equal(got, want) {
		t.Errorf("MarshalKeyboardPacket(%q) = %x, want %x", "", got, want)
	}
}

func TestMarshalDataPacket(t *testing.T) {
	iv := make([]byte, 12)
	iv[0] = 0xAA
	tag := make([]byte, 16)
	tag[0] = 0xBB
	encrypted := []byte{0x01, 0x02, 0x03}
	packetNum := uint32(42)

	got, err := MarshalDataPacket(iv, tag, encrypted, packetNum)
	if err != nil {
		t.Fatalf("MarshalDataPacket() error = %v", err)
	}

	// Build the exact expected byte sequence:
	// Field 1 (iv):        tag=0x0a, len=0x0c, 12 bytes (0xAA followed by 11 zeros)
	// Field 2 (tag):       tag=0x12, len=0x10, 16 bytes (0xBB followed by 15 zeros)
	// Field 3 (encrypted): tag=0x1a, len=0x03, 3 bytes
	// Field 4 (packetNum): tag=0x20, varint=0x2a (42)
	var want []byte
	want = append(want, 0x0a, 0x0c)
	want = append(want, iv...)
	want = append(want, 0x12, 0x10)
	want = append(want, tag...)
	want = append(want, 0x1a, 0x03)
	want = append(want, encrypted...)
	want = append(want, 0x20, 0x2a) // field 4 tag + varint 42

	if !bytes.Equal(got, want) {
		t.Errorf("MarshalDataPacket() =\n  got  %x\n  want %x", got, want)
	}
}

func TestMarshalDataPacketValidation(t *testing.T) {
	validIV := make([]byte, 12)
	validTag := make([]byte, 16)
	encrypted := []byte{0x01}

	// Wrong IV length
	_, err := MarshalDataPacket(make([]byte, 10), validTag, encrypted, 0)
	if err == nil {
		t.Error("expected error for wrong IV length")
	}

	// Wrong tag length
	_, err = MarshalDataPacket(validIV, make([]byte, 8), encrypted, 0)
	if err == nil {
		t.Error("expected error for wrong tag length")
	}
}

func TestMarshalEncryptedData(t *testing.T) {
	inner := []byte{0x0a, 0x05, 'h', 'e', 'l', 'l', 'o', 0x10, 0x05}
	got := MarshalEncryptedData(inner)

	// Should wrap inner as field 1 (bytes): tag=0x0a, length, then inner bytes
	var want []byte
	want = append(want, 0x0a, byte(len(inner)))
	want = append(want, inner...)

	if !bytes.Equal(got, want) {
		t.Errorf("MarshalEncryptedData() =\n  got  %x\n  want %x", got, want)
	}
}

func TestUnmarshalResponsePacket(t *testing.T) {
	// Hand-craft a ResponsePacket: type=1 (PEER_STATUS), peer_status=0 (PEER_UNKNOWN), data=0xDE 0xAD
	raw := []byte{
		0x08, 0x01, // field 1: varint 1
		0x10, 0x00, // field 2: varint 0
		0x1a, 0x02, 0xDE, 0xAD, // field 3: bytes len=2
	}
	resp, err := UnmarshalResponsePacket(raw)
	if err != nil {
		t.Fatalf("UnmarshalResponsePacket() error = %v", err)
	}
	if resp.Type != ResponseTypePeerStatus {
		t.Errorf("Type = %d, want %d", resp.Type, ResponseTypePeerStatus)
	}
	if resp.PeerStatus != PeerStatusUnknown {
		t.Errorf("PeerStatus = %d, want %d", resp.PeerStatus, PeerStatusUnknown)
	}
	if !bytes.Equal(resp.Data, []byte{0xDE, 0xAD}) {
		t.Errorf("Data = %x, want dead", resp.Data)
	}
}

func TestUnmarshalResponsePacketInvalid(t *testing.T) {
	_, err := UnmarshalResponsePacket([]byte{0xFF})
	if err == nil {
		t.Error("expected error for invalid protobuf")
	}
}

func TestUnmarshalResponsePacketNilAndEmpty(t *testing.T) {
	// nil input should return zero-valued packet with no error
	resp, err := UnmarshalResponsePacket(nil)
	if err != nil {
		t.Fatalf("UnmarshalResponsePacket(nil) error = %v", err)
	}
	if resp.Type != 0 || resp.PeerStatus != 0 || resp.Data != nil {
		t.Errorf("UnmarshalResponsePacket(nil) = %+v, want zero-valued", resp)
	}

	// empty input should return zero-valued packet with no error
	resp, err = UnmarshalResponsePacket([]byte{})
	if err != nil {
		t.Fatalf("UnmarshalResponsePacket([]byte{}) error = %v", err)
	}
	if resp.Type != 0 || resp.PeerStatus != 0 || resp.Data != nil {
		t.Errorf("UnmarshalResponsePacket([]byte{}) = %+v, want zero-valued", resp)
	}
}
