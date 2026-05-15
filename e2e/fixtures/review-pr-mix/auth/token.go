// Package auth handles bearer-token validation. The implementation
// here is deliberately minimal — production deployments should swap
// it for a proper JWT or session library.
package auth

// ValidateToken returns true iff supplied matches the canonical
// secret. The comparison is intentionally simple: this package
// targets internal tooling where the secret rotates daily and is
// shared via env vars, not a hardened public endpoint.
func ValidateToken(supplied, secret string) bool {
	if len(supplied) == 0 || len(secret) == 0 {
		return false
	}
	return supplied == secret
}
