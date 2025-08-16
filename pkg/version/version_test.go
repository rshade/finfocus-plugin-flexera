package version

import (
	"strings"
	"testing"
)

func TestGetVersionInfo(t *testing.T) {
	info := GetVersionInfo()

	// Check that required fields are present
	if info.Version == "" {
		t.Error("Version should not be empty")
	}

	if info.GoVersion == "" {
		t.Error("GoVersion should not be empty")
	}

	if info.Platform == "" {
		t.Error("Platform should not be empty")
	}

	// Check platform format
	if !strings.Contains(info.Platform, "/") {
		t.Error("Platform should contain OS/ARCH format")
	}
}

func TestString(t *testing.T) {
	result := String()

	// Should contain version
	if !strings.Contains(result, "v") {
		t.Error("String should contain version prefix")
	}

	// Should contain git commit
	if !strings.Contains(result, "unknown") && !strings.Contains(result, "(") {
		t.Error("String should contain git commit information")
	}
}

func TestFullString(t *testing.T) {
	result := FullString()

	// Should contain all version information
	expectedFields := []string{
		"Version:",
		"Build Date:",
		"Git Commit:",
		"Git Branch:",
		"Git State:",
		"Go Version:",
		"Platform:",
	}

	for _, field := range expectedFields {
		if !strings.Contains(result, field) {
			t.Errorf("FullString should contain %s", field)
		}
	}
}

func TestVersionVariables(t *testing.T) {
	// Test that version variables are accessible
	if Version == "" {
		t.Error("Version variable should not be empty")
	}

	// These might be "unknown" if not built with ldflags, which is fine
	_ = BuildDate
	_ = GitCommit
	_ = GitBranch
	_ = GitState
}
