// Package x448 implements Diffie-Hellman over Curve448.
// This shim provides the type definitions needed for compilation.
// The actual crypto operations are not used in float's gitsnap package
// (only local unsigned commits are created; no PGP/OpenPGP paths are reached).
package x448

// Size is the size in bytes of a x448 key.
const Size = 56

// Key represents a 56-byte x448 key (public or private).
type Key [Size]byte

// KeyGen sets pub to the public key corresponding to priv.
func KeyGen(pub, priv *Key) {
	panic("x448: KeyGen not implemented in circl shim (PGP signing is not used in float)")
}

// Shared computes the shared secret between priv and pub.
func Shared(shared, priv, pub *Key) bool {
	panic("x448: Shared not implemented in circl shim (PGP signing is not used in float)")
}
