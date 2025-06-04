package vrata

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TunnelCluster manages multiple connections to the localtunnel server
type TunnelCluster struct {
	info        *TunnelInfo
	options     *TunnelOptions
	events      *TunnelEvents
	connections []*TunnelConnection
	mutex       sync.RWMutex
	closed      bool
}

// TunnelConnection represents a single connection to the tunnel server
type TunnelConnection struct {
	cluster *TunnelCluster
	conn    net.Conn
	active  bool
	mutex   sync.RWMutex
}

// NewTunnelCluster creates a new tunnel cluster
func NewTunnelCluster(info *TunnelInfo, options *TunnelOptions, events *TunnelEvents) (*TunnelCluster, error) {
	return &TunnelCluster{
		info:    info,
		options: options,
		events:  events,
	}, nil
}

// Start begins the cluster operation
func (tc *TunnelCluster) Start(ctx context.Context) error {
	maxConn := tc.info.MaxConn
	if maxConn <= 0 {
		maxConn = 10 // Default connection count
	}

	// Parse the tunnel URL to get connection details
	tunnelURL, err := url.Parse(tc.info.URL)
	if err != nil {
		return fmt.Errorf("invalid tunnel URL: %w", err)
	}

	host := tunnelURL.Hostname()
	if host == "" {
		return fmt.Errorf("could not determine host from URL: %s", tc.info.URL)
	}

	// Create connections
	for i := 0; i < maxConn; i++ {
		conn := &TunnelConnection{
			cluster: tc,
		}

		tc.mutex.Lock()
		tc.connections = append(tc.connections, conn)
		tc.mutex.Unlock()

		go conn.connect(ctx, host, tc.info.Port)
	}

	// Keep connections alive
	go tc.maintainConnections(ctx, host, tc.info.Port)

	return nil
}

// Close shuts down the cluster
func (tc *TunnelCluster) Close() {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	if tc.closed {
		return
	}

	tc.closed = true

	for _, conn := range tc.connections {
		conn.close()
	}
}

// maintainConnections keeps the connection pool healthy
func (tc *TunnelCluster) maintainConnections(ctx context.Context, host string, port int) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tc.checkConnections(ctx, host, port)
		}
	}
}

// checkConnections verifies and recreates dead connections
func (tc *TunnelCluster) checkConnections(ctx context.Context, host string, port int) {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	if tc.closed {
		return
	}

	for _, conn := range tc.connections {
		if !conn.isActive() {
			go conn.connect(ctx, host, port)
		}
	}
}

// connect establishes a connection to the tunnel server
func (conn *TunnelConnection) connect(ctx context.Context, host string, port int) {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	if conn.active {
		return
	}

	address := fmt.Sprintf("%s:%d", host, port)

	// Connect to the tunnel server
	netConn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		select {
		case conn.cluster.events.Error <- fmt.Errorf("failed to connect to %s: %w", address, err):
		case <-ctx.Done():
		}
		return
	}

	conn.conn = netConn
	conn.active = true

	// Handle the connection
	go conn.handleConnection(ctx)
}

// handleConnection processes incoming requests on this connection
func (conn *TunnelConnection) handleConnection(ctx context.Context) {
	defer conn.close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set read deadline
		conn.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		// Create connection to local server
		localConn, err := conn.connectToLocal()
		if err != nil {
			select {
			case conn.cluster.events.Error <- err:
			case <-ctx.Done():
			}
			continue
		}

		// Create header transformer
		transformer := NewHeaderHostTransformer(conn.cluster.options.LocalHost + fmt.Sprintf(":%d", conn.cluster.options.Port))

		// Handle the request/response cycle
		go conn.proxyConnection(localConn, transformer)
	}
}

// connectToLocal creates a connection to the local server
func (conn *TunnelConnection) connectToLocal() (net.Conn, error) {
	address := fmt.Sprintf("%s:%d", conn.cluster.options.LocalHost, conn.cluster.options.Port)

	if conn.cluster.options.LocalHTTPS {
		// Use TLS for HTTPS
		config := &tls.Config{
			InsecureSkipVerify: true, // For local development
		}
		return tls.Dial("tcp", address, config)
	}

	return net.Dial("tcp", address)
}

// proxyConnection handles bidirectional data transfer
func (conn *TunnelConnection) proxyConnection(localConn net.Conn, transformer *HeaderHostTransformer) {
	defer localConn.Close()

	// Create pipes for bidirectional communication
	done := make(chan struct{}, 2)

	// Remote -> Local (with header transformation)
	go func() {
		defer func() { done <- struct{}{} }()

		// For the first request, transform headers
		transformer.Transform(conn.conn, localConn)

		// Then copy the rest directly
		io.Copy(localConn, conn.conn)
	}()

	// Local -> Remote
	go func() {
		defer func() { done <- struct{}{} }()
		io.Copy(conn.conn, localConn)
	}()

	// Wait for either direction to complete
	<-done
}

// extractRequestInfo parses HTTP request for logging
func extractRequestInfo(data []byte) *RequestInfo {
	lines := strings.Split(string(data), "\r\n")
	if len(lines) == 0 {
		return nil
	}

	parts := strings.Fields(lines[0])
	if len(parts) < 3 {
		return nil
	}

	return &RequestInfo{
		Method: parts[0],
		Path:   parts[1],
		URL:    parts[1],
	}
}

// isActive checks if the connection is still active
func (conn *TunnelConnection) isActive() bool {
	conn.mutex.RLock()
	defer conn.mutex.RUnlock()
	return conn.active
}

// close terminates the connection
func (conn *TunnelConnection) close() {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	if !conn.active {
		return
	}

	conn.active = false
	if conn.conn != nil {
		conn.conn.Close()
		conn.conn = nil
	}
}
