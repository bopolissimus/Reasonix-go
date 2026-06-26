package security

import "crypto/rand"
import "encoding/hex"

// DataNonce generates a cryptographic random nonce for delimiting untrusted
// data in LLM prompts. This is the "StruQ + Spotlighting" pattern from
// prompt injection research: a unique boundary token per request that an
// attacker cannot predict, preventing tag-closure attacks.
//
// REX-88: Dynamic nonce-based delimiters.
func DataNonce() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
