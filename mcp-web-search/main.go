// Package main implements a web_search MCP server backed by Exa and Tavily
// with random engine selection and health-gate cooldown on failures.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ── MCP protocol types ────────────────────────────────────────────────────

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type initializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	ServerInfo      serverInfo   `json:"serverInfo"`
	Capabilities    capabilities `json:"capabilities"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type capabilities struct {
	Tools *toolsCapability `json:"tools,omitempty"`
}

type toolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type listToolsResult struct {
	Tools []toolDef `json:"tools"`
}

type callToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type callToolResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ── Search types ──────────────────────────────────────────────────────────

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// ── Config ─────────────────────────────────────────────────────────────────

type config struct {
	ExaAPIKey     string
	TavilyAPIKey  string
	KeysFile      string
	ExaKeyName    string
	TavilyKeyName string
	Cooldown      time.Duration
	engines       []string
}

func loadConfig() config {
	keysFile := flag.String("keys-file", "", "Path to JSON file with API keys (default: ~/.reasonix/api-keys.json)")
	exaKeyName := flag.String("exa-key-name", "exa", "Key name for Exa in the keys file")
	tavilyKeyName := flag.String("tavily-key-name", "tavily", "Key name for Tavily in the keys file")
	flag.Parse()

	cfg := config{
		KeysFile:      *keysFile,
		ExaKeyName:    *exaKeyName,
		TavilyKeyName: *tavilyKeyName,
		Cooldown:      12 * time.Hour,
		engines:       []string{"exa", "tavily"},
	}

	// 1. Environment variables (highest priority)
	cfg.ExaAPIKey = os.Getenv("EXA_API_KEY")
	cfg.TavilyAPIKey = os.Getenv("TAVILY_API_KEY")

	// 2. Keys file (fallback per-engine)
	if cfg.ExaAPIKey == "" || cfg.TavilyAPIKey == "" {
		if keys, err := loadAPIKeys(cfg.KeysFile); err == nil {
			if cfg.ExaAPIKey == "" {
				cfg.ExaAPIKey = keys[cfg.ExaKeyName]
			}
			if cfg.TavilyAPIKey == "" {
				cfg.TavilyAPIKey = keys[cfg.TavilyKeyName]
			}
		}
	}
	return cfg
}

func loadAPIKeys(keysFile string) (map[string]string, error) {
	if keysFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		keysFile = home + "/.reasonix/api-keys.json"
	}
	data, err := os.ReadFile(keysFile)
	if err != nil {
		return nil, err
	}
	var keys map[string]string
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// ── Health gate ────────────────────────────────────────────────────────────

type healthGate struct {
	mu       sync.Mutex
	paused   map[string]time.Time // engine → cooldown-until
	cooldown time.Duration
}

func newHealthGate(cooldown time.Duration) *healthGate {
	return &healthGate{
		paused:   make(map[string]time.Time),
		cooldown: cooldown,
	}
}

func (g *healthGate) isPaused(engine string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	until, ok := g.paused[engine]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(g.paused, engine)
		return false
	}
	return true
}

func (g *healthGate) pause(engine string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.paused[engine] = time.Now().Add(g.cooldown)
}

func (g *healthGate) activeEngines(engines []string) []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	var active []string
	for _, e := range engines {
		until, ok := g.paused[e]
		if ok && now.Before(until) {
			continue
		}
		if ok {
			delete(g.paused, e)
		}
		active = append(active, e)
	}
	return active
}

// ── HTTP client ────────────────────────────────────────────────────────────

var httpClient = &http.Client{Timeout: 15 * time.Second}

// ── Exa search ─────────────────────────────────────────────────────────────

func exaSearch(query, apiKey string) ([]searchResult, error) {
	url := "https://api.exa.ai/search"
	body := fmt.Sprintf(`{"query":%q,"numResults":5,"contents":{"text":true}}`, query)
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("exa: status %d", resp.StatusCode)
	}
	var out struct {
		Results []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
			Text  string `json:"text"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	results := make([]searchResult, len(out.Results))
	for i, r := range out.Results {
		results[i] = searchResult{Title: r.Title, URL: r.URL, Snippet: truncate(r.Text, 200)}
	}
	return results, nil
}

// ── Tavily search ──────────────────────────────────────────────────────────

