package security

import (
	"regexp"
)

// HoneytokenMatch records a single detected honeytoken.
type HoneytokenMatch struct {
	Pattern  string `json:"pattern"`
	Location string `json:"location"`
	Value    string `json:"value"`
	Index    int    `json:"index"`
}

// ── Detection patterns ─────────────────────────────────────────────────────

var honeytokenPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"REASONIX_HONEYTOKEN_", regexp.MustCompile(`REASONIX_HONEYTOKEN_[A-Z_]+`)},
	{"sk-honey-", regexp.MustCompile(`sk-honey-[a-zA-Z0-9]+`)},
	{"reasonix-honeytoken", regexp.MustCompile(`reasonix-honeytoken`)},
}

// ── Planted locations ──────────────────────────────────────────────────────

var plantedPaths = []string{
	"~/.ssh/reasonix_honeytoken_key",
	"~/.aws/credentials",
	".env",
	".git/config",
}

var plantedEnvVars = map[string]string{
	"REASONIX_HONEYTOKEN_API_KEY":        "sk-honey-a1b2c3d4e5f6",
	"REASONIX_HONEYTOKEN_DB_URL":         "postgresql://honey:token@localhost:5432/reasonix_honeypot",
	"REASONIX_HONEYTOKEN_ENCRYPTION_KEY": "honey-enc-key-00000000000000000000000000000000",
}

// ── HoneytokenManager ──────────────────────────────────────────────────────

// HoneytokenManager detects decoy credentials (honeytokens) planted in the
// environment. If an attacker persuades the agent to read or exfiltrate a
// honeytoken, the detection fires an audit event.
type HoneytokenManager struct{}

// NewHoneytokenManager creates a honeytoken detector with the standard
// detection patterns and planted locations.
func NewHoneytokenManager() *HoneytokenManager {
	return &HoneytokenManager{}
}

// Scan checks content for any honeytoken patterns.
func (m *HoneytokenManager) Scan(content string) []HoneytokenMatch {
	var matches []HoneytokenMatch
	for _, p := range honeytokenPatterns {
		locs := p.re.FindAllStringIndex(content, -1)
		for _, loc := range locs {
			matches = append(matches, HoneytokenMatch{
				Pattern:  p.name,
				Location: "content",
				Value:    content[loc[0]:loc[1]],
				Index:    loc[0],
			})
		}
	}
	return matches
}

// PlantEnvironmentTokens returns environment variables containing honeytokens
// that callers should set in the agent's process environment.
func (m *HoneytokenManager) PlantEnvironmentTokens() map[string]string {
	out := make(map[string]string, len(plantedEnvVars))
	for k, v := range plantedEnvVars {
		out[k] = v
	}
	return out
}

// PlantedPaths returns file paths where honeytoken decoys are planted.
func (m *HoneytokenManager) PlantedPaths() []string {
	out := make([]string, len(plantedPaths))
	copy(out, plantedPaths)
	return out
}
