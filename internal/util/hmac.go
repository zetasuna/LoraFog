package util

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

// GenerateRandomKey returns n random bytes (use n=16 for AES-128 / HMAC key).
func GenerateRandomKey(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate random key: %w", err)
	}
	return b, nil
}

// ComputeHMAC computes HMAC-SHA256 and returns the first tagLen bytes.
func ComputeHMAC(key, data []byte, tagLen int) []byte {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write(data)
	full := m.Sum(nil)
	if tagLen <= 0 || tagLen > len(full) {
		tagLen = len(full)
	}
	return full[:tagLen]
}

// VerifyHMAC compares truncated HMAC in constant time.
func VerifyHMAC(key, data, tag []byte) bool {
	expected := ComputeHMAC(key, data, len(tag))
	return hmac.Equal(expected, tag)
}
