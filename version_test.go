package main

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestVersionSourcesAgree(t *testing.T) {
	versionBytes, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	version := strings.TrimSpace(string(versionBytes))
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(version) {
		t.Fatalf("VERSION = %q, want semver", version)
	}

	configBytes, err := os.ReadFile("build/config.yml")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^\s*version:\s*"([^"]+)"`).FindStringSubmatch(string(configBytes))
	if len(match) != 2 {
		t.Fatal("build/config.yml version not found")
	}
	if match[1] != version {
		t.Fatalf("build/config.yml version = %q, VERSION = %q", match[1], version)
	}

	var info struct {
		Fixed struct {
			FileVersion string `json:"file_version"`
		} `json:"fixed"`
		Info map[string]struct {
			ProductVersion string `json:"ProductVersion"`
		} `json:"info"`
	}
	infoBytes, err := os.ReadFile("build/windows/info.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(infoBytes, &info); err != nil {
		t.Fatal(err)
	}
	if info.Info["0409"].ProductVersion != version {
		t.Fatalf("info.json version = %q, VERSION = %q", info.Info["0409"].ProductVersion, version)
	}
	if info.Fixed.FileVersion != version {
		t.Fatalf("info.json file_version = %q, VERSION = %q", info.Fixed.FileVersion, version)
	}

	changelog, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(changelog), "## ["+version+"]") {
		t.Fatalf("CHANGELOG has no [%s] section", version)
	}

	var pkg struct {
		Version string `json:"version"`
	}
	pkgBytes, err := os.ReadFile("frontend/package.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(pkgBytes, &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Version != version {
		t.Fatalf("package.json version = %q, VERSION = %q", pkg.Version, version)
	}
}
