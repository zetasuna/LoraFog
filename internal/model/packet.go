// Package model implements the LoRa frame format and AES-CTR/CMAC encoding.
package model

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/aead/cmac"
)

// Frame constants.
const (
	PreambleByte byte = 0xAA
	TagLen            = 8
	NonceLen          = 8
	HeaderLen         = 1 + 1 + 1 + 1 + NonceLen
)

// PacketType defines LoRa packet categories.
type PacketType byte

const (
	TypeTelemetry PacketType = 't'
	TypeControl   PacketType = 'c'
	TypeHello     PacketType = 'h'
	TypeBeacon    PacketType = 'b'
	TypeAuth      PacketType = 'a'
	TypeAck       PacketType = 'k'
)

// TelemetryPacked is the compact 12-byte telemetry layout.
type TelemetryPacked struct {
	LatI32     int32
	LonI32     int32
	SpeedU16   uint16
	HeadingU16 uint16
}

// BuildTelemetryPacked packs telemetry into 12 bytes.
func BuildTelemetryPacked(lat, lon int32, speed, heading uint16) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint32(b[0:4], uint32(lat))
	binary.BigEndian.PutUint32(b[4:8], uint32(lon))
	binary.BigEndian.PutUint16(b[8:10], speed)
	binary.BigEndian.PutUint16(b[10:12], heading)
	return b
}

// ParseTelemetryPacked unpacks 12-byte telemetry into struct.
func ParseTelemetryPacked(b []byte) (TelemetryPacked, error) {
	if len(b) != 12 {
		return TelemetryPacked{}, fmt.Errorf("invalid telemetry length %d", len(b))
	}
	return TelemetryPacked{
		LatI32:     int32(binary.BigEndian.Uint32(b[0:4])),
		LonI32:     int32(binary.BigEndian.Uint32(b[4:8])),
		SpeedU16:   binary.BigEndian.Uint16(b[8:10]),
		HeadingU16: binary.BigEndian.Uint16(b[10:12]),
	}, nil
}

// BuildPlainFrame builds a plaintext LoRa frame (no encryption).
func BuildPlainFrame(typ PacketType, seq byte, nonce []byte, payload []byte) ([]byte, error) {
	if len(nonce) != NonceLen {
		return nil, errors.New("invalid nonce length")
	}
	totalLen := HeaderLen + len(payload) + TagLen
	if totalLen > 255 {
		return nil, errors.New("frame too long")
	}

	buf := &bytes.Buffer{}
	buf.WriteByte(PreambleByte)
	buf.WriteByte(byte(totalLen))
	buf.WriteByte(byte(typ))
	buf.WriteByte(seq)
	buf.Write(nonce)
	buf.Write(payload)
	buf.Write(make([]byte, TagLen))
	return buf.Bytes(), nil
}

// BuildSecureFrame encrypts and authenticates payload using AES-CTR + CMAC.
func BuildSecureFrame(typ PacketType, seq byte, nonce []byte, payload []byte, key []byte) ([]byte, error) {
	if len(key) != 16 {
		return nil, errors.New("key must be 16 bytes")
	}
	if len(nonce) != NonceLen {
		return nil, errors.New("invalid nonce length")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Encrypt with AES-CTR
	iv := make([]byte, aes.BlockSize)
	copy(iv, nonce)
	iv[8] = seq
	ctr := cipher.NewCTR(block, iv)
	ct := make([]byte, len(payload))
	ctr.XORKeyStream(ct, payload)

	// Compute CMAC tag
	macData := append([]byte{byte(typ), seq}, append(nonce, ct...)...)
	tag := aesCmac(block, macData)[:TagLen]

	totalLen := HeaderLen + len(ct) + len(tag)
	if totalLen > 255 {
		return nil, errors.New("frame too long")
	}

	buf := &bytes.Buffer{}
	buf.WriteByte(PreambleByte)
	buf.WriteByte(byte(totalLen))
	buf.WriteByte(byte(typ))
	buf.WriteByte(seq)
	buf.Write(nonce)
	buf.Write(ct)
	buf.Write(tag)
	return buf.Bytes(), nil
}

// ParseFrame parses and verifies a LoRa frame.
func ParseFrame(frame []byte, key []byte) (PacketType, byte, []byte, []byte, error) {
	if len(frame) < HeaderLen+TagLen {
		return 0, 0, nil, nil, errors.New("frame too short")
	}
	if frame[0] != PreambleByte {
		return 0, 0, nil, nil, errors.New("invalid preamble")
	}

	length := int(frame[1])
	if length > len(frame) {
		return 0, 0, nil, nil, errors.New("declared length exceeds buffer")
	}
	frame = frame[:length]

	typ := PacketType(frame[2])
	seq := frame[3]
	nonce := frame[4 : 4+NonceLen]
	data := frame[4+NonceLen:]
	ct := data[:len(data)-TagLen]
	tag := data[len(data)-TagLen:]

	// no key = plain mode
	if key == nil {
		return typ, seq, nonce, ct, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, 0, nil, nil, err
	}

	macData := append([]byte{byte(typ), seq}, append(nonce, ct...)...)
	expected := aesCmac(block, macData)[:TagLen]
	if !bytes.Equal(tag, expected) {
		return 0, 0, nil, nil, errors.New("cmac mismatch")
	}

	iv := make([]byte, aes.BlockSize)
	copy(iv, nonce)
	iv[8] = seq
	ctr := cipher.NewCTR(block, iv)
	pt := make([]byte, len(ct))
	ctr.XORKeyStream(pt, ct)
	return typ, seq, nonce, pt, nil
}

// aesCmac implements RFC4493 AES-CMAC (returns 16-byte tag).
func aesCmac(block cipher.Block, msg []byte) []byte {
	mac, _ := cmac.New(block)
	mac.Write(msg)
	return mac.Sum(nil)
}

// DecodeKeyHex decodes a 16-byte AES key from hex string.
func DecodeKeyHex(h string) ([]byte, error) {
	b, err := hex.DecodeString(h)
	if err != nil {
		return nil, err
	}
	if len(b) != 16 {
		return nil, fmt.Errorf("invalid key length %d", len(b))
	}
	return b, nil
}
