package journal

import (
	"strings"

	"github.com/google/uuid"
)

// MintFID generates a random 8-character lowercase hex string using a UUID v4.
// It takes the first 8 characters of the UUID (excluding dashes).
// Example output: "a1b2c3d4"
func MintFID() string {
	id := uuid.New().String()              // "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
	clean := strings.ReplaceAll(id, "-", "") // remove dashes
	return clean[:8]
}
