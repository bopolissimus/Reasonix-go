package security

import (
	"encoding/base64"
	"regexp"
	"strings"
)

// OutputValidationConfig holds configuration for output content scanning.
type OutputValidationConfig struct {
	Enabled      bool
	BlockURLs    bool // block unexpected URLs in output
	BlockHTML    bool // block raw HTML/script tags
	BlockEncoded bool // block base64/hex encoded blobs
	URLAllowlist []string
}

// OutputValidationResult is the result of scanning an LLM output.
type OutputValidationResult struct {
	Clean    bool
	Findings []OutputFinding
}

// OutputFinding records a suspicious pattern in model output.
type OutputFinding struct {
	Type    string
	Content string // the matched text, truncated
}

var (
	urlPattern    = regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`)
	htmlPattern   = regexp.MustCompile(`(?i)<(script|iframe|object|embed|form|input|meta|link)[^>]*>`)
	base64Pattern = regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`)
)

// ValidateOutput scans LLM output for suspicious content.
// REX-89: Output validation / content allowlist.
func ValidateOutput(output string, cfg OutputValidationConfig) OutputValidationResult {
	if !cfg.Enabled || output == "" {
		return OutputValidationResult{Clean: true}
	}

	var findings []OutputFinding

	// Check for raw HTML/script injection
	if cfg.BlockHTML {
		if m := htmlPattern.FindString(output); m != "" {
			findings = append(findings, OutputFinding{
				Type:    "raw_html",
				Content: truncateOutput(m, 100),
			})
		}
	}

	// Check for unexpected URLs
	if cfg.BlockURLs {
		urls := urlPattern.FindAllString(output, 10)
		for _, u := range urls {
			if !isAllowedURL(u, cfg.URLAllowlist) {
				findings = append(findings, OutputFinding{
					Type:    "unexpected_url",
					Content: truncateOutput(u, 100),
				})
				break // one unexpected URL is enough to flag
			}
		}
	}

	// Check for encoded data blobs
	if cfg.BlockEncoded {
		if b64 := base64Pattern.FindString(output); b64 != "" {
			// Verify it actually decodes to confirm it's not a coincidence
			if _, err := base64.StdEncoding.DecodeString(b64); err == nil {
				findings = append(findings, OutputFinding{
					Type:    "encoded_blob",
					Content: truncateOutput(b64, 50),
				})
			}
		}
	}

	if len(findings) > 0 {
		return OutputValidationResult{Clean: false, Findings: findings}
	}
	return OutputValidationResult{Clean: true}
}

func isAllowedURL(url string, allowlist []string) bool {
	if len(allowlist) == 0 {
		return true // no allowlist = allow all
	}
	lower := strings.ToLower(url)
	for _, allowed := range allowlist {
		if strings.Contains(lower, strings.ToLower(allowed)) {
			return true
		}
	}
	return false
}

func truncateOutput(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
