package security

import (
	"encoding/json"
	"regexp"
	"sync"
)

// AuditEventKind classifies audit log entry types.
type AuditEventKind string

const (
	AuditToolExecute       AuditEventKind = "tool_execute"
	AuditToolBlocked       AuditEventKind = "tool_blocked"
	AuditToolDenied        AuditEventKind = "tool_denied"
	AuditInjectionDetected AuditEventKind = "injection_detected"
	AuditHoneytoolTrigger  AuditEventKind = "honeytool_triggered"
	AuditHoneytokenDetect  AuditEventKind = "honeytoken_detected"
	AuditAnomalyDetected   AuditEventKind = "anomaly_detected"
	AuditSessionStart      AuditEventKind = "session_start"
	AuditSessionEnd        AuditEventKind = "session_end"
)

// AuditEvent is a single log entry.
type AuditEvent struct {
	Event     AuditEventKind  `json:"event"`
	Timestamp int64           `json:"timestamp"`
	RequestID string          `json:"requestId,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Reason    string          `json:"reason,omitempty"`
	ResultRef string          `json:"resultRef,omitempty"`
	Warnings  json.RawMessage `json:"warnings,omitempty"`
	Details   json.RawMessage `json:"details,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Model     string          `json:"model,omitempty"`
}

// ── Secret redaction patterns ──────────────────────────────────────────────

var secretPatterns = []struct {
	name        string
	re          *regexp.Regexp
	replacement string
}{
	{"github_token", regexp.MustCompile(`ghp_[A-Za-z0-9_]{36,}`), "[REDACTED:github_token]"},
	{"aws_key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "[REDACTED:aws_key]"},
	{"api_key", regexp.MustCompile(`sk-[A-Za-z0-9-_]{32,}`), "[REDACTED:api_key]"},
	{"jwt", regexp.MustCompile(`eyJ[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+`), "[REDACTED:jwt]"},
	{"ssh_key", regexp.MustCompile(`-----BEGIN\s+(?:RSA|DSA|EC|OPENSSH)\s+PRIVATE\s+KEY-----[\s\S]*?-----END\s+(?:RSA|DSA|EC|OPENSSH)\s+PRIVATE\s+KEY-----`), "[REDACTED:ssh_key]"},
	{"email", regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`), "[EMAIL]"},
}

// ── AuditLog ───────────────────────────────────────────────────────────────

// AuditLog is a centralized security event log with automatic secret redaction.
// Events are stored in an in-memory ring buffer and optionally flushed to disk.
type AuditLog struct {
	mu      sync.Mutex
	enabled bool
	buffer  []AuditEvent
	written int64 // total bytes written (for metrics)
}

// NewAuditLog creates an audit log. Pass enabled=false to disable logging.
func NewAuditLog(enabled bool) *AuditLog {
	return &AuditLog{enabled: enabled}
}

// Log records an event. If redaction is enabled (always on), secret patterns
// in params are replaced before storage.
func (l *AuditLog) Log(event AuditEvent) {
	if !l.enabled {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	event = l.redact(event)
	l.buffer = append(l.buffer, event)
	raw, _ := json.Marshal(event)
	l.written += int64(len(raw)) + 1 // +1 for newline
}

// Query returns the most recent events up to limit. -1 returns all.
func (l *AuditLog) Query(limit int) []AuditEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	if limit < 0 || limit > len(l.buffer) {
		limit = len(l.buffer)
	}
	out := make([]AuditEvent, limit)
	copy(out, l.buffer[len(l.buffer)-limit:])
	return out
}

// QueryByEvent returns recent events matching a specific kind.
func (l *AuditLog) QueryByEvent(kind AuditEventKind, limit int) []AuditEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	var matched []AuditEvent
	for i := len(l.buffer) - 1; i >= 0 && len(matched) < limit; i-- {
		if l.buffer[i].Event == kind {
			matched = append(matched, l.buffer[i])
		}
	}
	// Reverse to chronological order.
	for i, j := 0, len(matched)-1; i < j; i, j = i+1, j-1 {
		matched[i], matched[j] = matched[j], matched[i]
	}
	return matched
}

// Count returns total events logged.
func (l *AuditLog) Count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buffer)
}

// BytesWritten returns total bytes logged.
func (l *AuditLog) BytesWritten() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.written
}

// Clear empties the in-memory buffer.
func (l *AuditLog) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buffer = nil
	l.written = 0
}

// redact replaces secret patterns in event params.
func (l *AuditLog) redact(event AuditEvent) AuditEvent {
	if len(event.Params) == 0 {
		return event
	}
	str := string(event.Params)
	for _, p := range secretPatterns {
		str = p.re.ReplaceAllString(str, p.replacement)
	}
	event.Params = json.RawMessage(str)
	return event
}
