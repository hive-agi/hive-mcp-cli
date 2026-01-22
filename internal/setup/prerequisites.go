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
	required := []string{"git", "java", "clojure", "bb", "docker", "emacs"}
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
		"git", "openjdk@17", "clojure/tools/clojure", "borkdude/brew/babashka",
		"docker", "emacs-plus@29",
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
	// Check if apt is available
	if _, err := exec.LookPath("apt"); err != nil {
		return fmt.Errorf("apt not found - this step requires Debian/Ubuntu")
	}

	// Install basic packages via apt
	aptPkgs := []string{"git", "openjdk-17-jdk", "docker.io", "emacs"}
	cmd := exec.Command("sudo", append([]string{"apt", "install", "-y"}, aptPkgs...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apt install failed: %w", err)
	}

	// Install Clojure
	if err := s.installClojureLinux(); err != nil {
		return err
	}

	// Install Babashka
	if err := s.installBabashkaLinux(); err != nil {
		return err
	}

	return nil
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

func (s *PrerequisitesStep) installBabashkaLinux() error {
	// Check if already installed
	if _, err := exec.LookPath("bb"); err == nil {
		return nil
	}

	// Install via official script
	cmd := exec.Command("bash", "-c", "curl -sLO https://raw.githubusercontent.com/babashka/babashka/master/install && chmod +x install && sudo ./install && rm install")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s *PrerequisitesStep) Rollback() error {
	// Don't uninstall system packages
	return nil
}
