//go:build !windows

package environment

import (
	"context"

	"audio-ui/internal/domain/valueobject"
)

func detectWebView2(_ context.Context) valueobject.WebView2Status {
	return valueobject.WebView2Status{Error: "当前平台不支持 WebView2 检测"}
}
