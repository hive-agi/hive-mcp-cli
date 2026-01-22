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
	// Check if ollama command exists
	if _, err := exec.LookPath("ollama"); err != nil {
		return fmt.Errorf("ollama not installed - please install from https://ollama.ai")
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
