# GoProxy Plugin

A Go-based proxy plugin for drpy-node that provides m3u8/mp4/ts/http resource proxying with optional header injection, host allow list, HMAC signature validation, m3u8 segment rewriting, simple in-memory caching, and CORS support.

## Features

- **Resource Proxying**: Supports proxying m3u8, mp4, ts, and other HTTP resources
- **Header Injection**: Optional User-Agent, Referer, and Cookie headers
- **Host Allow List**: Restrict access to specific hosts
- **HMAC Signature Validation**: Optional signature verification for secure access
- **M3U8 Segment Rewriting**: Automatically rewrite m3u8 playlists to proxy segments
- **In-Memory Caching**: Simple caching with configurable TTL
- **CORS Support**: Cross-origin resource sharing support
- **Configurable Limits**: Maximum response size limits

## Command Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `-listen` | Listen address and port | `:8080` |
| `-path` | Proxy endpoint path | `/proxy` |
| `-allow` | Allowed host (can be repeated) | None (all hosts allowed) |
| `-ua` | User-Agent header | `goproxy/1.0` |
| `-referer` | Referer header | None |
| `-cookie` | Cookie header | None |
| `-rule-host-ua` | Host-based UA rule (format: host\|ua) | None |
| `-rule-regex-ua` | Regex-based UA rule (format: regex\|ua) | None |
| `-rewrite` | Enable m3u8 segment rewriting | `false` |
| `-sign-required` | Require HMAC signature validation | `false` |
| `-sign-secret` | HMAC signature secret | None |
| `-cache-ttl` | Cache TTL in seconds (0 = no cache) | `0` |
| `-cors-origins` | CORS allowed origins (comma-separated) | None |
| `-log` | Enable request logging | `false` |
| `-max-mb` | Maximum response size in MB | `100` |

## Usage

### Basic Usage

Start the proxy server:
```bash
./goproxy-linux -listen :57573 -path /proxy -log
```

Proxy a resource:
```
GET http://localhost:57573/proxy?url=https://example.com/video.m3u8
```

### With Security Features

Start with signature validation and host restrictions:
```bash
./goproxy-linux -listen :57573 -allow example.com -allow cdn.example.com -sign-required -sign-secret "mysecret" -log
```

Access with signature:
```
GET http://localhost:57573/proxy?url=https://example.com/video.m3u8&sign=<hmac_signature>
```

### With M3U8 Rewriting

Enable automatic m3u8 segment rewriting:
```bash
./goproxy-linux -listen :57573 -rewrite -log
```

This will automatically rewrite relative URLs in m3u8 playlists to use the proxy.

### With Caching

Enable caching with 5-minute TTL:
```bash
./goproxy-linux -listen :57573 -cache-ttl 300 -log
```

### Custom Headers

Set custom headers based on rules:
```bash
# Set UA for specific host
./goproxy-linux -rule-host-ua "cdn.example.com|Mozilla/5.0 Custom UA"

# Set UA based on regex pattern
./goproxy-linux -rule-regex-ua ".*\.m3u8.*|VideoPlayer/1.0"

# Set global headers
./goproxy-linux -ua "Custom User Agent" -referer "https://example.com" -cookie "session=abc123"
```

## Plugin Configuration

Add to your `.plugins.js` file:

```javascript
{
    name: 'goproxy',
    path: 'plugins/goproxy',
    params: '-listen :57573 -path /proxy -log -cache-ttl 300 -max-mb 100 -cors-origins *',
    desc: 'Go代理服务',
    active: true
}
```

## Endpoints

- `GET /proxy?url=<target_url>[&sign=<signature>]` - Proxy a resource
- `GET /health` - Health check endpoint

## Response Headers

The proxy preserves most response headers from the target server, with the following exceptions:
- `Content-Length` is recalculated
- `Transfer-Encoding` and `Connection` headers are stripped
- CORS headers are added if configured

## Security

- Host allow list prevents access to unauthorized domains
- HMAC signature validation ensures requests are authenticated
- Maximum response size limits prevent resource exhaustion
- Request logging for audit trails

## Building

To build the binary for different platforms:

```bash
# Linux
go build -o goproxy-linux main.go

# Windows
GOOS=windows GOARCH=amd64 go build -o goproxy-windows.exe main.go

# macOS
GOOS=darwin GOARCH=amd64 go build -o goproxy-darwin main.go

# Android
GOOS=android GOARCH=arm64 go build -o goproxy-android main.go
```

## License

This plugin is part of the drpy-node project and follows the same license terms.