// internal/util/packet.go
// CHANGELOG:
// - New helpers for building and verifying wire frames used on LoRa:
//   Frame layout: [LEN(2)][NONCE(4)][HMAC(8)][PAYLOAD]
// - Uses util.ComputeHMAC / VerifyHMAC for HMAC-SHA256 truncated

package util

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Frame constants
const (
	NonceLen  = 4
	HMACLen   = 8
	PrefixLen = 2 // LEN field (uint16)
	HeaderLen = NonceLen + HMACLen
	MinFrame  = PrefixLen + HeaderLen
)

// BuildFrame creates a simple frame [LEN(2)][PAYLOAD].
func BuildFrame(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, errors.New("empty payload")
	}
	totalLen := PrefixLen + len(payload)
	if totalLen > 0xFFFF {
		return nil, fmt.Errorf("payload too large")
	}

	frame := make([]byte, totalLen)
	binary.BigEndian.PutUint16(frame[0:PrefixLen], uint16(len(payload)))
	copy(frame[PrefixLen:], payload)
	return frame, nil
}

// ParseFrame strips the 2-byte prefix and returns only the payload.
func ParseFrame(frame []byte) ([]byte, error) {
	if len(frame) < PrefixLen {
		return nil, errors.New("frame too short")
	}
	expectedLen := int(binary.BigEndian.Uint16(frame[:PrefixLen]))
	if expectedLen != len(frame)-PrefixLen {
		return nil, fmt.Errorf("length mismatch: got %d, expected %d", len(frame)-PrefixLen, expectedLen)
	}
	return frame[PrefixLen:], nil
}

// BuildFrameWithKey builds a wire frame from payload and a shared key.
// Returns full frame including 2-byte length prefix.
// Frame = [LEN:2][NONCE:4][HMAC:8][PAYLOAD]
func BuildFrameWithKey(key []byte, payload []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("empty key")
	}
	// generate 4-byte nonce
	var nonce [NonceLen]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// compute HMAC over (nonce || payload)
	data := append(nonce[:], payload...)
	tag := ComputeHMAC(key, data, HMACLen)

	totalLen := PrefixLen + HeaderLen + len(payload)
	if totalLen > 0xFFFF {
		return nil, fmt.Errorf("payload too large")
	}

	frame := make([]byte, totalLen)
	// write length (big endian) at [0:2]
	binary.BigEndian.PutUint16(frame[0:PrefixLen], uint16(totalLen))
	// write nonce
	copy(frame[PrefixLen:PrefixLen+NonceLen], nonce[:])
	// write tag
	copy(frame[PrefixLen+NonceLen:PrefixLen+NonceLen+HMACLen], tag)
	// write payload
	copy(frame[PrefixLen+HeaderLen:], payload)

	return frame, nil
}

// ParseAndVerifyFrameWithKey parses a raw frame (as returned by LoraDevice.ReadFrame())
// and verifies HMAC using the provided key. If valid, returns payload and nonce.
func ParseAndVerifyFrameWithKey(key []byte, frame []byte) (payload []byte, nonce []byte, err error) {
	if len(frame) < HeaderLen {
		return nil, nil, errors.New("frame too short")
	}
	// Expectation: caller may pass in payload portion (without LEN) or full frame with LEN.
	// Accept both: if len(frame) >= MinFrame and the first two bytes look like length, strip prefix.
	if len(frame) >= MinFrame {
		// check whether first two bytes indicate the total length equals len(frame)
		pref := binary.BigEndian.Uint16(frame[0:PrefixLen])
		if int(pref) == len(frame) {
			// strip length prefix
			frame = frame[PrefixLen:]
		}
	}
	// Now frame layout: [NONCE(4)][HMAC(8)][PAYLOAD]
	if len(frame) < HeaderLen {
		return nil, nil, errors.New("payload too short after stripping prefix")
	}

	nonce = make([]byte, NonceLen)
	copy(nonce, frame[0:NonceLen])
	tag := frame[NonceLen : NonceLen+HMACLen]
	payload = make([]byte, len(frame)-HeaderLen)
	copy(payload, frame[HeaderLen:])

	// compute expected tag
	data := append(nonce, payload...)
	if !VerifyHMAC(key, data, tag) {
		return nil, nil, errors.New("hmac verification failed")
	}
	return payload, nonce, nil
}
