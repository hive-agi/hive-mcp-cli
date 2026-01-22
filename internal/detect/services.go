package detect

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// ServiceCheck contains the result of a service check
type ServiceCheck struct {
	Status   Status
	Name     string
	Endpoint string // host:port or URL
	Message  string
}

// serviceSpec defines a service to check
type serviceSpec struct {
	name    string
	checkFn func() ServiceCheck
}

var services = []serviceSpec{
	{name: "Emacs Daemon", checkFn: checkEmacsDaemon},
	{name: "Chroma", checkFn: checkChroma},
	{name: "Ollama", checkFn: checkOllama},
}

// CheckAllServices checks all services
func CheckAllServices() []ServiceCheck {
	results := make([]ServiceCheck, 0, len(services))
	for _, spec := range services {
		check := spec.checkFn()
		check.Name = spec.name
		results = append(results, check)
	}
	return results
}

func checkEmacsDaemon() ServiceCheck {
	check := ServiceCheck{}

	// Check if emacsclient can connect to the daemon
	cmd := exec.Command("emacsclient", "--eval", "(emacs-pid)")
	out, err := cmd.CombinedOutput()
	if err != nil {
		check.Status = StatusMissing
		check.Message = "Emacs daemon not running"
		return check
	}

	pid := strings.TrimSpace(string(out))
	if pid != "" && pid != "nil" {
		check.Status = StatusOK
		check.Endpoint = fmt.Sprintf("PID %s", pid)
	} else {
		check.Status = StatusMissing
		check.Message = "Emacs daemon not running"
	}

	return check
}

func checkChroma() ServiceCheck {
	check := ServiceCheck{
		Endpoint: "localhost:8000",
	}

	// Try HTTP health endpoint
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:8000/api/v2/heartbeat")
	if err != nil {
		// Try plain TCP connection as fallback
		conn, err := net.DialTimeout("tcp", "localhost:8000", 2*time.Second)
		if err != nil {
			check.Status = StatusMissing
			check.Message = "Chroma not running on port 8000"
			return check
		}
		conn.Close()
		check.Status = StatusOK
		return check
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		check.Status = StatusOK
	} else {
		check.Status = StatusWarning
		check.Message = fmt.Sprintf("Chroma returned status %d", resp.StatusCode)
	}

	return check
}

func checkOllama() ServiceCheck {
	check := ServiceCheck{
		Endpoint: "localhost:11434",
	}

	// Try HTTP endpoint
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		// Try plain TCP connection as fallback
		conn, err := net.DialTimeout("tcp", "localhost:11434", 2*time.Second)
		if err != nil {
			check.Status = StatusMissing
			check.Message = "Ollama not running on port 11434"
			return check
		}
		conn.Close()
		check.Status = StatusOK
		return check
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		check.Status = StatusOK
	} else {
		check.Status = StatusWarning
		check.Message = fmt.Sprintf("Ollama returned status %d", resp.StatusCode)
	}

	return check
}
