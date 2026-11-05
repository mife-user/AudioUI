package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseModeRecognizesSDKVersionProbe(t *testing.T) {
	for _, arg := range []string{"-v", "--version", "-version"} {
		if got := parseMode([]string{arg}); got != modeVersion {
			t.Errorf("parseMode(%q) = %v, want modeVersion", arg, got)
		}
	}
}

func TestSDKVersionProbeReturnsBeforeTouchingApplicationData(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AUDIOUI_HOME", root)
	t.Setenv(sdkSkipVersionCheckEnv, "")

	originalArgs := os.Args
	os.Args = []string{"audio-ui.exe", "-v"}
	defer func() { os.Args = originalArgs }()

	if code := Run(Config{Version: "0.2.0"}); code != 0 {
		t.Fatalf("Run(-v) exit code = %d, want 0", code)
	}
	if got := os.Getenv(sdkSkipVersionCheckEnv); got != "1" {
		t.Fatalf("SDK version check env = %q, want 1", got)
	}
	for _, name := range []string{"data", "logs", "webview2"} {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("version probe touched %s: stat error = %v", path, err)
		}
	}
}
