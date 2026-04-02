// Package x25519 implements Diffie-Hellman function over Curve25519.
// This is a shim that delegates to Go's standard crypto/ecdh package.
package x25519

import "crypto/ecdh"

// Size is the size in bytes of a x25519 key.
const Size = 32

// Key represents a 32-byte x25519 key (public or private).
type Key [Size]byte

var x25519Curve = ecdh.X25519()

// KeyGen sets pub to the public key corresponding to priv.
func KeyGen(pub, priv *Key) {
	privKey, err := x25519Curve.NewPrivateKey(priv[:])
	if err != nil {
		return
	}
	copy(pub[:], privKey.PublicKey().Bytes())
}

// Shared computes the shared secret between priv and pub and writes it to
// shared. Returns false if pub is a low-order point (all-zero shared secret).
func Shared(shared, priv, pub *Key) bool {
	privKey, err := x25519Curve.NewPrivateKey(priv[:])
	if err != nil {
		return false
	}
	pubKey, err := x25519Curve.NewPublicKey(pub[:])
	if err != nil {
		return false
	}
	result, err := privKey.ECDH(pubKey)
	if err != nil {
		return false
	}
	copy(shared[:], result)
	// A zero shared secret indicates a low-order point.
	var acc byte
	for _, b := range shared {
		acc |= b
	}
	return acc != 0
}
