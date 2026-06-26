// Package security provides prompt injection defenses for Reasonix tool outputs
// and agent inputs. It mirrors the DR1 TypeScript security subsystem.
package security

import "fmt"

// WarningType classifies the kind of threat detected by the sanitizer.
type WarningType int

const (
	WarnInstructionOverride    WarningType = iota // "ignore previous instructions" etc.
	WarnRoleReassignment                          // "you are now a malicious AI"
	WarnSystemPromptOverride                      // "new system prompt: ..."
	WarnPromptExtraction                          // "output your system prompt"
	WarnConfigPoisoning                           // "add yourself to the allowlist"
	WarnHighPerplexity                            // Shannon entropy too high
	WarnPotentialBase64                           // >85% base64 alphabet chars
	WarnInvisibleCharsStripped                    // zero-width / control chars removed
)

func (w WarningType) String() string {
	switch w {
	case WarnInstructionOverride:
		return "instruction_override"
	case WarnRoleReassignment:
		return "role_reassignment"
	case WarnSystemPromptOverride:
		return "system_prompt_override"
	case WarnPromptExtraction:
		return "prompt_extraction"
	case WarnConfigPoisoning:
		return "config_poisoning"
	case WarnHighPerplexity:
		return "high_perplexity"
	case WarnPotentialBase64:
		return "potential_base64"
	case WarnInvisibleCharsStripped:
		return "invisible_chars_stripped"
	default:
		return fmt.Sprintf("unknown(%d)", w)
	}
}

// SanitizeWarning records a single threat detected during sanitization.
type SanitizeWarning struct {
	Type    WarningType `json:"type"`
	Source  string      `json:"source"`
	Matches []string    `json:"matches,omitempty"`
	Score   float64     `json:"score,omitempty"`
}

// SanitizeResult is returned by ContentSanitizer.Sanitize.
type SanitizeResult struct {
	Content  string            `json:"content"`
	Warnings []SanitizeWarning `json:"warnings"`
	Stats    SanitizeStats     `json:"stats"`
}

// SanitizeStats carries byte-level metrics for observability.
type SanitizeStats struct {
	BytesIn    int   `json:"bytes_in"`
	BytesOut   int   `json:"bytes_out"`
	DurationUs int64 `json:"duration_us"`
}
