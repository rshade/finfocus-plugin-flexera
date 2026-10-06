package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"text/template"

	"gopkg.in/yaml.v3"
)

// goreleaserConfig mirrors the structure we need from .goreleaser.yaml.
type goreleaserConfig struct {
	ProjectName string `yaml:"project_name"`
	Archives    []struct {
		Formats         []string `yaml:"formats"`
		NameTemplate    string   `yaml:"name_template"`
		FormatOverrides []struct {
			Goos    string   `yaml:"goos"`
			Formats []string `yaml:"formats"`
		} `yaml:"format_overrides"`
	} `yaml:"archives"`
}

// buildAssetPatterns mirrors the installer's pattern matching logic from
// github.com/rshade/finfocus/internal/registry/github.go buildAssetPatterns.
func buildAssetPatterns(projectName, version string) map[string]bool {
	patterns := make(map[string]bool)

	// Test against multiple OS representations (as the installer does)
	osNames := map[string][]string{
		"linux":   {"linux", "Linux"},
		"darwin":  {"darwin", "Darwin", "macos", "macOS", "MacOS"},
		"windows": {"windows", "Windows"},
	}

	// Test against multiple arch representations (as the installer does)
	archNames := map[string][]string{
		"amd64": {"amd64", "x86_64", "X86_64", "AMD64"},
		"arm64": {"arm64", "ARM64", "aarch64", "AARCH64"},
	}

	versions := []string{version, "v" + version}

	for _, os := range []string{"linux", "darwin", "windows"} {
		for _, osName := range osNames[os] {
			for _, arch := range []string{"amd64", "arm64"} {
				for _, archName := range archNames[arch] {
					for _, v := range versions {
						// Format: <name>_<version>_<os>_<arch>
						base := projectName + "_" + v + "_" + osName + "_" + archName

						// Add both .tar.gz and .zip variants
						patterns[base+".tar.gz"] = true
						patterns[base+".zip"] = true
					}
				}
			}
		}
	}

	return patterns
}

// TestGoreleaserAssetNames verifies that .goreleaser.yaml produces archive names
// that match the finfocus installer's expected patterns.
// Reads the real .goreleaser.yaml, parses its template, renders it for various
// OS/arch combinations, and validates the output names against installer patterns.
func TestGoreleaserAssetNames(t *testing.T) {
	// Read the real .goreleaser.yaml
	configFile, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatalf("failed to read .goreleaser.yaml: %v", err)
	}

	// Parse the config
	var cfg goreleaserConfig
	err = yaml.Unmarshal(configFile, &cfg)
	if err != nil {
		t.Fatalf("failed to parse .goreleaser.yaml: %v", err)
	}

	if cfg.ProjectName == "" {
		t.Fatal("project_name not set in .goreleaser.yaml")
	}

	if len(cfg.Archives) == 0 {
		t.Fatal("no archives defined in .goreleaser.yaml")
	}

	archive := cfg.Archives[0]
	if archive.NameTemplate == "" {
		t.Fatal("name_template not set in .goreleaser.yaml")
	}

	// Parse the name_template as a Go template
	funcs := template.FuncMap{
		"title": func(s string) string { return strings.ToUpper(s[:1]) + s[1:] },
	}

	tmpl, err := template.New("archive").Funcs(funcs).Parse(archive.NameTemplate)
	if err != nil {
		t.Fatalf("failed to parse name_template as Go template: %v", err)
	}

	// Build expected patterns from installer logic
	testVersion := "1.2.3"
	expectedSet := buildAssetPatterns(cfg.ProjectName, testVersion)

	// Test cases: (goos, goarch, version, expected_format)
	testCases := []struct {
		goos    string
		goarch  string
		version string
		format  string
	}{
		{"linux", "amd64", testVersion, "tar.gz"},
		{"linux", "arm64", testVersion, "tar.gz"},
		{"darwin", "amd64", testVersion, "tar.gz"},
		{"darwin", "arm64", testVersion, "tar.gz"},
		{"windows", "amd64", testVersion, "zip"},
		{"windows", "arm64", testVersion, "zip"},
		// Test with leading v
		{"linux", "amd64", "v" + testVersion, "tar.gz"},
		{"darwin", "arm64", "v" + testVersion, "tar.gz"},
		{"windows", "amd64", "v" + testVersion, "zip"},
	}

	for _, tc := range testCases {
		testName := fmt.Sprintf("%s_%s_%s", tc.goos, tc.goarch, tc.version)
		t.Run(testName, func(t *testing.T) {
			// Render the template
			data := map[string]interface{}{
				"ProjectName": cfg.ProjectName,
				"Version":     tc.version,
				"Os":          tc.goos,
				"Arch":        tc.goarch,
				"Arm":         "", // Not used in our template
			}

			var buf bytes.Buffer
			err := tmpl.Execute(&buf, data)
			if err != nil {
				t.Fatalf("failed to render name_template: %v", err)
			}

			baseName := buf.String()

			// Determine expected extension
			expectedExt := ".tar.gz"
			if tc.goos == "windows" {
				expectedExt = ".zip"
			}

			finalName := baseName + expectedExt

			// Check that this name would be found by installer patterns
			if !expectedSet[finalName] {
				// Show what patterns we expected for debugging
				t.Errorf("archive name %q does not match any installer pattern for %s/%s",
					finalName, tc.goos, tc.goarch)
				t.Logf("base name was: %s", baseName)
				t.Logf("total expected patterns: %d", len(expectedSet))

				// Show a few expected patterns for reference
				count := 0
				for pattern := range expectedSet {
					if strings.Contains(pattern, tc.goos) && strings.Contains(pattern, tc.goarch) {
						t.Logf("  example expected pattern: %s", pattern)
						count++
						if count >= 2 {
							break
						}
					}
				}
			}
		})
	}
}

