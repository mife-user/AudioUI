//go:build !windows

package winmsg

import "fmt"

func Error(title, message string) {
	fmt.Printf("%s: %s\n", title, message)
}
