package detect

import (
	"os"
)

// EnvVarCheck contains the result of an environment variable check
type EnvVarCheck struct {
	Status    Status
	Name      string
	Value     string
	Required  bool
	Sensitive bool // mask value in output
}

// envVarSpec defines an environment variable to check
type envVarSpec struct {
	name      string
	required  bool
	sensitive bool
}

var envVars = []envVarSpec{
	{name: "HIVE_MCP_DIR", required: true, sensitive: false},
	{name: "BB_MCP_DIR", required: true, sensitive: false},
	{name: "OPENROUTER_API_KEY", required: false, sensitive: true},
	{name: "HOME", required: true, sensitive: false},
	{name: "SHELL", required: true, sensitive: false},
}

// CheckAllEnvVars checks all environment variables
func CheckAllEnvVars() []EnvVarCheck {
	results := make([]EnvVarCheck, 0, len(envVars))
	for _, spec := range envVars {
		results = append(results, checkEnvVar(spec))
	}
	return results
}

func checkEnvVar(spec envVarSpec) EnvVarCheck {
	check := EnvVarCheck{
		Name:      spec.name,
		Required:  spec.required,
		Sensitive: spec.sensitive,
	}

	value := os.Getenv(spec.name)
	check.Value = value

	if value != "" {
		check.Status = StatusOK
	} else if spec.required {
		check.Status = StatusMissing
	} else {
		check.Status = StatusWarning
	}

	return check
}

// Helper functions used across the package

// getEnv returns the value of an environment variable or a default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// dirExists checks if a directory exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// isExecutable checks if a file is executable
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode()&0111 != 0
}