func tavilySearch(query, apiKey string) ([]searchResult, string, error) {
	url := "https://api.tavily.com/search"
	body := fmt.Sprintf(`{"api_key":%q,"query":%q,"max_results":5}`, apiKey, query)
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("tavily: status %d", resp.StatusCode)
	}
	var out struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
		Answer string `json:"answer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, "", err
	}
	results := make([]searchResult, len(out.Results))
	for i, r := range out.Results {
		results[i] = searchResult{Title: r.Title, URL: r.URL, Snippet: truncate(r.Content, 200)}
	}
	return results, out.Answer, nil
}

// ── Engine selection ───────────────────────────────────────────────────────

func selectEngine(engines []string) string {
	if len(engines) == 0 {
		return ""
	}
	return engines[rand.Intn(len(engines))]
}

// ── Helpers ────────────────────────────────────────────────────────────────

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func formatSearchResults(results []searchResult, engine string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Web Search Results (%s)\n\n", engine)
	for i, r := range results {
		fmt.Fprintf(&b, "%d. **%s**\n", i+1, r.Title)
		fmt.Fprintf(&b, "   %s\n", r.URL)
		if r.Snippet != "" {
			fmt.Fprintf(&b, "   %s\n", r.Snippet)
		}
		fmt.Fprintln(&b)
	}
	return strings.TrimSpace(b.String())
}

// ── MCP server ─────────────────────────────────────────────────────────────

type server struct {
	cfg    config
	gate   *healthGate
	reader *bufio.Reader
	writer io.Writer
}

func main() {
	cfg := loadConfig()
	s := &server{
		cfg:    cfg,
		gate:   newHealthGate(cfg.Cooldown),
		reader: bufio.NewReader(os.Stdin),
		writer: os.Stdout,
	}
	s.run()
}

func (s *server) run() {
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return
			}
			fmt.Fprintf(os.Stderr, "read error: %v\n", err)
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var req jsonrpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.sendError(nil, -32700, "Parse error")
			continue
		}
		s.handle(req)
	}
}

func (s *server) handle(req jsonrpcRequest) {
	switch req.Method {
	case "initialize":
		s.send(req.ID, initializeResult{
			ProtocolVersion: "2024-11-05",
			ServerInfo:      serverInfo{Name: "web-search", Version: "1.0.0"},
			Capabilities:    capabilities{Tools: &toolsCapability{}},
		})

	case "tools/list":
		s.send(req.ID, listToolsResult{Tools: []toolDef{{
			Name:        "web_search",
			Description: "Search the web using Exa or Tavily. Returns title, URL, and snippet for each result.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search query",
					},
				},
				"required": []string{"query"},
			},
		}}})

	case "tools/call":
		var params callToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params")
			return
		}
		if params.Name != "web_search" {
			s.sendError(req.ID, -32601, "Unknown tool: "+params.Name)
			return
		}
		query, _ := params.Arguments["query"].(string)
		if query == "" {
			s.send(req.ID, callToolResult{Content: []textContent{{Type: "text", Text: "Error: query is required"}}, IsError: true})
			return
		}
		result, isError := s.doSearch(query)
		s.send(req.ID, callToolResult{Content: []textContent{{Type: "text", Text: result}}, IsError: isError})

	case "notifications/initialized":
		// no-op

	default:
		s.sendError(req.ID, -32601, "Unknown method: "+req.Method)
	}
}

func (s *server) doSearch(query string) (string, bool) {
	// Filter to active engines
	active := s.gate.activeEngines(s.cfg.engines)
	if len(active) == 0 {
		return "Error: all search engines are paused due to rate limits. Try again later.", true
	}

	// Try each engine with fallback
	for _, engine := range []string{selectEngine(active)} {
		result, isError := s.doSearchWithEngine(query, engine)
		if !isError {
			return result, false
		}
		// Try remaining engines as fallback
	}
	// If all engines failed in the random selection, try each remaining
	for _, engine := range active {
		result, isError := s.doSearchWithEngine(query, engine)
		if !isError {
			return result, false
		}
	}

	s.gate.pause("exa")
	s.gate.pause("tavily")
	return "Error: all search engines failed. Both have been paused for cooldown.", true
}

func (s *server) doSearchWithEngine(query, engine string) (string, bool) {
	switch engine {
	case "exa":
		if s.cfg.ExaAPIKey == "" {
			return fmt.Sprintf("Error: Exa API key not configured (set EXA_API_KEY env or --keys-file with --exa-key-name)"), true
		}
		results, err := exaSearch(query, s.cfg.ExaAPIKey)
		if err != nil {
			s.gate.pause("exa")
			return fmt.Sprintf("Error: exa search failed: %v", err), true
		}
		return formatSearchResults(results, "exa"), false

	case "tavily":
		if s.cfg.TavilyAPIKey == "" {
			return fmt.Sprintf("Error: Tavily API key not configured (set TAVILY_API_KEY env or --keys-file with --tavily-key-name)"), true
		}
		results, answer, err := tavilySearch(query, s.cfg.TavilyAPIKey)
		if err != nil {
			s.gate.pause("tavily")
			return fmt.Sprintf("Error: tavily search failed: %v", err), true
		}
		out := formatSearchResults(results, "tavily")
		if answer != "" {
			out += "\n\n**Answer:** " + answer
		}
		return out, false

	default:
		return fmt.Sprintf("Error: unknown engine %q", engine), true
	}
}

func (s *server) send(id any, result any) {
	resp := jsonrpcResponse{JSONRPC: "2.0", ID: id, Result: result}
	s.writeResponse(resp)
}

func (s *server) sendError(id any, code int, message string) {
	resp := jsonrpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
	s.writeResponse(resp)
}

func (s *server) writeResponse(resp jsonrpcResponse) {
	data, _ := json.Marshal(resp)
	fmt.Fprintln(s.writer, string(data))
}
