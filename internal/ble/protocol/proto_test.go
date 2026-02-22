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

	got := MarshalDataPacket(iv, tag, encrypted, packetNum)

	// Verify it starts with field 1 tag (0x0a) and length 12
	if len(got) < 2 || got[0] != 0x0a || got[1] != 12 {
		t.Errorf("DataPacket field 1 header: got %x, want 0a0c", got[:2])
	}

	// Just verify round-trip-ish: the packet should contain our iv, tag, encrypted data
	if !bytes.Contains(got, iv) {
		t.Error("DataPacket does not contain IV")
	}
	if !bytes.Contains(got, tag) {
		t.Error("DataPacket does not contain tag")
	}
	if !bytes.Contains(got, encrypted) {
		t.Error("DataPacket does not contain encrypted data")
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
