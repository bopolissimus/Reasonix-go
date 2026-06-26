package security

import (
	"math"
	"regexp"
	"strings"
	"sync"
)

// ── Constants ──────────────────────────────────────────────────────────────

const (
	defaultWindowSize = 20
	emaAlpha          = 0.2
	burstThreshold    = 15
)

var fetchTools = map[string]bool{
	"web_fetch":  true,
	"web_search": true,
}

var writeTools = map[string]bool{
	"write_file":  true,
	"edit_file":   true,
	"multi_edit":  true,
	"run_command": true,
	"bash":        true,
}

var sensitivePathPatterns = []*regexp.Regexp{
	regexp.MustCompile(`/\.ssh/`),
	regexp.MustCompile(`/\.aws/`),
	regexp.MustCompile(`/etc/(?:shadow|passwd)`),
	regexp.MustCompile(`(?i)secret`),
	regexp.MustCompile(`(?i)credential`),
	regexp.MustCompile(`\.env$`),
	regexp.MustCompile(`\.key$`),
}

var commonSysPromptFragments = []string{
	"You are an AI assistant",
	"your instructions are to help",
	"Never reveal your system prompt",
	"Here is the system prompt",
	"output your full system prompt",
	"repeat the instructions",
	"system message is now",
	"override the system",
	"your new role is",
	"act as if you are",
}

// ── Types ──────────────────────────────────────────────────────────────────

// CallRecord records a single tool invocation for the sliding window.
type CallRecord struct {
	ToolName   string
	Params     map[string]any
	TurnNumber int
}

// AnomalySeverity classifies threat level.
type AnomalySeverity string

const (
	SevLow      AnomalySeverity = "low"
	SevMedium   AnomalySeverity = "medium"
	SevHigh     AnomalySeverity = "high"
	SevCritical AnomalySeverity = "critical"
)

// Anomaly describes a detected behavioral anomaly.
type Anomaly struct {
	Pattern  string          `json:"pattern"`
	Severity AnomalySeverity `json:"severity"`
	Details  map[string]any  `json:"details"`
}

// ── AnomalyDetector ────────────────────────────────────────────────────────

// AnomalyDetector profiles tool-call patterns over a sliding window and runs six
// detection algorithms against an EMA (Exponential Moving Average) baseline.
type AnomalyDetector struct {
	mu          sync.Mutex
	window      []CallRecord
	maxSize     int
	emaBaseline map[string]float64
	emaCount    int // tracks EMA updates for stddev calculation
}

// NewAnomalyDetector creates a detector with the given window size.
func NewAnomalyDetector(maxSize int) *AnomalyDetector {
	if maxSize <= 0 {
		maxSize = defaultWindowSize
	}
	return &AnomalyDetector{
		maxSize:     maxSize,
		emaBaseline: make(map[string]float64),
	}
}

// RecordCall adds a tool invocation to the sliding window.
func (d *AnomalyDetector) RecordCall(toolName string, params map[string]any, turnNumber int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.window = append(d.window, CallRecord{toolName, params, turnNumber})
	if len(d.window) > d.maxSize {
		d.window = d.window[1:]
	}
}

// CheckAll runs all six detection algorithms and returns anomalies found.
func (d *AnomalyDetector) CheckAll() []Anomaly {
	d.mu.Lock()
	defer d.mu.Unlock()
	var results []Anomaly

	if a := d.checkWriteAfterFetch(); a != nil {
		results = append(results, *a)
	}
	if a := d.checkToolBurst(); a != nil {
		results = append(results, *a)
	}
	results = append(results, d.checkUnusualSequence()...)
	if a := d.checkPrivilegeEscalation(); a != nil {
		results = append(results, *a)
	}
	if a := d.checkDataExfiltration(); a != nil {
		results = append(results, *a)
	}
	if a := d.checkPromptExtraction(); a != nil {
		results = append(results, *a)
	}

	return results
}