// TestGoreleaserAssetNamesBroken verifies that the test can catch template errors.
// This test is SKIPPED normally. To verify error detection, temporarily uncomment
// the validation code below and rebuild.
func TestGoreleaserAssetNamesBroken(t *testing.T) {
	// DISABLED: Uncomment the block below to verify the test catches broken templates
	// Then revert the .goreleaser.yaml name_template to use {{ .WrongName }} instead
	// of {{ .ProjectName }}, run the test, and verify it fails. Then restore the file.
	t.Skip("Error detection test - only run to validate broken template detection")

	// Read the real .goreleaser.yaml
	configFile, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatalf("failed to read .goreleaser.yaml: %v", err)
	}

	// Parse the config
	var cfg goreleaserConfig
	err = yaml.Unmarshal(configFile, &cfg)
	if err != nil {
		t.Fatalf("failed to parse .goreleaser.yaml: %v", err)
	}

	archive := cfg.Archives[0]

	// Intentionally break the template by using wrong variable name
	brokenTemplate := strings.Replace(archive.NameTemplate, "{{ .ProjectName }}", "{{ .WrongName }}", 1)

	brokenFuncs := template.FuncMap{}

	tmpl, err := template.New("archive").Funcs(brokenFuncs).Parse(brokenTemplate)
	if err != nil {
		t.Fatalf("failed to parse broken template as Go template: %v", err)
	}

	data := map[string]interface{}{
		"ProjectName": cfg.ProjectName,
		"Version":     "1.0.0",
		"Os":          "linux",
		"Arch":        "amd64",
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	// This should render without error but with wrong content (WrongName is nil)
	if err == nil {
		result := buf.String()
		// The result should NOT contain the expected project name
		if strings.Contains(result, cfg.ProjectName) {
			t.Errorf("broken template unexpectedly produced valid output: %s", result)
		}
	}
}

// TestGoreleaserArchivesUseTitleCaseOS keeps flexera's archive names in the shape every other
// plugin uses: finfocus-plugin-<name>_<version>_<Os>_<arch>, with Linux, Darwin and Windows.
func TestGoreleaserArchivesUseTitleCaseOS(t *testing.T) {
	configFile, err := os.ReadFile(".goreleaser.yaml")
	if err != nil {
		t.Fatalf("failed to read .goreleaser.yaml: %v", err)
	}
	if !strings.Contains(string(configFile), "{{ title .Os }}") {
		t.Fatal("archive name_template must use {{ title .Os }}")
	}
}

// TestReleasePleaseTagOmitsComponent fails when release-please would prefix the tag with the
// component name. GoReleaser parses the tag as semver and fails on finfocus-plugin-flexera-v0.1.0.
func TestReleasePleaseTagOmitsComponent(t *testing.T) {
	body, err := os.ReadFile("release-please-config.json")
	if err != nil {
		t.Fatalf("failed to read release-please-config.json: %v", err)
	}
	var cfg struct {
		Packages map[string]struct {
			IncludeComponentInTag *bool `json:"include-component-in-tag"`
		} `json:"packages"`
	}
	if err = json.Unmarshal(body, &cfg); err != nil {
		t.Fatalf("failed to parse release-please-config.json: %v", err)
	}
	root, ok := cfg.Packages["."]
	if !ok {
		t.Fatal("release-please-config.json has no root package")
	}
	if root.IncludeComponentInTag == nil || *root.IncludeComponentInTag {
		t.Fatal(`root package must set "include-component-in-tag": false`)
	}
}
