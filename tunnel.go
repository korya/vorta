package vrata

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// TunnelOptions holds configuration for creating a tunnel
type TunnelOptions struct {
	Port       int
	Host       string
	Subdomain  string
	LocalHost  string
	LocalHTTPS bool
}

// TunnelInfo represents the server response for tunnel creation
type TunnelInfo struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	Port    int    `json:"port"`
	MaxConn int    `json:"max_conn_count"`
}

// RequestInfo contains information about proxied requests
type RequestInfo struct {
	Method string
	Path   string
	URL    string
}

// TunnelEvents provides channels for tunnel events
type TunnelEvents struct {
	URL     chan string
	Error   chan error
	Request chan RequestInfo
	Close   chan struct{}
}

// Tunnel represents a localtunnel connection
type Tunnel struct {
	options *TunnelOptions
	info    *TunnelInfo
	events  *TunnelEvents
	cluster *TunnelCluster
	ctx     context.Context
	cancel  context.CancelFunc
	closed  bool
	mutex   sync.RWMutex
}

// NewTunnel creates a new tunnel instance
func NewTunnel(port int, options *TunnelOptions) (*Tunnel, error) {
	if options == nil {
		options = &TunnelOptions{}
	}
	options.Port = port

	if options.Host == "" {
		options.Host = "https://localtunnel.me"
	}
	if options.LocalHost == "" {
		options.LocalHost = "localhost"
	}

	ctx, cancel := context.WithCancel(context.Background())

	events := &TunnelEvents{
		URL:     make(chan string, 1),
		Error:   make(chan error, 10),
		Request: make(chan RequestInfo, 100),
		Close:   make(chan struct{}, 1),
	}

	return &Tunnel{
		options: options,
		events:  events,
		ctx:     ctx,
		cancel:  cancel,
	}, nil
}

// Open establishes the tunnel connection
func (t *Tunnel) Open() error {
	// Register with the localtunnel server
	info, err := t.requestTunnel()
	if err != nil {
		return fmt.Errorf("failed to request tunnel: %w", err)
	}

	t.info = info

	// Create the tunnel cluster for connection management
	cluster, err := NewTunnelCluster(t.info, t.options, t.events)
	if err != nil {
		return fmt.Errorf("failed to create tunnel cluster: %w", err)
	}

	t.cluster = cluster

	// Start the cluster
	go func() {
		if err := t.cluster.Start(t.ctx); err != nil {
			select {
			case t.events.Error <- err:
			case <-t.ctx.Done():
			}
		}
	}()

	// Send the URL event
	select {
	case t.events.URL <- t.info.URL:
	case <-t.ctx.Done():
		return t.ctx.Err()
	}

	return nil
}

// Close shuts down the tunnel
func (t *Tunnel) Close() error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.closed {
		return nil
	}

	t.closed = true
	t.cancel()

	if t.cluster != nil {
		t.cluster.Close()
	}

	select {
	case t.events.Close <- struct{}{}:
	default:
	}

	return nil
}

// URL returns the tunnel URL (blocking until available)
func (t *Tunnel) URL() (string, error) {
	select {
	case url := <-t.events.URL:
		return url, nil
	case err := <-t.events.Error:
		return "", err
	case <-t.ctx.Done():
		return "", t.ctx.Err()
	}
}

// Events returns the events channels
func (t *Tunnel) Events() *TunnelEvents {
	return t.events
}

// requestTunnel makes an HTTP request to get tunnel info from the server
func (t *Tunnel) requestTunnel() (*TunnelInfo, error) {
	reqURL := t.options.Host
	if t.options.Subdomain != "" {
		reqURL += "/" + t.options.Subdomain
	}

	params := url.Values{}
	params.Set("new", "")

	if reqURL+"?"+params.Encode() != reqURL+"?new=" {
		reqURL += "?" + params.Encode()
	} else {
		reqURL += "?new="
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server responded with status %d", resp.StatusCode)
	}

	var info TunnelInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &info, nil
}

// OpenURL opens a URL in the default browser
func OpenURL(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // linux, freebsd, openbsd, netbsd
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// HeaderHostTransformer modifies HTTP headers to use localhost
type HeaderHostTransformer struct {
	host string
}

// NewHeaderHostTransformer creates a new header transformer
func NewHeaderHostTransformer(host string) *HeaderHostTransformer {
	return &HeaderHostTransformer{host: host}
}

// Transform modifies the request headers
func (h *HeaderHostTransformer) Transform(reader io.Reader, writer io.Writer) error {
	scanner := bufio.NewScanner(reader)

	// Read and transform the first line (HTTP request line)
	if !scanner.Scan() {
		return scanner.Err()
	}

	firstLine := scanner.Text()
	fmt.Fprintf(writer, "%s\r\n", firstLine)

	// Read and transform headers
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			fmt.Fprintf(writer, "\r\n")
			break
		}

		if strings.HasPrefix(strings.ToLower(line), "host:") {
			fmt.Fprintf(writer, "Host: %s\r\n", h.host)
		} else {
			fmt.Fprintf(writer, "%s\r\n", line)
		}
	}

	// Copy the rest of the body
	_, err := io.Copy(writer, reader)
	return err
}
