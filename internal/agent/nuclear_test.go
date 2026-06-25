package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/event"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// fakeNukeTool is a minimal tool that records whether executeOne reached the
// Execute phase, letting us verify NUCLEAR-YOLO blocks before execution.
type fakeNukeTool struct {
	executed *bool
}

func (f fakeNukeTool) Name() string            { return "bash" }
func (f fakeNukeTool) Description() string     { return "fake bash" }
func (f fakeNukeTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (f fakeNukeTool) ReadOnly() bool          { return false }
func (f fakeNukeTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	*f.executed = true
	return "executed", nil
}

// TestNuclearYoloBlocksGitCommands covers REX-64.
func TestNuclearYoloBlocksGitCommands(t *testing.T) {
	executed := false
	reg := tool.NewRegistry()
	reg.Add(fakeNukeTool{executed: &executed})

	a := New(nil, reg, NewSession("nuke"), Options{}, event.Discard)
	a.SetNuclearYolo(true)

	blockedCmds := []string{
		"git push origin main",
		"git push",
		"git commit -m test",
		"git add .",
	}

	for _, cmd := range blockedCmds {
		executed = false
		args := json.RawMessage(`{"command":"` + cmd + `"}`)
		outcome := a.executeOne(t.Context(), provider.ToolCall{
			Name:      "bash",
			Arguments: string(args),
		})
		if executed {
			t.Errorf("REX-64: %q should have been blocked before execution", cmd)
		}
		if !outcome.blocked {
			t.Errorf("REX-64: %q should be blocked, got output=%q", cmd, outcome.output)
		}
		if !strings.Contains(outcome.errMsg, "NUCLEAR-YOLO") {
			t.Errorf("REX-64: wrong errMsg for %q: %q", cmd, outcome.errMsg)
		}
	}
}

// TestNuclearYoloAllowsSafeGitCommands covers REX-64.
func TestNuclearYoloAllowsSafeGitCommands(t *testing.T) {
	executed := false
	reg := tool.NewRegistry()
	reg.Add(fakeNukeTool{executed: &executed})

	a := New(nil, reg, NewSession("nuke"), Options{}, event.Discard)
	a.SetNuclearYolo(true)

	safeCmds := []string{
		"git status",
		"git diff",
		"git log --oneline",
		"ls -la",
		"echo hello",
	}

	for _, cmd := range safeCmds {
		executed = false
		args := json.RawMessage(`{"command":"` + cmd + `"}`)
		outcome := a.executeOne(t.Context(), provider.ToolCall{
			Name:      "bash",
			Arguments: string(args),
		})
		if !executed {
			t.Errorf("REX-64: %q should have been allowed to execute", cmd)
		}
		if outcome.blocked {
			t.Errorf("REX-64: %q should not be blocked, got: %s", cmd, outcome.errMsg)
		}
	}
}

// TestNuclearYoloOffGitPushAllowed covers REX-64.
func TestNuclearYoloOffGitPushAllowed(t *testing.T) {
	executed := false
	reg := tool.NewRegistry()
	reg.Add(fakeNukeTool{executed: &executed})

	a := New(nil, reg, NewSession("nuke"), Options{}, event.Discard)

	args := json.RawMessage(`{"command":"git push origin main"}`)
	outcome := a.executeOne(t.Context(), provider.ToolCall{
		Name:      "bash",
		Arguments: string(args),
	})
	if !executed {
		t.Error("REX-64: when off, git push should execute")
	}
	if outcome.blocked && strings.Contains(outcome.errMsg, "NUCLEAR-YOLO") {
		t.Errorf("REX-64: when off, should not be NUCLEAR-YOLO blocked: %s", outcome.errMsg)
	}
}

// TestBashCommandExtraction covers REX-64 helper.
func TestBashCommandExtraction(t *testing.T) {
	a := New(nil, nil, nil, Options{}, nil)

	tests := []struct {
		args json.RawMessage
		want string
	}{
		{json.RawMessage(`{"command":"git push"}`), "git push"},
		{json.RawMessage(`{"command":"ls -la"}`), "ls -la"},
		{json.RawMessage(`{}`), ""},
		{json.RawMessage(`{"other":"value"}`), ""},
		{json.RawMessage(`invalid`), ""},
	}

	for _, tt := range tests {
		got := a.bashCommand(tt.args)
		if got != tt.want {
			t.Errorf("REX-64: bashCommand(%s) = %q, want %q", tt.args, got, tt.want)
		}
	}
}
