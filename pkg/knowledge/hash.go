package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
)

// ChecksumHex returns the lowercase hex SHA-256 of b — the canonical
// content checksum stored on a DocumentMeta by both adapters.
func ChecksumHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
