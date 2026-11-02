//go:build windows

package environment

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"audio-ui/internal/domain/valueobject"

	"golang.org/x/sys/windows/registry"
)

const (
	webView2ClientID = `{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	webView2ExeName  = "msedgewebview2.exe"
)

func detectWebView2(ctx context.Context) valueobject.WebView2Status {
	if err := ctx.Err(); err != nil {
		return valueobject.WebView2Status{Error: err.Error()}
	}

	keys := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\` + webView2ClientID},
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\EdgeUpdate\Clients\` + webView2ClientID},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\EdgeUpdate\Clients\` + webView2ClientID},
	}
	for _, item := range keys {
		key, err := registry.OpenKey(item.root, item.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		version, _, versionErr := key.GetStringValue("pv")
		_ = key.Close()
		if versionErr == nil && validWebView2Version(version) {
			return valueobject.WebView2Status{
				Installed: true,
				Version:   strings.TrimSpace(version),
			}
		}
	}

	if folder := strings.TrimSpace(os.Getenv("WEBVIEW2_BROWSER_EXECUTABLE_FOLDER")); folder != "" {
		if hasWebView2Executable(folder) {
			return valueobject.WebView2Status{Installed: true, Path: folder}
		}
	}

	roots := []string{
		os.Getenv("ProgramFiles(x86)"),
		os.Getenv("ProgramFiles"),
		os.Getenv("LOCALAPPDATA"),
	}
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		candidate := filepath.Join(root, "Microsoft", "EdgeWebView", "Application")
		version := newestWebView2Version(candidate)
		if version == "" {
			continue
		}
		return valueobject.WebView2Status{Installed: true, Path: candidate, Version: version}
	}

	return valueobject.WebView2Status{Error: "未找到 WebView2 运行时"}
}

func validWebView2Version(version string) bool {
	version = strings.TrimSpace(version)
	return version != "" && version != "0.0.0.0"
}

func hasWebView2Executable(folder string) bool {
	info, err := os.Stat(filepath.Join(folder, webView2ExeName))
	return err == nil && !info.IsDir()
}

func newestWebView2Version(root string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	newest := ""
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		version := entry.Name()
		if !hasWebView2Executable(filepath.Join(root, version)) {
			continue
		}
		if newest == "" || compareVersions(version, newest) > 0 {
			newest = version
		}
	}
	return newest
}

func compareVersions(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")
	count := len(partsA)
	if len(partsB) > count {
		count = len(partsB)
	}
	for i := 0; i < count; i++ {
		var left, right int
		if i < len(partsA) {
			left, _ = strconv.Atoi(partsA[i])
		}
		if i < len(partsB) {
			right, _ = strconv.Atoi(partsB[i])
		}
		if left > right {
			return 1
		}
		if left < right {
			return -1
		}
	}
	return strings.Compare(a, b)
}
