package webhooks

import (
	"crypto/subtle"

	"github.com/SocialGouv/iterion/pkg/auth"
	"github.com/SocialGouv/iterion/pkg/secrets"
)

// TokenPrefix marks an iterion webhook token so it's recognisable in
// configs/logs (the secret material follows the prefix).
const TokenPrefix = "iwh_"

// MintToken returns a fresh webhook token plaintext (shown to the
// operator exactly once) plus the at-rest fields persisted on a Config:
// a salted hash, the last 4 chars, and a fingerprint. Reuses the same
// random-token + hash primitives as operator session/invitation tokens.
func MintToken() (plaintext, hash, last4, fingerprint string, err error) {
	tok, _, err := auth.GenerateRandomToken(32)
	if err != nil {
		return "", "", "", "", err
	}
	plaintext = TokenPrefix + tok
	return plaintext, auth.HashRefreshToken(plaintext), secrets.Last4(plaintext), secrets.FingerprintSHA256(plaintext), nil
}

// VerifyToken constant-time compares a presented token against a stored
// hash. Constant-time avoids leaking a near-miss via timing.
func VerifyToken(presented, storedHash string) bool {
	if presented == "" || storedHash == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(auth.HashRefreshToken(presented)), []byte(storedHash)) == 1
}