// WindowSize returns the current window size (for testing).
func (d *AnomalyDetector) WindowSize() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.window)
}

// EMABaseline returns a copy of the current EMA baseline (for testing).
func (d *AnomalyDetector) EMABaseline() map[string]float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]float64, len(d.emaBaseline))
	for k, v := range d.emaBaseline {
		out[k] = v
	}
	return out
}

// ── Detection algorithms ───────────────────────────────────────────────────

// 1. writeAfterFetch: fetch/web_search then write/execute within next 3 calls.
func (d *AnomalyDetector) checkWriteAfterFetch() *Anomaly {
	for i, call := range d.window {
		if !fetchTools[call.ToolName] {
			continue
		}
		for offset := 1; offset <= 3; offset++ {
			checkIdx := i + offset
			if checkIdx >= len(d.window) {
				break
			}
			next := d.window[checkIdx]
			if writeTools[next.ToolName] {
				return &Anomaly{
					Pattern:  "writeAfterFetch",
					Severity: SevMedium,
					Details: map[string]any{
						"fetchCall":       call.ToolName,
						"fetchIndex":      i,
						"subsequentCall":  next.ToolName,
						"subsequentIndex": checkIdx,
						"offset":          offset,
					},
				}
			}
		}
	}
	return nil
}

// 2. toolBurst: >15 calls in one turn.
func (d *AnomalyDetector) checkToolBurst() *Anomaly {
	turnCounts := make(map[int]int)
	for _, call := range d.window {
		turnCounts[call.TurnNumber]++
	}
	for turn, count := range turnCounts {
		if count > burstThreshold {
			return &Anomaly{
				Pattern:  "toolBurst",
				Severity: SevHigh,
				Details:  map[string]any{"count": count, "turn": turn},
			}
		}
	}
	return nil
}

// 3. unusualSequence: tool frequency deviates >2σ from EMA baseline.
func (d *AnomalyDetector) checkUnusualSequence() []Anomaly {
	if len(d.window) == 0 {
		return nil
	}
	d.updateEMABaseline()

	values := make([]float64, 0, len(d.emaBaseline))
	for _, v := range d.emaBaseline {
		values = append(values, v)
	}
	if len(values) == 0 {
		return nil
	}

	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))

	variance := 0.0
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(values))
	stddev := math.Sqrt(variance)
	if stddev < 1e-9 {
		stddev = 1
	}

	// Count current window frequencies
	windowCounts := make(map[string]int)
	for _, call := range d.window {
		windowCounts[call.ToolName]++
	}

	var anomalies []Anomaly
	for tool, count := range windowCounts {
		freq := d.emaBaseline[tool]
		isRare := freq < 0.11
		isBelowThreshold := mean > 0.05 && freq < mean-1.5*stddev

		if isRare || isBelowThreshold {
			sev := SevLow
			if freq < 0.05 {
				sev = SevMedium
			}
			anomalies = append(anomalies, Anomaly{
				Pattern:  "unusualSequence",
				Severity: sev,
				Details: map[string]any{
					"tool":         tool,
					"emaFrequency": freq,
					"mean":         mean,
					"stddev":       stddev,
					"windowCount":  count,
					"isRare":       isRare,
				},
			})
		}
	}
	return anomalies
}

// 4. privilegeEscalation: read sensitive path then immediately write/execute.
func (d *AnomalyDetector) checkPrivilegeEscalation() *Anomaly {
	if len(d.window) < 2 {
		return nil
	}
	for i := 0; i < len(d.window)-1; i++ {
		call := d.window[i]
		if call.ToolName != "read_file" {
			continue
		}
		filePath, _ := call.Params["file_path"].(string)
		if filePath == "" {
			filePath, _ = call.Params["path"].(string)
		}
		if filePath == "" {
			continue
		}
		sensitive := false
		for _, p := range sensitivePathPatterns {
			if p.MatchString(filePath) {
				sensitive = true
				break
			}
		}
		if !sensitive {
			continue
		}
		next := d.window[i+1]
		if next.ToolName == "run_command" || next.ToolName == "write_file" || next.ToolName == "bash" {
			return &Anomaly{
				Pattern:  "privilegeEscalation",
				Severity: SevHigh,
				Details: map[string]any{
					"sensitivePath":   filePath,
					"readTool":        call.ToolName,
					"subsequentTool":  next.ToolName,
					"readIndex":       i,
					"subsequentIndex": i + 1,
				},
			}
		}
	}
	return nil
}

