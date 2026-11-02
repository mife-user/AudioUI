//go:build windows

package winmsg

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32     = windows.NewLazySystemDLL("user32.dll")
	messageBox = user32.NewProc("MessageBoxW")
)

const (
	mbIconError = 0x00000010
	mbOK        = 0x00000000
)

// Error shows a native error dialog for failures that happen before Wails can
// create a window, such as an unwritable portable directory.
func Error(title, message string) {
	titlePtr, titleErr := windows.UTF16PtrFromString(title)
	messagePtr, messageErr := windows.UTF16PtrFromString(message)
	if titleErr != nil || messageErr != nil {
		return
	}
	_, _, _ = messageBox.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		mbIconError|mbOK,
	)
}
