package security

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// DataNonce generates a cryptographic random nonce for delimiting untrusted
// data in LLM prompts. This is the "StruQ + Spotlighting" pattern from
// prompt injection research: a unique boundary token per request that an
// attacker cannot predict, preventing tag-closure attacks.
//
// 4 bytes → 8 hex chars → ~4 billion possibilities — sufficient for per-request
// unpredictability (an attacker cannot observe the nonce before the turn runs).
//
// REX-88: Dynamic nonce-based delimiters.
func DataNonce() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// DataNonceSafe is like DataNonce but guarantees the nonce does not appear as a
// substring of content. This prevents the degenerate case where a tool output
// happens to contain the nonce bytes, which would let an attacker close the
// delimiter from inside. The collision probability for 8 hex chars is ~1 in
// 4 billion per nonce-sized window, but the re-roll is essentially free.
func DataNonceSafe(content string) string {
	for {
		n := DataNonce()
		if !strings.Contains(content, n) {
			return n
		}
	}
}
