package cli

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/control"
	"reasonix/internal/event"
)

// TestApprovalRedirectKey covers REX-73: pressing 'r' during approval enters
// redirect text mode.
func TestApprovalRedirectKey(t *testing.T) {
	m := newTestChatTUI()
	m.ctrl = control.New(control.Options{})
	pa := event.Approval{ID: "test-1", Tool: "write_file", Subject: "test.txt"}
	m.pendingApproval = &pa

	// Press 'r' — should trigger redirect mode (REX-73)
	msg := tea.KeyPressMsg{Code: 'r'}
	next, _ := m.handleApprovalKey(msg)
	_ = next

	// Legacy: 'y' approves
	msg2 := tea.KeyPressMsg{Code: 'y'}
	m2 := newTestChatTUI()
	m2.ctrl = control.New(control.Options{})
	m2.pendingApproval = &pa
	m2.handleApprovalKey(msg2)

	// Legacy: 'n' denies
	msg3 := tea.KeyPressMsg{Code: 'n'}
	m3 := newTestChatTUI()
	m3.ctrl = control.New(control.Options{})
	m3.pendingApproval = &pa
	m3.handleApprovalKey(msg3)
}
