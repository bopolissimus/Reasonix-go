package security

import (
	"strings"
	"testing"
)

func TestDataNonceLength(t *testing.T) {
	n := DataNonce()
	if len(n) != 8 {
		t.Fatalf("DataNonce length = %d, want 8", len(n))
	}
}

func TestDataNonceUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		n := DataNonce()
		if seen[n] {
			t.Fatalf("DataNonce collision after %d calls: %q", i, n)
		}
		seen[n] = true
	}
}

func TestDataNonceFormat(t *testing.T) {
	n := DataNonce()
	for _, c := range n {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("DataNonce contains non-hex char: %q", n)
		}
	}
}

func TestDataNonceSafe(t *testing.T) {
	// DataNonceSafe should return a nonce that does NOT appear in the content.
	for i := 0; i < 100; i++ {
		content := "some sample output with no matching hex chars"
		n := DataNonceSafe(content)
		if strings.Contains(content, n) {
			t.Fatalf("DataNonceSafe returned %q which appears in content %q", n, content)
		}
	}
}

func TestDataNonceSafeHandlesCollision(t *testing.T) {
	// Generate a nonce, then pass it as "content" — DataNonceSafe must
	// re-roll and return a different one.
	n1 := DataNonce()
	n2 := DataNonceSafe(n1)
	if n1 == n2 {
		t.Fatalf("DataNonceSafe should return a different nonce when content contains it")
	}
	if strings.Contains(n1, n2) || strings.Contains(n2, n1) {
		t.Fatalf("DataNonceSafe returned %q which appears in content %q", n2, n1)
	}
}
