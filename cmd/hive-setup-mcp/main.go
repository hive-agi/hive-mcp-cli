// Copyright 2026 hive-agi
// SPDX-License-Identifier: MIT

package main

import (
	"log"

	"github.com/hive-agi/hive-mcp-cli/internal/hive"
	"github.com/mark3labs/mcp-go/server"
	bmcp "github.com/BuddhiLW/bonzai/mcp"
)

func main() {
	// Create MCP server from Bonzai command tree
	// OnlyTagged() ensures only commands with Mcp metadata are exposed
	s := bmcp.NewServer(hive.Cmd, bmcp.OnlyTagged())

	// Start the server with stdio transport
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
