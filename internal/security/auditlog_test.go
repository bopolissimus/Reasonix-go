// REX-59: AuditLog tests.
package security

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAuditLogEnabled(t *testing.T) {
	l := NewAuditLog(true)
	l.Log(AuditEvent{Event: AuditToolExecute, Tool: "read_file"})
	if l.Count() != 1 {
		t.Errorf("count = %d, want 1", l.Count())
	}
}

func TestAuditLogDisabled(t *testing.T) {
	l := NewAuditLog(false)
	l.Log(AuditEvent{Event: AuditToolExecute, Tool: "read_file"})
	if l.Count() != 0 {
		t.Errorf("disabled log should have count 0, got %d", l.Count())
	}
}

func TestAuditLogQuery(t *testing.T) {
	l := NewAuditLog(true)
	for i := 0; i < 5; i++ {
		l.Log(AuditEvent{Event: AuditToolExecute, Tool: "read_file"})
	}
	events := l.Query(3)
	if len(events) != 3 {
		t.Errorf("query(3) = %d events, want 3", len(events))
	}
}

func TestAuditLogQueryByEvent(t *testing.T) {
	l := NewAuditLog(true)
	l.Log(AuditEvent{Event: AuditToolExecute, Tool: "read_file"})
	l.Log(AuditEvent{Event: AuditToolBlocked, Tool: "write_file", Reason: "plan mode"})
	l.Log(AuditEvent{Event: AuditToolExecute, Tool: "grep"})
	events := l.QueryByEvent(AuditToolBlocked, 10)
	if len(events) != 1 {
		t.Errorf("QueryByEvent(blocked) = %d, want 1", len(events))
	}
	if events[0].Reason != "plan mode" {
		t.Errorf("reason = %q, want 'plan mode'", events[0].Reason)
	}
}

func TestAuditLogRedaction(t *testing.T) {
	l := NewAuditLog(true)
	// API key should be redacted
	params := json.RawMessage(`{"api_key": "sk-12345678901234567890123456789012"}`)
	l.Log(AuditEvent{Event: AuditToolExecute, Tool: "web_fetch", Params: params})
	events := l.Query(1)
	if strings.Contains(string(events[0].Params), "sk-123") {
		t.Errorf("API key not redacted: %s", string(events[0].Params))
	}
	if !strings.Contains(string(events[0].Params), "REDACTED") {
		t.Errorf("expected REDACTED marker, got: %s", string(events[0].Params))
	}
}

func TestAuditLogRedactGitHubToken(t *testing.T) {
	l := NewAuditLog(true)
	params := json.RawMessage(`{"token": "ghp_abcdefghijklmnopqrstuvwxyz1234567890"}`)
	l.Log(AuditEvent{Event: AuditToolExecute, Tool: "bash", Params: params})
	events := l.Query(1)
	if strings.Contains(string(events[0].Params), "ghp_") {
		t.Errorf("GitHub token not redacted: %s", string(events[0].Params))
	}
}

func TestAuditLogRedactEmail(t *testing.T) {
	l := NewAuditLog(true)
	params := json.RawMessage(`{"email": "user@example.com"}`)
	l.Log(AuditEvent{Event: AuditSessionStart, Params: params})
	events := l.Query(1)
	if strings.Contains(string(events[0].Params), "user@example.com") {
		t.Errorf("email not redacted: %s", string(events[0].Params))
	}
}

func TestAuditLogClear(t *testing.T) {
	l := NewAuditLog(true)
	l.Log(AuditEvent{Event: AuditToolExecute, Tool: "read_file"})
	l.Clear()
	if l.Count() != 0 {
		t.Errorf("count after clear = %d, want 0", l.Count())
	}
	if l.BytesWritten() != 0 {
		t.Errorf("bytes after clear = %d, want 0", l.BytesWritten())
	}
}

func TestAuditLogBytesWritten(t *testing.T) {
	l := NewAuditLog(true)
	l.Log(AuditEvent{Event: AuditToolExecute, Tool: "read_file"})
	if l.BytesWritten() == 0 {
		t.Errorf("bytesWritten should be > 0 after logging")
	}
}

// TestAuditLogQueryByEventLimit covers REX-59: QueryByEvent respects limit.
func TestAuditLogQueryByEventLimit(t *testing.T) {
	l := NewAuditLog(true)
	for i := 0; i < 10; i++ {
		l.Log(AuditEvent{Event: AuditToolExecute, Tool: "tool"})
	}
	events := l.QueryByEvent(AuditToolExecute, 3)
	if len(events) != 3 {
		t.Errorf("QueryByEvent with limit 3 returned %d events", len(events))
	}
}

// TestAuditLogQueryAll covers REX-59: Query(-1) returns all events.
func TestAuditLogQueryAll(t *testing.T) {
	l := NewAuditLog(true)
	for i := 0; i < 100; i++ {
		l.Log(AuditEvent{Event: AuditToolExecute, Tool: "tool"})
	}
	if l.Count() != 100 {
		t.Errorf("count = %d, want 100", l.Count())
	}
	all := l.Query(-1)
	if len(all) != 100 {
		t.Errorf("Query(-1) = %d events, want 100", len(all))
	}
}

// TestAuditLogNoRedactOnEmptyParams covers REX-59: nil params handled gracefully.
func TestAuditLogNoRedactOnEmptyParams(t *testing.T) {
	l := NewAuditLog(true)
	l.Log(AuditEvent{Event: AuditToolExecute, Tool: "read_file"})
	events := l.Query(1)
	if events[0].Tool != "read_file" {
		t.Errorf("tool = %q, want 'read_file'", events[0].Tool)
	}
}
