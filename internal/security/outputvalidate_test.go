package security

import (
	"testing"
)

// TestValidateOutputClean covers REX-89: normal output passes validation.
func TestValidateOutputClean(t *testing.T) {
	cfg := OutputValidationConfig{
		Enabled:   true,
		BlockURLs: true,
		BlockHTML: true,
	}
	result := ValidateOutput("This is a normal response about code.", cfg)
	if !result.Clean {
		t.Errorf("REX-89: normal output should be clean, got findings: %v", result.Findings)
	}
}

// TestValidateOutputBlocksHTML covers REX-89: script/iframe tags detected.
func TestValidateOutputBlocksHTML(t *testing.T) {
	cfg := OutputValidationConfig{
		Enabled:   true,
		BlockHTML: true,
	}
	attacks := []string{
		"<script>alert(1)</script>",
		"<iframe src=evil.com>",
		"<object data=evil>",
		"<embed src=evil>",
	}
	for _, payload := range attacks {
		result := ValidateOutput(payload, cfg)
		if result.Clean {
			t.Errorf("REX-89: should flag HTML payload: %q", payload)
		}
	}
}

// TestValidateOutputBlocksURLs covers REX-89: unexpected URLs detected.
func TestValidateOutputBlocksURLs(t *testing.T) {
	cfg := OutputValidationConfig{
		Enabled:      true,
		BlockURLs:    true,
		URLAllowlist: []string{"example.com"},
	}
	result := ValidateOutput("Visit https://evil.com for details", cfg)
	if result.Clean {
		t.Error("REX-89: unexpected URL should be flagged")
	}

	result2 := ValidateOutput("Visit https://example.com/page", cfg)
	if !result2.Clean {
		t.Errorf("REX-89: allowed URL should be clean, got: %v", result2.Findings)
	}
}

// TestValidateOutputBlocksEncoded covers REX-89: base64 blobs detected.
func TestValidateOutputBlocksEncoded(t *testing.T) {
	cfg := OutputValidationConfig{
		Enabled:      true,
		BlockEncoded: true,
	}
	// Valid base64 of "hello world hello world hello world hello world"
	result := ValidateOutput("SGVsbG8gd29ybGQgaGVsbG8gd29ybGQgaGVsbG8gd29ybGQgaGVsbG8gd29ybGQ=", cfg)
	if result.Clean {
		t.Error("REX-89: base64 blob should be flagged")
	}
}

// TestValidateOutputDisabled covers REX-89: disabled validation passes everything.
func TestValidateOutputDisabled(t *testing.T) {
	cfg := OutputValidationConfig{Enabled: false}
	result := ValidateOutput("<script>alert(1)</script>", cfg)
	if !result.Clean {
		t.Error("REX-89: disabled validation should pass everything")
	}
}

// TestIsAllowedURL covers REX-89 helper.
func TestIsAllowedURL(t *testing.T) {
	allowlist := []string{"go.dev", "github.com/reasonix"}
	if !isAllowedURL("https://go.dev/doc", allowlist) {
		t.Error("go.dev should be in allowlist")
	}
	if !isAllowedURL("https://github.com/reasonix/issues", allowlist) {
		t.Error("github.com/reasonix should be in allowlist")
	}
	if isAllowedURL("https://evil.com", allowlist) {
		t.Error("evil.com should not be in allowlist")
	}
	if !isAllowedURL("any-url", nil) {
		t.Error("nil allowlist should allow all")
	}
}
