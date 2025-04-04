package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	port = ":3456"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", requestLoggerHandler)

	server := &http.Server{
		Addr:    port,
		Handler: mux,
	}

	// Channel to listen for OS signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Goroutine to start the server
	go func() {
		log.Println("Starting server on", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe error: %v", err)
		}
	}()

	// Wait for a signal
	<-stop
	log.Println("Shutting down server...")

	// Create a context with a timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server Shutdown Failed:%+v", err)
	}

	log.Println("Server gracefully stopped")
}

func requestLoggerHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("--- New Request ---")
	fmt.Printf("Method: %s\n", r.Method)
	fmt.Printf("Path: %s\n", r.URL.Path)
	fmt.Printf("Query: %s\n", r.URL.RawQuery)

	fmt.Println("Headers:")
	for name, headers := range r.Header {
		for _, h := range headers {
			fmt.Printf("  %v: %v\n", name, h)
		}
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close() // Ensure body is closed

	bodyString := string(bodyBytes)
	if len(bodyString) > 0 {
		fmt.Printf("Body:\n%s\n", bodyString)
	} else {
		fmt.Println("Body: (empty)")
	}

	fmt.Printf("--- End Request ---\n\n")

	// Send a simple response
	fmt.Fprintln(w, "Request received and logged.")
}
