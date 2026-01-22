package detect

import (
	"os/exec"
	"runtime"
	"strings"
)

// PlatformInfo contains information about the host platform
type PlatformInfo struct {
	Status         Status
	OS             string // "linux", "darwin"
	Distro         string // "ubuntu", "debian", "arch", etc. (Linux only)
	Version        string // OS version
	PackageManager string // "apt", "brew", "pacman", etc.
	Arch           string // "amd64", "arm64"
}

// DetectPlatform detects the current platform information
func DetectPlatform() PlatformInfo {
	info := PlatformInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	switch info.OS {
	case "linux":
		info.detectLinux()
	case "darwin":
		info.detectMacOS()
	default:
		info.Status = StatusError
		return info
	}

	// Validate package manager exists
	if info.PackageManager != "" {
		if _, err := exec.LookPath(info.PackageManager); err != nil {
			info.Status = StatusWarning
			return info
		}
	}

	info.Status = StatusOK
	return info
}

func (p *PlatformInfo) detectLinux() {
	// Try to detect the distro from /etc/os-release
	out, err := exec.Command("cat", "/etc/os-release").Output()
	if err != nil {
		p.Distro = "unknown"
		p.PackageManager = "unknown"
		return
	}

	osRelease := string(out)

	// Parse ID
	for _, line := range strings.Split(osRelease, "\n") {
		if strings.HasPrefix(line, "ID=") {
			p.Distro = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		}
		if strings.HasPrefix(line, "VERSION_ID=") {
			p.Version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		}
	}

	// Determine package manager based on distro
	switch p.Distro {
	case "ubuntu", "debian", "linuxmint", "pop":
		p.PackageManager = "apt"
	case "fedora", "rhel", "centos":
		p.PackageManager = "dnf"
	case "arch", "manjaro", "endeavouros":
		p.PackageManager = "pacman"
	case "opensuse", "opensuse-leap", "opensuse-tumbleweed":
		p.PackageManager = "zypper"
	case "alpine":
		p.PackageManager = "apk"
	default:
		// Try to detect by available commands
		p.PackageManager = detectPackageManagerByCommand()
	}
}

func (p *PlatformInfo) detectMacOS() {
	p.Distro = "macOS"

	// Get macOS version
	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err == nil {
		p.Version = strings.TrimSpace(string(out))
	}

	// macOS uses brew
	p.PackageManager = "brew"
}

func detectPackageManagerByCommand() string {
	managers := []string{"apt", "dnf", "yum", "pacman", "zypper", "apk"}
	for _, m := range managers {
		if _, err := exec.LookPath(m); err == nil {
			return m
		}
	}
	return "unknown"
}

// ShellInfo contains information about the user's shell
type ShellInfo struct {
	Status     Status
	Name       string // "bash", "zsh"
	ConfigFile string // path to config file
	Version    string
}

// DetectShell detects the user's shell
func DetectShell() ShellInfo {
	info := ShellInfo{}

	// Detect shell from SHELL env var
	shell := getEnv("SHELL", "/bin/bash")
	parts := strings.Split(shell, "/")
	info.Name = parts[len(parts)-1]

	// Get version
	var versionCmd string
	switch info.Name {
	case "bash":
		versionCmd = "bash"
	case "zsh":
		versionCmd = "zsh"
	default:
		info.Status = StatusWarning
		return info
	}

	out, err := exec.Command(versionCmd, "--version").Output()
	if err == nil {
		// Extract first line
		lines := strings.Split(string(out), "\n")
		if len(lines) > 0 {
			info.Version = strings.TrimSpace(lines[0])
		}
	}

	// Determine config file
	info.ConfigFile = detectShellConfigFile(info.Name)

	if info.ConfigFile != "" {
		info.Status = StatusOK
	} else {
		info.Status = StatusWarning
	}

	return info
}

func detectShellConfigFile(shell string) string {
	home := getEnv("HOME", "")
	if home == "" {
		return ""
	}

	var candidates []string
	switch shell {
	case "bash":
		candidates = []string{
			home + "/.bashrc",
			home + "/.bash_profile",
			home + "/.profile",
		}
	case "zsh":
		candidates = []string{
			home + "/.zshrc",
			home + "/.zprofile",
		}
	}

	for _, f := range candidates {
		if fileExists(f) {
			return f
		}
	}

	// Return default even if it doesn't exist yet
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}
