// Package ed25519 implements the Ed25519 signature algorithm.
// This shim delegates to Go's standard crypto/ed25519 package.
package ed25519

import (
	"io"

	goed25519 "crypto/ed25519"
)

// PublicKey is an Ed25519 public key (32 bytes).
type PublicKey = goed25519.PublicKey

// PrivateKey is an Ed25519 private key (64 bytes: seed || public key).
type PrivateKey = goed25519.PrivateKey

// Size constants for Ed25519.
const (
	PublicKeySize = goed25519.PublicKeySize // 32
	SeedSize      = goed25519.SeedSize      // 32
	SignatureSize = goed25519.SignatureSize // 64
)

// GenerateKey generates a public/private key pair using entropy from rand.
func GenerateKey(rand io.Reader) (PublicKey, PrivateKey, error) {
	return goed25519.GenerateKey(rand)
}

// Sign signs message with sk and returns a signature.
func Sign(sk PrivateKey, message []byte) []byte {
	return goed25519.Sign(sk, message)
}

// Verify reports whether sig is a valid signature of message by pk.
func Verify(pk PublicKey, message, sig []byte) bool {
	return goed25519.Verify(pk, message, sig)
}

// NewKeyFromSeed calculates a private key from a seed.
func NewKeyFromSeed(seed []byte) PrivateKey {
	return goed25519.NewKeyFromSeed(seed)
}
