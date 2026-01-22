package setup

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// ChromaStep starts Docker services (Chroma)
type ChromaStep struct {
	HiveMCPDir string
}

func (s *ChromaStep) Name() string {
	return "Start Docker services (Chroma)"
}

func (s *ChromaStep) hiveMCPDir() string {
	if s.HiveMCPDir != "" {
		return expandPath(s.HiveMCPDir)
	}
	return DefaultHiveMCPDir()
}

func (s *ChromaStep) Check() (bool, error) {
	// Check if Chroma is already responding
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:8000/api/v2/heartbeat")
	if err == nil {
		resp.Body.Close()
		return resp.StatusCode == 200, nil
	}
	return false, nil
}

func (s *ChromaStep) Run() error {
	// First ensure Docker is running
	if err := exec.Command("docker", "info").Run(); err != nil {
		return fmt.Errorf("docker is not running: %w", err)
	}

	// Start Chroma using docker-compose in hive-mcp directory
	dir := s.hiveMCPDir()
	cmd := exec.Command("docker", "compose", "up", "-d", "chroma")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start Chroma: %w", err)
	}

	// Wait for Chroma to be ready (up to 30 seconds)
	client := &http.Client{Timeout: 2 * time.Second}
	for i := 0; i < 15; i++ {
		resp, err := client.Get("http://localhost:8000/api/v2/heartbeat")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("Chroma failed to start within 30 seconds")
}

func (s *ChromaStep) Rollback() error {
	dir := s.hiveMCPDir()
	cmd := exec.Command("docker", "compose", "down")
	cmd.Dir = dir
	return cmd.Run()
}
