# mcp-web-search

Standalone MCP (Model Context Protocol) server for web search.

**Zero dependencies.** Built with only Go standard library.

## Backends

- **Exa** — neural search with content extraction (`EXA_API_KEY`)
- **Tavily** — AI-optimized search with answer synthesis (`TAVILY_API_KEY`)

## Health gate

If either engine returns an error, it's paused for a 12-hour cooldown.
A random engine is selected per request to distribute load and avoid
rate limits. If the randomly selected engine fails, the other is tried
as fallback.

## Build

```bash
go build -o mcp-web-search .
```

## Run

```bash
export EXA_API_KEY=your-key
export TAVILY_API_KEY=your-key
./mcp-web-search
```

The server reads JSON-RPC requests from stdin and writes responses to stdout.
It implements the MCP 2024-11-05 protocol.

## API keys

Keys can also be read from `~/.reasonix/api-keys.json`:

```json
{
  "exa": "your-exa-key",
  "tavily": "your-tavily-key"
}
```

## Configuring in Reasonix

Add to `reasonix.toml`:

```toml
[[plugins]]
name = "web-search"
command = "/path/to/mcp-web-search"
```

## Testing

```bash
go test ./...
```
