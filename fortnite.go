package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func nvidiaMetrics() (any, any) {
	if runtime.GOOS != "windows" {
		return nil, nil
	}
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		path = filepath.Join(os.Getenv("PROGRAMFILES"), "NVIDIA Corporation", "NVSMI", "nvidia-smi.exe")
		if _, e := os.Stat(path); e != nil {
			return nil, nil
		}
	}
	out, err := runCmd(3*time.Second, path, "--query-gpu=utilization.gpu,temperature.gpu", "--format=csv,noheader,nounits")
	if err != nil {
		return nil, nil
	}
	line := strings.Split(out, "\n")[0]
	s := strings.Split(line, ",")
	if len(s) != 2 {
		return nil, nil
	}
	gpu, e1 := strconv.Atoi(strings.TrimSpace(s[0]))
	temp, e2 := strconv.Atoi(strings.TrimSpace(s[1]))
	var g, t any
	if e1 == nil {
		g = gpu
	}
	if e2 == nil {
		t = temp
	}
	return g, t
}
func findFortniteINI() (string, error) {
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		return "", errors.New("LOCALAPPDATA not available")
	}
	for _, folder := range []string{"WindowsClient", "Windows"} {
		path := filepath.Join(root, "FortniteGame", "Saved", "Config", folder, "GameUserSettings.ini")
		if info, e := os.Stat(path); e == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", errors.New("Fortnite GameUserSettings.ini not found. Install and launch Fortnite once, then close it before applying a profile")
}
func fortniteRunning() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	out, _ := runCmd(7*time.Second, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `if(Get-Process -Name 'FortniteClient-Win64-Shipping' -ErrorAction SilentlyContinue){'RUNNING'}`)
	return strings.Contains(out, "RUNNING")
}
func updateINI(data string, changes map[string]map[string]string) string {
	newline := "\r\n"
	if !strings.Contains(data, "\r\n") {
		newline = "\n"
	}
	result := []string{}
	section := ""
	found := map[string]map[string]bool{}
	for group := range changes {
		found[group] = map[string]bool{}
	}
	for _, line := range strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = trimmed
		}
		if k, _, ok := strings.Cut(trimmed, "="); ok {
			k = strings.TrimSpace(k)
			if group, ok := changes[section]; ok {
				if val, ok := group[k]; ok {
					line = k + "=" + val
					found[section][k] = true
				}
			}
		}
		result = append(result, line)
	}
	for group, kv := range changes {
		marker := -1
		for i, line := range result {
			if strings.TrimSpace(line) == group {
				marker = i
				break
			}
		}
		if marker < 0 {
			result = append(result, "", group)
			marker = len(result) - 1
		}
		insertAt := marker + 1
		for insertAt < len(result) && !strings.HasPrefix(strings.TrimSpace(result[insertAt]), "[") {
			insertAt++
		}
		add := []string{}
		for k, val := range kv {
			if !found[group][k] {
				add = append(add, k+"="+val)
			}
		}
		result = append(result[:insertAt], append(add, result[insertAt:]...)...)
	}
	return strings.Join(result, newline)
}
func (a *App) fortnite(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Mode       string `json:"mode"`
		Quality    int    `json:"quality"`
		FPS        int    `json:"fps"`
		Resolution string `json:"resolution"`
	}
	if err := parseJSON(r, &p); err != nil {
		fail(w, 400, err)
		return
	}
	if runtime.GOOS != "windows" {
		fail(w, 422, errors.New("Fortnite settings require Windows"))
		return
	}
	if fortniteRunning() {
		fail(w, 409, errors.New("Close Fortnite completely before editing settings"))
		return
	}
	filename, err := findFortniteINI()
	if err != nil {
		fail(w, 404, err)
		return
	}
	if p.Mode == "restore" {
		a.mu.Lock()
		backup := a.state.LastFortniteBackup
		a.mu.Unlock()
		if backup == "" {
			fail(w, 404, errors.New("No Fortnite backup created by this app"))
			return
		}
		content, err := os.ReadFile(backup)
		if err != nil {
			fail(w, 500, err)
			return
		}
		if err = os.WriteFile(filename, content, 0644); err != nil {
			fail(w, 500, err)
			return
		}
		a.record("Fortnite settings", "Restored last saved backup", true)
		jsonOut(w, map[string]any{"ok": true, "message": "Fortnite settings restored from backup"})
		return
	}
	if p.Mode != "apply" {
		fail(w, 400, errors.New("Use mode 'apply' or 'restore'"))
		return
	}
	if p.Quality < 25 || p.Quality > 100 || p.Quality%5 != 0 {
		fail(w, 400, errors.New("Quality must be 25–100 in steps of 5"))
		return
	}
	allowedFPS := map[int]bool{0: true, 60: true, 120: true, 144: true, 165: true, 180: true, 240: true, 360: true}
	if !allowedFPS[p.FPS] {
		fail(w, 400, errors.New("Invalid FPS limit"))
		return
	}
	allowedRes := map[string][2]int{"native": {0, 0}, "1280x720": {1280, 720}, "1024x768": {1024, 768}, "960x540": {960, 540}}
	res, ok := allowedRes[p.Resolution]
	if !ok {
		fail(w, 400, errors.New("Invalid resolution"))
		return
	}
	raw, err := os.ReadFile(filename)
	if err != nil {
		fail(w, 500, err)
		return
	}
	if len(raw) >= 2 && ((raw[0] == 0xff && raw[1] == 0xfe) || (raw[0] == 0xfe && raw[1] == 0xff)) {
		fail(w, 422, errors.New("UTF-16 INI file: encoding cannot be safely rewritten by this version"))
		return
	}
	backup := filename + ".violettweaks-backup-" + time.Now().Format("20060102-150405")
	if err = os.WriteFile(backup, raw, 0600); err != nil {
		fail(w, 500, fmt.Errorf("Could not create backup: %w", err))
		return
	}
	changes := map[string]map[string]string{
		"[ScalabilityGroups]":                         {"sg.ResolutionQuality": strconv.Itoa(p.Quality), "sg.ViewDistanceQuality": "0", "sg.AntiAliasingQuality": "0", "sg.ShadowQuality": "0", "sg.GlobalIlluminationQuality": "0", "sg.ReflectionQuality": "0", "sg.PostProcessQuality": "0", "sg.TextureQuality": "0", "sg.EffectsQuality": "0", "sg.FoliageQuality": "0", "sg.ShadingQuality": "0"},
		"[/Script/FortniteGame.FortGameUserSettings]": {"bUseVSync": "False", "bUseDynamicResolution": "False", "bMotionBlur": "False", "FrameRateLimit": fmt.Sprintf("%.6f", float64(p.FPS))},
	}
	if res[0] != 0 {
		for k, v := range map[string]int{"ResolutionSizeX": res[0], "ResolutionSizeY": res[1], "LastUserConfirmedResolutionSizeX": res[0], "LastUserConfirmedResolutionSizeY": res[1]} {
			changes["[/Script/FortniteGame.FortGameUserSettings]"][k] = strconv.Itoa(v)
		}
	}
	updated := updateINI(string(raw), changes)
	info, _ := os.Stat(filename)
	perm := os.FileMode(0644)
	if info != nil {
		perm = info.Mode().Perm()
	}
	if err = os.WriteFile(filename, []byte(updated), perm); err != nil {
		fail(w, 500, err)
		return
	}
	a.mu.Lock()
	a.state.LastFortniteBackup = backup
	saveErr := a.saveLocked()
	a.mu.Unlock()
	if saveErr != nil {
		fail(w, 500, saveErr)
		return
	}
	a.record("Fortnite Potato Graphics", fmt.Sprintf("Applied minimum scalability; 3D resolution %d%%; FPS limit %d; display resolution %s; backup %s", p.Quality, p.FPS, p.Resolution, backup), true)
	jsonOut(w, map[string]any{"ok": true, "message": "Low graphics profile applied. Start Fortnite and check settings; the game can override them.", "backup": backup, "file": filename})
}

// Ensure JSON dependency is exercised during compiler checks if this file is extracted.
var _ = json.Valid
