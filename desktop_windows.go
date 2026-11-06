//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func hideProcessWindow(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
func launchWindow(url string) {
	if os.Getenv("VIOLET_NO_BROWSER") == "1" {
		return
	}
	pf := os.Getenv("PROGRAMFILES")
	pf86 := os.Getenv("PROGRAMFILES(X86)")
	local := os.Getenv("LOCALAPPDATA")
	paths := []string{filepath.Join(pf86, "Microsoft", "Edge", "Application", "msedge.exe"), filepath.Join(pf, "Microsoft", "Edge", "Application", "msedge.exe"), filepath.Join(local, "Microsoft", "Edge", "Application", "msedge.exe"), filepath.Join(pf, "Google", "Chrome", "Application", "chrome.exe"), filepath.Join(pf86, "Google", "Chrome", "Application", "chrome.exe"), filepath.Join(local, "Google", "Chrome", "Application", "chrome.exe")}
	// An isolated app profile keeps browser extensions, saved auto-translation
	// preferences, and another already-open Edge/Chrome instance from affecting
	// the desktop toolkit. It does not change the user's primary browser.
	profileRoot := filepath.Join(local, "VioletTweaks", "BrowserProfile")
	if err := os.MkdirAll(profileRoot, 0700); err != nil {
		profileRoot = filepath.Join(os.TempDir(), "VioletTweaksBrowserProfile")
		_ = os.MkdirAll(profileRoot, 0700)
	}
	for _, p := range paths {
		if _, e := os.Stat(p); e == nil {
			cmd := exec.Command(p,
				"--app="+url,
				"--user-data-dir="+profileRoot,
				"--window-size=1350,880",
				"--no-first-run",
				"--no-default-browser-check",
				"--disable-extensions",
				"--disable-features=Translate,TranslateUI",
				"--lang=en-US",
			)
			if cmd.Start() == nil {
				return
			}
		}
	}
	c := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
	_ = c.Start()
}

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var procGetSystemTimes = kernel32.NewProc("GetSystemTimes")
var procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")

type fileTime struct{ Low, High uint32 }

func (f fileTime) uint64() uint64 { return uint64(f.High)<<32 | uint64(f.Low) }

type memoryStatus struct {
	Size                                                                                  uint32
	Load                                                                                  uint32
	TotalPhys, AvailPhys, TotalPage, AvailPage, TotalVirtual, AvailVirtual, AvailExtended uint64
}

func systemMetrics() (float64, float64, float64) {
	mem := memoryStatus{Size: uint32(unsafe.Sizeof(memoryStatus{}))}
	used, total := 0.0, 0.0
	if ok, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem))); ok != 0 {
		total = float64(mem.TotalPhys) / 1024 / 1024 / 1024
		used = float64(mem.TotalPhys-mem.AvailPhys) / 1024 / 1024 / 1024
	}
	snap := func() (uint64, uint64) {
		var idle, kernel, user fileTime
		ok, _, _ := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
		if ok == 0 {
			return 0, 0
		}
		return idle.uint64(), kernel.uint64() + user.uint64()
	}
	i1, t1 := snap()
	time.Sleep(250 * time.Millisecond)
	i2, t2 := snap()
	cpu := 0.0
	if t2 > t1 {
		cpu = 100 * (1 - float64(i2-i1)/float64(t2-t1))
	}
	if cpu < 0 {
		cpu = 0
	}
	if cpu > 100 {
		cpu = 100
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(strconv.FormatFloat(cpu, 'f', 1, 64)), 64)
	return v, used, total
}
