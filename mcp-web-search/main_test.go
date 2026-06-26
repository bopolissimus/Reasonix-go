package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSelectEngine(t *testing.T) {
	engines := []string{"exa", "tavily"}
	seen := make(map[string]int)
	for i := 0; i < 100; i++ {
		e := selectEngine(engines)
		seen[e]++
	}
	if seen["exa"] == 0 || seen["tavily"] == 0 {
		t.Errorf("random engine selection not distributing: exa=%d tavily=%d", seen["exa"], seen["tavily"])
	}
}

func TestSelectEngineEmpty(t *testing.T) {
	if got := selectEngine(nil); got != "" {
		t.Errorf("selectEngine(nil) = %q, want empty", got)
	}
	if got := selectEngine([]string{}); got != "" {
		t.Errorf("selectEngine([]) = %q, want empty", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 3); got != "hel..." {
		t.Errorf("truncate(hello, 3) = %q, want hel...", got)
	}
	if got := truncate("hi", 10); got != "hi" {
		t.Errorf("truncate(hi, 10) = %q, want hi", got)
	}
}

func TestFormatSearchResults(t *testing.T) {
	results := []searchResult{
		{Title: "Test", URL: "https://example.com", Snippet: "A snippet"},
	}
	out := formatSearchResults(results, "exa")
	if out == "" {
		t.Error("formatSearchResults returned empty")
	}
}

func TestExaSearchMockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[{"title":"Go","url":"https://go.dev","text":"Go programming language"}]}`))
	}))
	defer srv.Close()

	// Test with manual HTTP call (exaSearch uses api.exa.ai, not mock)
	// Just verify the server pattern works
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("expected 401 without auth, got %d", resp.StatusCode)
	}
}

func TestHealthGatePause(t *testing.T) {
	g := newHealthGate(100 * time.Millisecond)
	if g.isPaused("exa") {
		t.Error("engine should not be paused initially")
	}
	g.pause("exa")
	if !g.isPaused("exa") {
		t.Error("engine should be paused after pause()")
	}
	time.Sleep(150 * time.Millisecond)
	if g.isPaused("exa") {
		t.Error("engine should be unpaused after cooldown")
	}
}

func TestHealthGateActiveEngines(t *testing.T) {
	g := newHealthGate(100 * time.Millisecond)
	g.pause("exa")
	active := g.activeEngines([]string{"exa", "tavily"})
	if len(active) != 1 || active[0] != "tavily" {
		t.Errorf("expected only tavily active, got %v", active)
	}
}

func TestHealthGateAllPaused(t *testing.T) {
	g := newHealthGate(1 * time.Hour)
	g.pause("exa")
	g.pause("tavily")
	active := g.activeEngines([]string{"exa", "tavily"})
	if len(active) != 0 {
		t.Errorf("expected no active engines, got %v", active)
	}
}

func TestMCPInitialize(t *testing.T) {
	s := &server{
		cfg:    config{engines: []string{"exa"}},
		gate:   newHealthGate(1 * time.Hour),
		reader: nil,
	}
	req := jsonrpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize"}
	var captured jsonrpcResponse
	s.writer = &testWriter{fn: func(data []byte) {
		json.Unmarshal(data, &captured)
	}}
	s.handle(req)
	if captured.ID != float64(1) { // JSON unmarshals numbers as float64
		t.Errorf("initialize: unexpected id %v", captured.ID)
	}
	var init initializeResult
	data, _ := json.Marshal(captured.Result)
	json.Unmarshal(data, &init)
	if init.ServerInfo.Name != "web-search" {
		t.Errorf("server name = %q, want web-search", init.ServerInfo.Name)
	}
}

func TestMCPListTools(t *testing.T) {
	s := &server{
		cfg:    config{engines: []string{"exa"}},
		gate:   newHealthGate(1 * time.Hour),
		reader: nil,
	}
	req := jsonrpcRequest{JSONRPC: "2.0", ID: 2, Method: "tools/list"}
	var captured jsonrpcResponse
	s.writer = &testWriter{fn: func(data []byte) {
		json.Unmarshal(data, &captured)
	}}
	s.handle(req)
	var tools listToolsResult
	data, _ := json.Marshal(captured.Result)
	json.Unmarshal(data, &tools)
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "web_search" {
		t.Errorf("unexpected tools: %+v", tools.Tools)
	}
}

func TestMCPCallToolMissingQuery(t *testing.T) {
	s := &server{
		cfg:    config{engines: []string{"exa"}, ExaAPIKey: "test-key"},
		gate:   newHealthGate(1 * time.Hour),
		reader: nil,
	}
	params, _ := json.Marshal(callToolParams{Name: "web_search", Arguments: map[string]any{}})
	req := jsonrpcRequest{JSONRPC: "2.0", ID: 3, Method: "tools/call", Params: params}
	var captured jsonrpcResponse
	s.writer = &testWriter{fn: func(data []byte) {
		json.Unmarshal(data, &captured)
	}}
	s.handle(req)
	var result callToolResult
	data, _ := json.Marshal(captured.Result)
	json.Unmarshal(data, &result)
	if !result.IsError {
		t.Error("expected error for missing query")
	}
}

func TestMCPUnknownMethod(t *testing.T) {
	s := &server{
		cfg:    config{engines: []string{"exa"}},
		gate:   newHealthGate(1 * time.Hour),
		reader: nil,
	}
	req := jsonrpcRequest{JSONRPC: "2.0", ID: 4, Method: "nonexistent"}
	var captured jsonrpcResponse
	s.writer = &testWriter{fn: func(data []byte) {
		json.Unmarshal(data, &captured)
	}}
	s.handle(req)
	if captured.Error == nil {
		t.Error("expected error for unknown method")
	}
}

// testWriter captures JSON-RPC responses for test assertions.
type testWriter struct {
	fn func([]byte)
}

func (w *testWriter) Write(p []byte) (int, error) {
	w.fn(p)
	return len(p), nil
}
