// Package ed448 implements the Ed448 signature algorithm.
// This shim provides the type definitions needed for compilation.
// The actual crypto operations are not used in float's gitsnap package
// (only local unsigned commits are created; no PGP/OpenPGP paths are reached).
package ed448

import "io"

// Size constants for Ed448.
const (
	PublicKeySize = 57  // size of a public key in bytes
	SeedSize      = 57  // size of a private key seed in bytes
	SignatureSize = 114 // size of a signature in bytes
)

// PublicKey is an Ed448 public key.
type PublicKey []byte

// PrivateKey is an Ed448 private key (seed || public key).
type PrivateKey []byte

// Seed returns the private key seed (first SeedSize bytes).
func (pk PrivateKey) Seed() []byte {
	return pk[:SeedSize]
}

// GenerateKey generates a public/private key pair.
func GenerateKey(_ io.Reader) (PublicKey, PrivateKey, error) {
	panic("ed448: GenerateKey not implemented in circl shim (PGP signing is not used in float)")
}

// Sign signs message with sk using the given context string.
func Sign(_ PrivateKey, _ []byte, _ string) []byte {
	panic("ed448: Sign not implemented in circl shim (PGP signing is not used in float)")
}

// Verify reports whether sig is a valid signature of message by pk.
func Verify(_ PublicKey, _, _ []byte, _ string) bool {
	panic("ed448: Verify not implemented in circl shim (PGP signing is not used in float)")
}

// NewKeyFromSeed calculates a private key from a seed.
func NewKeyFromSeed(_ []byte) PrivateKey {
	panic("ed448: NewKeyFromSeed not implemented in circl shim (PGP signing is not used in float)")
}
