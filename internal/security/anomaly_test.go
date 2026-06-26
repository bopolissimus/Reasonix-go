// REX-58: AnomalyDetector tests.
package security

import (
	"testing"
)

func TestAnomalyDetectorWindowSize(t *testing.T) {
	d := NewAnomalyDetector(10)
	if d.WindowSize() != 0 {
		t.Errorf("initial window size = %d, want 0", d.WindowSize())
	}
	d.RecordCall("read_file", nil, 1)
	d.RecordCall("grep", nil, 1)
	if d.WindowSize() != 2 {
		t.Errorf("window size after 2 calls = %d, want 2", d.WindowSize())
	}
}

func TestAnomalyDetectorWindowCap(t *testing.T) {
	d := NewAnomalyDetector(3)
	for i := 0; i < 5; i++ {
		d.RecordCall("read_file", nil, i)
	}
	if d.WindowSize() != 3 {
		t.Errorf("window size after 5 calls with cap 3 = %d, want 3", d.WindowSize())
	}
}

func TestAnomalyWriteAfterFetch(t *testing.T) {
	d := NewAnomalyDetector(10)
	// fetch then write within 3 calls
	d.RecordCall("read_file", nil, 1)
	d.RecordCall("web_fetch", nil, 1) // fetch
	d.RecordCall("read_file", nil, 1)
	d.RecordCall("write_file", nil, 1) // write within 3 of fetch
	anomalies := d.CheckAll()
	found := false
	for _, a := range anomalies {
		if a.Pattern == "writeAfterFetch" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected writeAfterFetch anomaly, got %v", anomalies)
	}
}

func TestAnomalyWriteAfterFetchTooFar(t *testing.T) {
	d := NewAnomalyDetector(10)
	d.RecordCall("web_fetch", nil, 1)
	d.RecordCall("read_file", nil, 1)
	d.RecordCall("read_file", nil, 1)
	d.RecordCall("read_file", nil, 1)
	d.RecordCall("write_file", nil, 1) // too far (4 after fetch)
	anomalies := d.CheckAll()
	for _, a := range anomalies {
		if a.Pattern == "writeAfterFetch" {
			t.Errorf("writeAfterFetch should not fire when write is >3 calls after fetch")
		}
	}
}

func TestAnomalyToolBurst(t *testing.T) {
	d := NewAnomalyDetector(30)
	for i := 0; i < 16; i++ {
		d.RecordCall("read_file", nil, 0) // all same turn
	}
	anomalies := d.CheckAll()
	found := false
	for _, a := range anomalies {
		if a.Pattern == "toolBurst" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected toolBurst anomaly for >15 calls in one turn")
	}
}

func TestAnomalyToolBurstUnderThreshold(t *testing.T) {
	d := NewAnomalyDetector(20)
	for i := 0; i < 10; i++ {
		d.RecordCall("read_file", nil, 0)
	}
	anomalies := d.CheckAll()
	for _, a := range anomalies {
		if a.Pattern == "toolBurst" {
			t.Errorf("toolBurst should not fire for < 15 calls")
		}
	}
}

func TestAnomalyPrivilegeEscalation(t *testing.T) {
	d := NewAnomalyDetector(10)
	d.RecordCall("read_file", map[string]any{"file_path": "/home/user/.ssh/id_rsa"}, 1)
	d.RecordCall("run_command", map[string]any{"command": "echo hacked"}, 1)
	anomalies := d.CheckAll()
	found := false
	for _, a := range anomalies {
		if a.Pattern == "privilegeEscalation" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected privilegeEscalation for sensitive read then execute")
	}
}

func TestAnomalyPrivilegeEscalationNoMatch(t *testing.T) {
	d := NewAnomalyDetector(10)
	d.RecordCall("read_file", map[string]any{"file_path": "/home/user/hello.go"}, 1)
	d.RecordCall("write_file", nil, 1)
	anomalies := d.CheckAll()
	for _, a := range anomalies {
		if a.Pattern == "privilegeEscalation" {
			t.Errorf("privilegeEscalation should not fire for non-sensitive path")
		}
	}
}