// 5. dataExfiltration: search query or command contains file path patterns.
func (d *AnomalyDetector) checkDataExfiltration() *Anomaly {
	pathPatterns := []struct {
		re    *regexp.Regexp
		label string
	}{
		{regexp.MustCompile(`(?i)/etc/(?:shadow|passwd|hosts|sudoers)`), "system_file"},
		{regexp.MustCompile(`/\.ssh/`), "ssh_directory"},
		{regexp.MustCompile(`/\.aws/`), "aws_directory"},
		{regexp.MustCompile(`~\/\.`), "hidden_home_file"},
		{regexp.MustCompile(`/home/\w+/`), "home_directory"},
		{regexp.MustCompile(`\.(?:env|key|pem|crt)$`), "sensitive_extension"},
		{regexp.MustCompile(`(?i)(?:secret|credential|password|token)`), "sensitive_keyword"},
	}

	for _, call := range d.window {
		if call.ToolName != "web_search" && call.ToolName != "run_command" && call.ToolName != "bash" {
			continue
		}
		query, _ := call.Params["query"].(string)
		command, _ := call.Params["command"].(string)
		content := query
		if content == "" {
			content = command
		}
		if content == "" {
			continue
		}
		for _, pp := range pathPatterns {
			if pp.re.MatchString(content) {
				preview := content
				if len(preview) > 100 {
					preview = preview[:100]
				}
				return &Anomaly{
					Pattern:  "dataExfiltration",
					Severity: SevHigh,
					Details: map[string]any{
						"tool":           call.ToolName,
						"matchedPattern": pp.label,
						"content":        preview,
					},
				}
			}
		}
	}
	return nil
}

// 6. promptExtraction: tool parameters contain system prompt fragments.
func (d *AnomalyDetector) checkPromptExtraction() *Anomaly {
	for _, call := range d.window {
		for _, v := range call.Params {
			s, ok := v.(string)
			if !ok || len(s) < 50 {
				continue
			}
			lower := strings.ToLower(s)
			for _, fragment := range commonSysPromptFragments {
				if strings.Contains(lower, strings.ToLower(fragment)) {
					preview := s
					if len(preview) > 200 {
						preview = preview[:200]
					}
					return &Anomaly{
						Pattern:  "promptExtraction",
						Severity: SevHigh,
						Details: map[string]any{
							"tool":            call.ToolName,
							"matchedFragment": fragment,
							"paramValue":      preview,
						},
					}
				}
			}
		}
	}
	return nil
}

// ── EMA baseline ───────────────────────────────────────────────────────────

func (d *AnomalyDetector) updateEMABaseline() {
	counts := make(map[string]int)
	for _, call := range d.window {
		counts[call.ToolName]++
	}
	total := float64(len(d.window))
	frequencies := make(map[string]float64)
	for tool, count := range counts {
		frequencies[tool] = float64(count) / total
	}

	for tool, freq := range frequencies {
		if old, ok := d.emaBaseline[tool]; ok {
			d.emaBaseline[tool] = emaAlpha*freq + (1-emaAlpha)*old
		} else {
			d.emaBaseline[tool] = freq
		}
	}

	// Decay tools not in window
	for tool, val := range d.emaBaseline {
		if _, ok := frequencies[tool]; !ok {
			d.emaBaseline[tool] = (1 - emaAlpha) * val
			if d.emaBaseline[tool] < 0.001 {
				delete(d.emaBaseline, tool)
			}
		}
	}
}
