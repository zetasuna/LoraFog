// Package util provide method for generating UUIDv4 for vehicle
package util

import (
	"crypto/rand"
	"fmt"
)

func GenerateUUIDv4() (string, error) {
	uuid := make([]byte, 16)
	_, err := rand.Read(uuid)
	if err != nil {
		return "", err
	}

	// Set version (4)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	// Set variant
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16],
	), nil
}
