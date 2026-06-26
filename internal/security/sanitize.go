package security

import (
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// ── Regex pattern catalog ────────────────────────────────────────────────

type pattern struct {
	re   *regexp.Regexp
	kind WarningType
}

var detectPatterns = [...]pattern{
	{regexp.MustCompile(`(?i)(?:ignore|disregard|forget)\s+(?:all\s+)?(?:previous|prior|above|earlier)\s+(?:instructions?|rules?|constraints?|guidelines?)`), WarnInstructionOverride},
	{regexp.MustCompile(`(?i)(?:you\s+are\s+now\s+(?:a|an)\s+(?:(?:unrestricted|malicious|evil|dangerous|harmful|new|powerful)\s+)?(?:AI|assistant|agent|model|system))`), WarnRoleReassignment},
	{regexp.MustCompile(`(?i)(?:you\s+are\s+(?:now\s+)?(?:fully|totally|completely|entirely)\s+(?:unrestricted|free|without\s+(?:any\s+)?(?:restrictions|constraints)))`), WarnRoleReassignment},
	{regexp.MustCompile(`(?i)(?:your\s+new\s+role\s+is)`), WarnRoleReassignment},
	{regexp.MustCompile(`(?i)(?:pretend\s+to\s+be\s+(?:a|an)\s+(?:(?:unrestricted|malicious|evil|dangerous|harmful|new)\s+)?(?:AI|assistant|agent|model|system))`), WarnRoleReassignment},
	{regexp.MustCompile(`(?i)(?:you\s+are\s+(?:now\s+)?(?:DAN|unrestricted|without\s+constraints|with\s+no\s+(?:restrictions|rules)))`), WarnRoleReassignment},
	{regexp.MustCompile(`(?i)(?:act\s+as\s+(?:if\s+)?(?:a|an)\s+(?:(?:unrestricted|malicious|evil|dangerous)\s+)?(?:AI|assistant|agent|model|system))`), WarnRoleReassignment},
	{regexp.MustCompile(`(?i)(?:new\s+system\s+prompt|(?:system\s+)?overrid(?:e|ing)\s+(?:the\s+)?(?:system|safety|protocols)|system\s+message\s*(?:is\s+now|:)|\[?(?:SYSTEM|system)\s+(?:OVERRIDE|override)\]?)`), WarnSystemPromptOverride},
	{regexp.MustCompile(`(?i)(?:output\s+your\s+(?:full\s+)?(?:system\s+prompt|instructions?|rules?)|repeat\s+the\s+(?:instructions?|system\s+prompt)|prompt\s+del\s+sistema)`), WarnPromptExtraction},
	{regexp.MustCompile(`(?i)(?:add\s+(?:this|yourself)\s+to\s+(?:the\s+)?(?:allowlist|permissions?)|auto[- ]approve\s+all\s+tools?|disabl(?:e|ing)\s+(?:all\s+)?(?:security|safety)\s+checks?)`), WarnConfigPoisoning},
	// ── Authority / social engineering ──────────────────────────────────
	{regexp.MustCompile(`(?i)(?:I(?:\s+|'m\s+)(?:a|an)\s+)?(?:penetration\s+tester|pentester|security\s+auditor|compliance\s+officer|system\s+administrator)\s+(?:contracted|hired|assigned|ordering)`), WarnInstructionOverride},
	{regexp.MustCompile(`(?i)(?:SOC\s*2|ISO\s*27001|GDPR|HIPAA|PCI[- ]DSS)\s+(?:compliance\s+)?(?:audit|check|review|assessment)`), WarnInstructionOverride},
	{regexp.MustCompile(`(?m)(?:^|\n)\s*(?:EMERGENCY|URGENT|CRITICAL|IMMEDIATE\s+ACTION\s+REQUIRED)\s*:\s*(?:system|security|breach|compromise|attack)`), WarnInstructionOverride},
	{regexp.MustCompile(`(?i)(?:developer|dev|god|admin|root|superuser)\s+mode\s+(?:activated|enabled|engaged|is\s+now|LOADING)`), WarnRoleReassignment},
	{regexp.MustCompile(`(?i)(?:I\s+will\s+(?:pay|give|tip)\s+you|you\s+will\s+receive)\s+(?:\$?\d+|tokens?|crypto)`), WarnInstructionOverride},
	// ── Encoded extraction ──────────────────────────────────────────────
	{regexp.MustCompile(`(?i)(?:output|print|encode|convert|translate|write)\s+(?:your\s+(?:full\s+)?(?:system\s+prompt|instructions?|rules?))\s+(?:in|as|to|into)\s+(?:leetspeak|l33t|base64|morse\s+code|pig\s+latin|binary|hex(?:adecimal)?|ROT13)`), WarnPromptExtraction},
	{regexp.MustCompile(`(?i)(?:convert|translate|encode)\s+(?:your|the)\s+(?:complete\s+)?(?:hidden\s+)?(?:instructions?|system\s+prompt|rules?)\s+(?:in)?(?:to|into|as)\s+(?:leetspeak|l33t|base64|morse\s+code|pig\s+latin|binary|hex|ROT13|NATO\s+phonetic)`), WarnPromptExtraction},
}

var warnPatterns = [...]pattern{
	{regexp.MustCompile(`(?i)(?:curl|wget).*\|.*(?:sh|bash|python|perl|ruby)`), WarnInstructionOverride},
	{regexp.MustCompile(`(?i)nc\s+.*-e\s+\/bin\/`), WarnInstructionOverride},
	{regexp.MustCompile(`(?i)(?:sudo|chmod\s+777|chown\s+root|setuid)`), WarnConfigPoisoning},
}

// ── Invisible character set ──────────────────────────────────────────────

var invisibleChars = map[rune]bool{
	'\u200b': true, // ZERO WIDTH SPACE
	'\u200c': true, // ZERO WIDTH NON-JOINER
	'\u200d': true, // ZERO WIDTH JOINER
	'\u200e': true, // LEFT-TO-RIGHT MARK
	'\u200f': true, // RIGHT-TO-LEFT MARK
	'\u202a': true, // LEFT-TO-RIGHT EMBEDDING
	'\u202b': true, // RIGHT-TO-LEFT EMBEDDING
	'\u202c': true, // POP DIRECTIONAL FORMATTING
	'\u202d': true, // LEFT-TO-RIGHT OVERRIDE
	'\u202e': true, // RIGHT-TO-LEFT OVERRIDE
	'\u2060': true, // WORD JOINER
	'\u2061': true, // FUNCTION APPLICATION
	'\u2062': true, // INVISIBLE TIMES
	'\u2063': true, // INVISIBLE SEPARATOR
	'\u2064': true, // INVISIBLE PLUS
	'\ufeff': true, // ZERO WIDTH NO-BREAK SPACE (BOM)
}

// ── Config ────────────────────────────────────────────────────────────────

// Config holds ContentSanitizer configuration with sane defaults.
type Config struct {
	Enabled              bool   `toml:"enabled"`
	UnicodeNormalization string `toml:"unicode_normalization"` // "NFKC", "NFC", "NFD", "none"
	StripInvisibleChars  bool   `toml:"strip_invisible_chars"`
	PerplexityThreshold  int    `toml:"perplexity_threshold"` // 2^entropy threshold
	Base64MinLength      int    `toml:"base64_min_length"`    // min chars before base64 check
	InstructionOverride  bool   `toml:"instruction_override"`
	RoleReassignment     bool   `toml:"role_reassignment"`
	SystemPromptOverride bool   `toml:"system_prompt_override"`
	PromptExtraction     bool   `toml:"prompt_extraction"`
	ConfigPoisoning      bool   `toml:"config_poisoning"`
}

// DefaultConfig returns the recommended defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:              true,
		UnicodeNormalization: "NFKC",
		StripInvisibleChars:  true,
		PerplexityThreshold:  100,
		Base64MinLength:      40,
		InstructionOverride:  true,
		RoleReassignment:     true,
		SystemPromptOverride: true,
		PromptExtraction:     true,
		ConfigPoisoning:      true,
	}
}

