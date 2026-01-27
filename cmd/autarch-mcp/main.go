// Command autarch-mcp provides an MCP server for Autarch tools.
//
// This server implements the Model Context Protocol (MCP) to expose
// Autarch's PRD management, task orchestration, and research tools
// to AI agents.
//
// Usage:
//
//	autarch-mcp [flags]
//
// Flags:
//
//	-project string
//	    Project directory (default: current directory)
//	-version
//	    Print version and exit
//
// The server communicates via JSON-RPC over stdin/stdout.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mistakeknot/autarch/pkg/mcp"
)

var version = "0.1.0"

func main() {
	projectPath := flag.String("project", "", "Project directory (default: current directory)")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("autarch-mcp version %s\n", version)
		os.Exit(0)
	}

	// Default to current directory
	if *projectPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to get current directory: %v\n", err)
			os.Exit(1)
		}
		*projectPath = cwd
	}

	// Validate project path exists
	if _, err := os.Stat(*projectPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: project directory does not exist: %s\n", *projectPath)
		os.Exit(1)
	}

	// Create server
	server := mcp.NewServer(*projectPath)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Run server
	if err := server.Run(ctx); err != nil {
		if err != context.Canceled {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}
