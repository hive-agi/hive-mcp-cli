package setup

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// OllamaStep ensures Ollama is running with the required model
type OllamaStep struct{}

func (s *OllamaStep) Name() string {
	return "Setup Ollama with nomic-embed-text model"
}

func (s *OllamaStep) Check() (bool, error) {
	// Check if Ollama is running
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()

	// Could parse response to check for nomic-embed-text model
	// For now, just check if Ollama is responding
	return resp.StatusCode == 200, nil
}

func (s *OllamaStep) Run() error {
	// Ollama is optional: without it memory still stores and retrieves by tag
	// and type, only semantic search is off, and the launcher boots anyway.
	// Failing setup here stopped every machine without Ollama one step short
	// of registering with Claude Code.
	if _, err := exec.LookPath("ollama"); err != nil {
		fmt.Println("    -- Ollama not installed: skipped. Semantic search stays off until you")
		fmt.Println("       install it (https://ollama.com) and run: ollama pull nomic-embed-text")
		return nil
	}

	// Pull the embedding model
	cmd := exec.Command("ollama", "pull", "nomic-embed-text")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pull nomic-embed-text: %w", err)
	}

	return nil
}

func (s *OllamaStep) Rollback() error {
	// Don't remove the model on rollback - user might want it
	return nil
}
