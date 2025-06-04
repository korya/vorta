# vrata - stable and easy HTTP tunneling

A faithful Go port of the [localtunnel](https://github.com/localtunnel/localtunnel) Node.js project. Expose your localhost to the world for easy testing and sharing!

## Features

- 🌍 **Instant Public URLs** - Get a public URL for your local development server
- 🔧 **CLI Interface** - Simple command-line tool just like the original
- 📚 **Go API** - Programmatic access with channels and contexts  
- 🔄 **Auto-Reconnection** - Automatically reconnects if connections drop
- 🛡️ **HTTPS Support** - Tunnel HTTPS traffic from your local server
- 🎯 **Custom Subdomains** - Request specific subdomains when available

## Installation

```bash
go install github.com/korya/vrata/cmd/vrata@latest
```

Or build from source:

```bash
git clone https://github.com/korya/vrata
cd vrata
go build -o vrata ./cmd/vrata
```

## CLI Usage

Basic usage:
```bash
# Expose local server on port 8080
vrata --port 8080

# Request a specific subdomain
vrata --port 3000 --subdomain myapp

# Open tunnel URL in browser automatically
vrata --port 8080 --open

# Use custom upstream server
vrata --port 8080 --host https://my-tunnel-server.com

# Tunnel HTTPS traffic
vrata --port 8443 --local-https

# Print request logs
vrata --port 8080 --print-requests
```

Command-line options:
```
  -p, --port           Internal HTTP server port (required)
  -h, --host           Upstream server (default: https://localtunnel.me)
  -s, --subdomain      Request specific subdomain
  -l, --local-host     Tunnel traffic to alternative localhost (default: localhost)
      --local-https    Enable HTTPS tunneling
  -o, --open           Automatically open tunnel URL in browser
      --print-requests Log request information
      --version        Show version
      --help           Show help
```

## Go API Usage

### Basic Example

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/korya/vrata"
)

func main() {
    // Create tunnel options
    options := &vrata.TunnelOptions{
        Port:      8080,
        Subdomain: "myapp", // optional
    }
    
    // Create and open tunnel
    tunnel, err := vrata.ConnectAndOpen(8080, options)
    if err != nil {
        log.Fatal(err)
    }
    defer tunnel.Close()
    
    // Get the public URL
    url, err := tunnel.URL()
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Tunnel URL: %s\n", url)
    
    // Listen for events
    events := tunnel.Events()
    for {
        select {
        case req := <-events.Request:
            fmt.Printf("Request: %s %s\n", req.Method, req.Path)
        case err := <-events.Error:
            fmt.Printf("Error: %v\n", err)
        case <-events.Close:
            fmt.Println("Tunnel closed")
            return
        }
    }
}
```

### Advanced Usage with Context

```go
package main

import (
    "context"
    "time"
    
    "github.com/korya/vrata"
)

func main() {
    // Create context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()
    
    // Create tunnel with context
    tunnel, err := vrata.ConnectWithContext(ctx, 8080, nil)
    if err != nil {
        panic(err)
    }
    defer tunnel.Close()
    
    // Open tunnel
    if err := tunnel.Open(); err != nil {
        panic(err)
    }
    
    // Wait for URL or timeout
    url, err := tunnel.URL()
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Tunnel: %s\n", url)
    
    // Tunnel will automatically close when context times out
    <-ctx.Done()
}
```

## API Reference

### Types

#### `TunnelOptions`
```go
type TunnelOptions struct {
    Port       int    // Local server port
    Host       string // Tunnel server URL (default: "https://localtunnel.me")
    Subdomain  string // Requested subdomain (optional)
    LocalHost  string // Local hostname (default: "localhost")
    LocalHTTPS bool   // Enable HTTPS for local connections
}
```

#### `TunnelEvents`
```go
type TunnelEvents struct {
    URL     chan string      // Tunnel URL ready
    Error   chan error       // Connection errors
    Request chan RequestInfo // Incoming requests
    Close   chan struct{}    // Tunnel closed
}
```

### Functions

#### `Connect(port int, options *TunnelOptions) (*Tunnel, error)`
Creates a new tunnel instance.

#### `ConnectAndOpen(port int, options *TunnelOptions) (*Tunnel, error)`
Creates and opens a tunnel in one call.

#### `ConnectWithContext(ctx context.Context, port int, options *TunnelOptions) (*Tunnel, error)`
Creates a tunnel with custom context for cancellation.

### Methods

#### `tunnel.Open() error`
Opens the tunnel connection.

#### `tunnel.Close() error`
Closes the tunnel and cleans up resources.

#### `tunnel.URL() (string, error)`
Returns the public tunnel URL (blocks until available).

#### `tunnel.Events() *TunnelEvents`
Returns the events channels for monitoring.

## Comparison with Node.js Version

This Go implementation provides the same functionality as the original Node.js localtunnel:

| Feature | Node.js | Go (vrata) |
|---------|---------|------------|
| CLI Interface | ✅ | ✅ |
| Programmatic API | ✅ | ✅ |
| Custom Subdomains | ✅ | ✅ |
| HTTPS Tunneling | ✅ | ✅ |
| Auto-Reconnection | ✅ | ✅ |
| Request Logging | ✅ | ✅ |
| Browser Auto-Open | ✅ | ✅ |
| Event System | EventEmitter | Channels |
| Async Pattern | Promises/Callbacks | Channels/Context |

## Examples

See the `example/` directory for complete working examples.

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Contributing

Contributions welcome! Please read the contributing guidelines and submit pull requests.
