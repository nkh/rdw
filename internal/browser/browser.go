// Package browser opens a URL in the default system browser.
package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Open opens url in the default browser. It returns an error if no browser
// launcher was found, but does not wait for the browser to exit.
func Open(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		return fmt.Errorf("browser: unsupported OS %q", runtime.GOOS)
	}

	return exec.Command(cmd, args...).Start()
}