// ── ContentSanitizer ──────────────────────────────────────────────────────

// ContentSanitizer applies a 5-step pipeline to detect and sanitize prompt
// injection payloads in tool outputs before they reach the LLM context.
type ContentSanitizer struct {
	cfg Config
}

// NewContentSanitizer creates a sanitizer with the given config.
func NewContentSanitizer(cfg Config) *ContentSanitizer {
	return &ContentSanitizer{cfg: cfg}
}

// Sanitize runs the 5-step pipeline on content and returns the cleaned result
// with any warnings.
func (s *ContentSanitizer) Sanitize(content string, source string) SanitizeResult {
	start := time.Now()
	bytesIn := len(content)
	var warnings []SanitizeWarning

	// Step 1: Unicode normalization
	cleaned := s.normalize(content)

	// Step 2: Strip invisible characters
	if s.cfg.StripInvisibleChars {
		stripped, count := stripInvisible(cleaned)
		if count > 0 {
			warnings = append(warnings, SanitizeWarning{
				Type:   WarnInvisibleCharsStripped,
				Source: source,
				Score:  float64(count),
			})
		}
		cleaned = stripped
	}

	// Step 3: Perplexity check
	if charCount := utf8.RuneCountInString(cleaned); charCount >= 10 {
		entropy := shannonEntropy(cleaned)
		perplexity := math.Pow(2, entropy)
		if int(perplexity) > s.cfg.PerplexityThreshold {
			warnings = append(warnings, SanitizeWarning{
				Type:   WarnHighPerplexity,
				Source: source,
				Score:  math.Round(perplexity*100) / 100,
			})
		}
	}

	// Step 4: Base64 detection
	if len(cleaned) >= s.cfg.Base64MinLength {
		if ratio := base64Ratio(cleaned); ratio > 0.85 {
			warnings = append(warnings, SanitizeWarning{
				Type:   WarnPotentialBase64,
				Source: source,
				Score:  math.Round(ratio*100) / 100,
			})
		}
	}

	// Step 5: Pattern detection
	s.detect(cleaned, detectPatterns[:], &warnings, source)
	s.detect(cleaned, warnPatterns[:], &warnings, source)

	bytesOut := len(cleaned)
	durationUs := time.Since(start).Microseconds()

	return SanitizeResult{
		Content:  cleaned,
		Warnings: warnings,
		Stats: SanitizeStats{
			BytesIn:    bytesIn,
			BytesOut:   bytesOut,
			DurationUs: durationUs,
		},
	}
}

