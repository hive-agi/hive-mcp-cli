package setup

import (
	"fmt"
	"os"
	"os/exec"
)

// PrerequisitesStep installs system prerequisites
type PrerequisitesStep struct {
	Platform string // "linux" or "darwin"
}

func (s *PrerequisitesStep) Name() string {
	return "Install system prerequisites"
}

func (s *PrerequisitesStep) Check() (bool, error) {
	// Check for key binaries
	// Emacs, tmux and Babashka are optional: the starter pack degrades the
	// matching addon when its host tool is absent.
	required := []string{"git", "java", "clojure", "docker"}
	for _, bin := range required {
		if _, err := exec.LookPath(bin); err != nil {
			return false, nil
		}
	}
	return true, nil
}

func (s *PrerequisitesStep) Run() error {
	switch s.Platform {
	case "darwin":
		return s.installDarwin()
	case "linux":
		return s.installLinux()
	default:
		return fmt.Errorf("unsupported platform: %s", s.Platform)
	}
}

func (s *PrerequisitesStep) installDarwin() error {
	// Check if Homebrew is available
	if _, err := exec.LookPath("brew"); err != nil {
		return fmt.Errorf("Homebrew not found - please install from https://brew.sh")
	}

	packages := []string{
		"git", "openjdk@21", "clojure/tools/clojure", "docker",
	}

	for _, pkg := range packages {
		cmd := exec.Command("brew", "install", pkg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// Continue even if some packages fail (might already be installed differently)
		cmd.Run()
	}

	return nil
}

func (s *PrerequisitesStep) installLinux() error {
	// apt-get, not apt: apt warns that its CLI is unstable in scripts.
	if _, err := exec.LookPath("apt-get"); err != nil {
		return fmt.Errorf("apt-get not found - this step requires Debian/Ubuntu")
	}

	// A fresh cloud image ships with empty package lists, so every install
	// below reports "has no installation candidate" until they are fetched.
	if err := sudoRun("apt-get", "update"); err != nil {
		return fmt.Errorf("apt-get update failed: %w", err)
	}

	// docker-compose-v2 provides `docker compose`, which the Chroma step runs;
	// docker.io alone does not carry it on Ubuntu. The headless JDK is enough
	// for a host with no UI, and skips the GTK stack the full one drags in.
	aptPkgs := []string{"git", "curl", "rlwrap", "openjdk-21-jdk-headless", "docker.io", "docker-compose-v2"}
	if err := sudoRun(append([]string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y"}, aptPkgs...)...); err != nil {
		return fmt.Errorf("apt-get install failed: %w", err)
	}

	// Without the group, every docker call below needs sudo. The membership
	// only reaches new logins; dockerCommand bridges this run with sg.
	if u := os.Getenv("USER"); u != "" && u != "root" {
		if err := sudoRun("usermod", "-aG", "docker", u); err != nil {
			return fmt.Errorf("adding %s to the docker group failed: %w", u, err)
		}
	}

	// Install Clojure
	return s.installClojureLinux()
}

func sudoRun(args ...string) error {
	cmd := exec.Command("sudo", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *PrerequisitesStep) installClojureLinux() error {
	// Check if already installed
	if _, err := exec.LookPath("clojure"); err == nil {
		return nil
	}

	// Install via official script
	script := `
curl -L -O https://github.com/clojure/brew-install/releases/latest/download/linux-install.sh
chmod +x linux-install.sh
sudo ./linux-install.sh
rm linux-install.sh
`
	cmd := exec.Command("bash", "-c", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *PrerequisitesStep) Rollback() error {
	// Don't uninstall system packages
	return nil
}
