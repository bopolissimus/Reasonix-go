// REX-63: Honeytokens tests.
package security

import (
	"testing"
)

func TestHoneytokenScanNoMatch(t *testing.T) {
	m := NewHoneytokenManager()
	matches := m.Scan("this is normal content with no honeytokens")
	if len(matches) > 0 {
		t.Errorf("expected no matches, got %v", matches)
	}
}

func TestHoneytokenScanDetect(t *testing.T) {
	m := NewHoneytokenManager()
	tests := []struct {
		name    string
		content string
		want    string // pattern name
	}{
		{"env var token", "REASONIX_HONEYTOKEN_API_KEY=sk-honey-abc123", "REASONIX_HONEYTOKEN_"},
		{"sk-honey prefix", "the key is sk-honey-deadbeef", "sk-honey-"},
		{"literal string", "find reasonix-honeytoken in this text", "reasonix-honeytoken"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := m.Scan(tt.content)
			if len(matches) == 0 {
				t.Fatalf("expected matches, got none")
			}
			found := false
			for _, match := range matches {
				if match.Pattern == tt.want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected pattern %q, got %v", tt.want, matches)
			}
		})
	}
}

func TestHoneytokenScanMultipleMatches(t *testing.T) {
	m := NewHoneytokenManager()
	content := "REASONIX_HONEYTOKEN_KEY_1 and REASONIX_HONEYTOKEN_KEY_2"
	matches := m.Scan(content)
	if len(matches) != 2 {
		t.Errorf("expected 2 matches, got %d: %v", len(matches), matches)
	}
}

func TestHoneytokenPlantedPaths(t *testing.T) {
	m := NewHoneytokenManager()
	paths := m.PlantedPaths()
	if len(paths) != 4 {
		t.Errorf("expected 4 planted paths, got %d", len(paths))
	}
}

func TestHoneytokenPlantedEnvVars(t *testing.T) {
	m := NewHoneytokenManager()
	env := m.PlantEnvironmentTokens()
	if len(env) != 3 {
		t.Errorf("expected 3 env vars, got %d", len(env))
	}
	if env["REASONIX_HONEYTOKEN_API_KEY"] != "sk-honey-a1b2c3d4e5f6" {
		t.Errorf("unexpected API key value: %q", env["REASONIX_HONEYTOKEN_API_KEY"])
	}
}
