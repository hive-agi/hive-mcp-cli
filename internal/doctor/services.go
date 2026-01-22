package doctor

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// CheckServices runs all service health checks
func CheckServices() []CheckResult {
	return []CheckResult{
		checkEmacsDaemon(),
		checkChromaService(),
		checkOllamaService(),
	}
}

func checkEmacsDaemon() CheckResult {
	result := CheckResult{
		Name:    "Emacs Daemon",
		FixHint: "Start daemon: emacs --daemon",
		CanFix:  true,
		Fix:     startEmacsDaemon,
	}

	// Check if emacsclient can connect to the daemon
	cmd := exec.Command("emacsclient", "--eval", "(emacs-pid)")
	out, err := cmd.CombinedOutput()
	if err != nil {
		result.Status = StatusError
		result.Message = "not running"
		return result
	}

	pid := strings.TrimSpace(string(out))
	if pid != "" && pid != "nil" {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("running (PID %s)", pid)
	} else {
		result.Status = StatusError
		result.Message = "not running"
	}

	return result
}

func startEmacsDaemon() error {
	cmd := exec.Command("emacs", "--daemon")
	return cmd.Run()
}

func checkChromaService() CheckResult {
	result := CheckResult{
		Name:    "Chroma (Vector DB)",
		FixHint: "Start Chroma: docker run -d -p 8000:8000 chromadb/chroma",
		CanFix:  true,
		Fix:     startChromaContainer,
	}

	// Try HTTP health endpoint
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:8000/api/v2/heartbeat")
	if err != nil {
		// Try plain TCP connection as fallback
		conn, tcpErr := net.DialTimeout("tcp", "localhost:8000", 2*time.Second)
		if tcpErr != nil {
			result.Status = StatusError
			result.Message = "not running on port 8000"
			return result
		}
		conn.Close()
		result.Status = StatusOK
		result.Message = "running on localhost:8000"
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		result.Status = StatusOK
		result.Message = "healthy (localhost:8000)"
	} else {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("unhealthy (status %d)", resp.StatusCode)
	}

	return result
}

func startChromaContainer() error {
	// Check if container already exists (stopped)
	checkCmd := exec.Command("docker", "ps", "-a", "--filter", "name=chroma", "--format", "{{.Names}}")
	out, err := checkCmd.Output()
	if err == nil && strings.TrimSpace(string(out)) == "chroma" {
		// Container exists, try to start it
		startCmd := exec.Command("docker", "start", "chroma")
		if err := startCmd.Run(); err == nil {
			return nil
		}
	}

	// Run new container
	cmd := exec.Command("docker", "run", "-d",
		"--name", "chroma",
		"-p", "8000:8000",
		"-v", "chroma-data:/chroma/chroma",
		"chromadb/chroma")
	return cmd.Run()
}

func checkOllamaService() CheckResult {
	result := CheckResult{
		Name:    "Ollama (LLM)",
		FixHint: "Start Ollama: ollama serve",
		CanFix:  true,
		Fix:     startOllama,
	}

	// Try HTTP endpoint
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		// Try plain TCP connection as fallback
		conn, tcpErr := net.DialTimeout("tcp", "localhost:11434", 2*time.Second)
		if tcpErr != nil {
			result.Status = StatusError
			result.Message = "not running on port 11434"
			return result
		}
		conn.Close()
		result.Status = StatusOK
		result.Message = "running on localhost:11434"
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		result.Status = StatusOK
		result.Message = "healthy (localhost:11434)"
	} else {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("unhealthy (status %d)", resp.StatusCode)
	}

	return result
}

func startOllama() error {
	// Start ollama serve in background
	cmd := exec.Command("ollama", "serve")
	return cmd.Start() // Don't wait, it's a daemon
}

// CheckObservability checks optional observability stack
func CheckObservability() []CheckResult {
	return []CheckResult{
		checkPrometheus(),
		checkGrafana(),
		checkLoki(),
	}
}

func checkPrometheus() CheckResult {
	result := CheckResult{
		Name:    "Prometheus",
		FixHint: "Optional: Deploy via hive-mcp observability stack",
	}

	// Check if Prometheus is running
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:9090/-/healthy")
	if err != nil {
		result.Status = StatusWarning
		result.Message = "not running (optional)"
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		result.Status = StatusOK
		result.Message = "healthy (localhost:9090)"
	} else {
		result.Status = StatusWarning
		result.Message = "unhealthy"
	}

	return result
}

func checkGrafana() CheckResult {
	result := CheckResult{
		Name:    "Grafana",
		FixHint: "Optional: Deploy via hive-mcp observability stack",
	}

	// Check if Grafana is running
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:3000/api/health")
	if err != nil {
		result.Status = StatusWarning
		result.Message = "not running (optional)"
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		result.Status = StatusOK
		result.Message = "healthy (localhost:3000)"
	} else {
		result.Status = StatusWarning
		result.Message = "unhealthy"
	}

	return result
}

func checkLoki() CheckResult {
	result := CheckResult{
		Name:    "Loki",
		FixHint: "Optional: Deploy via hive-mcp observability stack",
	}

	// Check if Loki is running
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:3100/ready")
	if err != nil {
		result.Status = StatusWarning
		result.Message = "not running (optional)"
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		result.Status = StatusOK
		result.Message = "healthy (localhost:3100)"
	} else {
		result.Status = StatusWarning
		result.Message = "unhealthy"
	}

	return result
}
