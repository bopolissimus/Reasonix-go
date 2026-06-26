// Package shell parses and analyzes shell command lines for safety checking.
// Built on mvdan.cc/sh/v3/syntax for AST parsing.
//
// REX-61: Shell pipeline splitting — replace substring-matching metacharacter
// blocking with AST-based command analysis.
package shell

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Parse parses a shell command string into a syntax.File AST.
// A parse error returns nil + error — callers should treat parse failures
// as blocked commands (fail-closed).
func Parse(cmd string) (*syntax.File, error) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil, fmt.Errorf("empty command")
	}
	f, err := syntax.NewParser().Parse(strings.NewReader(cmd), "")
	if err != nil {
		return nil, fmt.Errorf("shell parse error: %w", err)
	}
	return f, nil
}