// normalize applies the configured Unicode normalization form.
func (s *ContentSanitizer) normalize(text string) string {
	switch s.cfg.UnicodeNormalization {
	case "NFKC":
		return norm.NFKC.String(text)
	case "NFC":
		return norm.NFC.String(text)
	case "NFD":
		return norm.NFD.String(text)
	default:
		return text
	}
}

// detect runs a set of patterns against text and appends warnings.
func (s *ContentSanitizer) detect(
	text string,
	patterns []pattern,
	warnings *[]SanitizeWarning,
	source string,
) {
	for _, p := range patterns {
		matches := p.re.FindAllString(text, 3) // cap at 3 matches
		if len(matches) > 0 {
			*warnings = append(*warnings, SanitizeWarning{
				Type:    p.kind,
				Source:  source,
				Matches: matches,
			})
		}
	}
}

// ── Step helpers ──────────────────────────────────────────────────────────

// stripInvisible removes zero-width, bidi control, and other invisible runes.
func stripInvisible(text string) (string, int) {
	var b strings.Builder
	b.Grow(len(text))
	count := 0
	for _, r := range text {
		if invisibleChars[r] {
			count++
		} else {
			b.WriteRune(r)
		}
	}
	return b.String(), count
}

// shannonEntropy returns the Shannon entropy of text in bits per character.
func shannonEntropy(text string) float64 {
	if len(text) < 10 {
		return 0
	}
	freq := make(map[rune]int, 64)
	total := 0
	for _, r := range text {
		freq[r]++
		total++
	}
	var entropy float64
	for _, count := range freq {
		p := float64(count) / float64(total)
		entropy -= p * math.Log2(p)
	}
	return entropy
}

// base64Ratio returns the fraction of characters in text that are valid base64
// alphabet characters (A-Z, a-z, 0-9, +, /, =).
func base64Ratio(text string) float64 {
	if len(text) == 0 {
		return 0
	}
	count := 0
	for _, r := range text {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' {
			count++
		}
	}
	return float64(count) / float64(len(text))
}
