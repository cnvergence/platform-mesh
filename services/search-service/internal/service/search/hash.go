package search

import (
	"crypto/sha256"
	"encoding/hex"
)

func queryHash(q string) string {
	h := sha256.Sum256([]byte(q))
	return hex.EncodeToString(h[:])
}
