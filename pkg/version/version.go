package version

import (
	"fmt"
	"runtime"
)

// Version information
var (
	// Version is the semantic version of the application
	Version = "1.0.0"

	// BuildDate is the date when the binary was built
	BuildDate = "unknown"

	// GitCommit is the git commit hash
	GitCommit = "unknown"

	// GitBranch is the git branch name
	GitBranch = "unknown"

	// GitState is the state of the git repository (clean, dirty)
	GitState = "unknown"
)

// Info contains version information
type Info struct {
	Version   string `json:"version"`
	BuildDate string `json:"buildDate"`
	GitCommit string `json:"gitCommit"`
	GitBranch string `json:"gitBranch"`
	GitState  string `json:"gitState"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

// GetVersionInfo returns the complete version information
func GetVersionInfo() Info {
	return Info{
		Version:   Version,
		BuildDate: BuildDate,
		GitCommit: GitCommit,
		GitBranch: GitBranch,
		GitState:  GitState,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a formatted version string
func String() string {
	info := GetVersionInfo()
	return fmt.Sprintf("v%s (%s, %s, %s)",
		info.Version,
		info.GitCommit,
		info.BuildDate,
		info.Platform)
}

// FullString returns a detailed version string
func FullString() string {
	info := GetVersionInfo()
	return fmt.Sprintf("Version: %s\nBuild Date: %s\nGit Commit: %s\nGit Branch: %s\nGit State: %s\nGo Version: %s\nPlatform: %s",
		info.Version,
		info.BuildDate,
		info.GitCommit,
		info.GitBranch,
		info.GitState,
		info.GoVersion,
		info.Platform)
}
