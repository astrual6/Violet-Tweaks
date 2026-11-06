//go:build !windows

package main

import (
	"os"
	"os/exec"
)

func hideProcessWindow(c *exec.Cmd) {}
func launchWindow(url string) {
	if os.Getenv("VIOLET_NO_BROWSER") == "1" {
		return
	}
	_ = exec.Command("xdg-open", url).Start()
}
func systemMetrics() (float64, float64, float64) { return 0, 0, 0 }
