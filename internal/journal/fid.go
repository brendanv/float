package journal

import (
	"strings"

	"github.com/google/uuid"
)

// FIDLen is the length in characters of a float transaction ID.
const FIDLen = 8

// MintFID generates a random FIDLen-character lowercase hex string using a UUID v4.
// It takes the first FIDLen characters of the UUID (excluding dashes).
// Example output: "a1b2c3d4"
func MintFID() string {
	id := uuid.New().String()                // "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
	clean := strings.ReplaceAll(id, "-", "") // remove dashes
	return clean[:FIDLen]
}
