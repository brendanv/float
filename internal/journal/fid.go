package journal

import (
	"strings"

	"github.com/brendanv/float/internal/hledger"
	"github.com/google/uuid"
)

// MintFID generates a random hledger.FIDLen-character lowercase hex string using a UUID v4.
// It takes the first hledger.FIDLen characters of the UUID (excluding dashes).
// Example output: "a1b2c3d4"
func MintFID() string {
	id := uuid.New().String()                // "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
	clean := strings.ReplaceAll(id, "-", "") // remove dashes
	return clean[:hledger.FIDLen]
}
