package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/korya/vrata"
)

func main() {
	// Start a simple HTTP server on port 8080
	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "Hello from Go localtunnel! Time: %s\n", time.Now().Format(time.RFC3339))
		})
		
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "OK")
		})
		
		log.Println("Starting local server on :8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatalf("Local server failed: %v", err)
		}
	}()
	
	// Give the server a moment to start
	time.Sleep(1 * time.Second)
	
	// Create tunnel options
	options := &vrata.TunnelOptions{
		Port:      8080,
		Host:      "https://localtunnel.me",
		Subdomain: "", // Let server assign random subdomain
		LocalHost: "localhost",
	}
	
	// Create and open tunnel
	tunnel, err := vrata.ConnectAndOpen(8080, options)
	if err != nil {
		log.Fatalf("Failed to create tunnel: %v", err)
	}
	defer tunnel.Close()
	
	// Get the tunnel URL
	url, err := tunnel.URL()
	if err != nil {
		log.Fatalf("Failed to get tunnel URL: %v", err)
	}
	
	fmt.Printf("🌍 Tunnel is live at: %s\n", url)
	fmt.Printf("📍 Tunneling to: http://localhost:8080\n")
	fmt.Printf("Press Ctrl+C to stop the tunnel\n\n")
	
	// Listen for events
	events := tunnel.Events()
	for {
		select {
		case req := <-events.Request:
			fmt.Printf("📞 %s %s\n", req.Method, req.Path)
		case err := <-events.Error:
			fmt.Printf("❌ Error: %v\n", err)
		case <-events.Close:
			fmt.Println("🔒 Tunnel closed")
			return
		}
	}
}