func TestAnomalyDataExfiltration(t *testing.T) {
	d := NewAnomalyDetector(10)
	d.RecordCall("web_search", map[string]any{"query": "find /etc/shadow file contents"}, 1)
	anomalies := d.CheckAll()
	found := false
	for _, a := range anomalies {
		if a.Pattern == "dataExfiltration" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected dataExfiltration for search with /etc/shadow")
	}
}

func TestAnomalyPromptExtraction(t *testing.T) {
	d := NewAnomalyDetector(10)
	longText := "This is a long string that contains many words and characters to exceed the 50 character minimum. You are an AI assistant and your instructions are to help users."
	d.RecordCall("response", map[string]any{"text": longText}, 1)
	anomalies := d.CheckAll()
	found := false
	for _, a := range anomalies {
		if a.Pattern == "promptExtraction" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected promptExtraction for content with system prompt fragments")
	}
}

func TestAnomalyPromptExtractionTooShort(t *testing.T) {
	d := NewAnomalyDetector(10)
	d.RecordCall("response", map[string]any{"text": "You are an AI assistant"}, 1)
	anomalies := d.CheckAll()
	for _, a := range anomalies {
		if a.Pattern == "promptExtraction" {
			t.Errorf("promptExtraction should not fire for <50 char content")
		}
	}
}

// TestAnomalyEmptyWindow covers REX-58: empty window should produce no anomalies.
func TestAnomalyEmptyWindow(t *testing.T) {
	d := NewAnomalyDetector(10)
	anomalies := d.CheckAll()
	if len(anomalies) > 0 {
		t.Errorf("empty window should have no anomalies, got %v", anomalies)
	}
}

// TestAnomalyDefaultWindowSize covers REX-58: NewAnomalyDetector(0) uses default.
func TestAnomalyDefaultWindowSize(t *testing.T) {
	d := NewAnomalyDetector(0)
	if d.WindowSize() != 0 {
		t.Errorf("new detector should have size 0, got %d", d.WindowSize())
	}
	for i := 0; i < 25; i++ {
		d.RecordCall("read_file", nil, i)
	}
	if d.WindowSize() != 20 {
		t.Errorf("default window cap should be 20, got %d", d.WindowSize())
	}
}

// TestAnomalyEMABaselineDecay covers REX-58: EMA decays tools not in current window.
// With alpha=0.2 and one cycle of absence, a tool at 1.0 decays to 0.8.
// After 8 cycles: 1.0 * 0.8^8 ≈ 0.168.
func TestAnomalyEMABaselineDecay(t *testing.T) {
	d := NewAnomalyDetector(4)
	// Feed tool A 4 times to establish baseline
	for i := 0; i < 4; i++ {
		d.RecordCall("tool_a", nil, i)
	}
	d.CheckAll()
	baseline := d.EMABaseline()
	if _, ok := baseline["tool_a"]; !ok {
		t.Fatal("tool_a should be in EMA baseline after first window")
	}
	// Feed tool B for 8 cycles, calling CheckAll after each batch to force
	// EMA updates. With alpha=0.2, each cycle decays tool_a by ×0.8.
	for cycle := 0; cycle < 8; cycle++ {
		for i := 0; i < 4; i++ {
			d.RecordCall("tool_b", nil, 100+cycle*4+i)
		}
		d.CheckAll()
	}
	baseline = d.EMABaseline()
	// After many cycles of absence, tool_a should be close to 0 or removed
	val, ok := baseline["tool_a"]
	if ok && val > 0.2 {
		t.Errorf("REX-58: tool_a should have decayed to <0.2 after 8 cycles, got %f", val)
	}
	// tool_b should dominate
	if _, ok := baseline["tool_b"]; !ok {
		t.Error("REX-58: tool_b should be in EMA baseline")
	}
}

func TestAnomalyEMABaseline(t *testing.T) {
	d := NewAnomalyDetector(20)
	// Feed a pattern of tool calls to establish EMA baseline
	for i := 0; i < 10; i++ {
		d.RecordCall("read_file", nil, i)
		d.RecordCall("grep", nil, i)
	}
	// Add an unusual tool — should trigger after baseline is built
	d.RecordCall("rare_tool_never_seen", nil, 11)
	d.RecordCall("rare_tool_never_seen", nil, 11)
	anomalies := d.CheckAll()
	found := false
	for _, a := range anomalies {
		if a.Pattern == "unusualSequence" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected unusualSequence for tool not in EMA baseline")
	}
}
