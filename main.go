package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed ui/* data/tweaks.json
var assets embed.FS

type RegChange struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}
type Tweak struct {
	ID          string      `json:"id"`
	Category    string      `json:"category"`
	Group       string      `json:"group"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Risk        string      `json:"risk"`
	Impact      string      `json:"impact"`
	Kind        string      `json:"kind"`
	Changes     []RegChange `json:"changes"`
	Package     string      `json:"package"`
	Explain     string      `json:"explain"`
	Admin       bool        `json:"admin"`
	Reboot      bool        `json:"reboot"`
}
type RegBackup struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Value  string `json:"value"`
	Exists bool   `json:"exists"`
}
type Activity struct {
	Time    string `json:"time"`
	Title   string `json:"title"`
	Detail  string `json:"detail"`
	Success bool   `json:"success"`
}
type State struct {
	Applied            map[string][]RegBackup `json:"applied"`
	StartupDisabled    map[string]RegBackup   `json:"startupDisabled"`
	Activities         []Activity             `json:"activities"`
	LastFortniteBackup string                 `json:"lastFortniteBackup"`
}
type Job struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Started  string `json:"started"`
	Finished bool   `json:"finished"`
	Error    string `json:"error,omitempty"`
	Output   string `json:"output"`
}
type App struct {
	mu        sync.Mutex
	state     State
	jobs      map[string]*Job
	catalog   []Tweak
	byID      map[string]Tweak
	stateFile string
	token     string
	admin     bool
	lastSeen  time.Time
}

func main() {
	// Keep diagnostics even when the executable runs without a console window.
	if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
		logDir := filepath.Join(dir, "VioletTweaks")
		if err := os.MkdirAll(logDir, 0700); err == nil {
			if f, err := os.OpenFile(filepath.Join(logDir, "startup.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600); err == nil {
				defer f.Close()
				log.SetOutput(f)
			}
		}
	}
	log.Println("Launching VioletTweaks 1.0.3")
	a := newApp()
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.page)
	mux.HandleFunc("/ui/", a.static)
	mux.HandleFunc("/api/init", a.api(a.init))
	mux.HandleFunc("/api/status", a.api(a.status))
	mux.HandleFunc("/api/toggle", a.api(a.toggle))
	mux.HandleFunc("/api/startup", a.api(a.startup))
	mux.HandleFunc("/api/processes", a.api(a.processes))
	mux.HandleFunc("/api/action", a.api(a.action))
	mux.HandleFunc("/api/jobs", a.api(a.jobList))
	mux.HandleFunc("/api/fortnite", a.api(a.fortnite))
	mux.HandleFunc("/api/booster", a.api(a.booster))
	mux.HandleFunc("/api/ping", a.api(a.ping))
	mux.HandleFunc("/api/restorepoints", a.api(a.restorepoints))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	url := "http://" + listener.Addr().String() + "/"
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 7 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Println(err)
		}
	}()
	log.Printf("VioletTweaks ready: %s", url)
	launchWindow(url)
	// Closing the browser eventually releases the hidden local process. Never exit during repairs.
	ticker := time.NewTicker(50 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		a.mu.Lock()
		idle := time.Since(a.lastSeen) > 6*time.Minute
		busy := false
		for _, j := range a.jobs {
			if !j.Finished {
				busy = true
				break
			}
		}
		a.mu.Unlock()
		if idle && !busy {
			_ = server.Shutdown(context.Background())
			return
		}
	}
}
func newApp() *App {
	b, err := assets.ReadFile("data/tweaks.json")
	if err != nil {
		log.Fatal(err)
	}
	a := &App{jobs: make(map[string]*Job), byID: make(map[string]Tweak), lastSeen: time.Now(), admin: isAdmin()}
	if err := json.Unmarshal(b, &a.catalog); err != nil {
		log.Fatal(err)
	}
	for _, v := range a.catalog {
		a.byID[v.ID] = v
	}
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		root, err = os.UserConfigDir()
		if err != nil {
			root = os.TempDir()
		}
	}
	dir := filepath.Join(root, "VioletTweaks")
	_ = os.MkdirAll(dir, 0700)
	a.stateFile = filepath.Join(dir, "state.json")
	a.state = State{Applied: map[string][]RegBackup{}, StartupDisabled: map[string]RegBackup{}, Activities: []Activity{}}
	if b, err := os.ReadFile(a.stateFile); err == nil {
		_ = json.Unmarshal(b, &a.state)
	}
	if a.state.Applied == nil {
		a.state.Applied = map[string][]RegBackup{}
	}
	if a.state.StartupDisabled == nil {
		a.state.StartupDisabled = map[string]RegBackup{}
	}
	if a.state.Activities == nil {
		a.state.Activities = []Activity{}
	}
	salt := make([]byte, 24)
	if _, err := rand.Read(salt); err != nil {
		log.Fatal(err)
	}
	a.token = hex.EncodeToString(salt)
	return a
}
func (a *App) saveLocked() error {
	b, err := json.MarshalIndent(a.state, "", "  ")
	if err != nil {
		return err
	}
	temp := a.stateFile + ".tmp"
	if err = os.WriteFile(temp, b, 0600); err != nil {
		return err
	}
	return os.Rename(temp, a.stateFile)
}
func (a *App) record(title, detail string, ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Activities = append([]Activity{{Time: time.Now().Format("2006-01-02 15:04:05"), Title: title, Detail: detail, Success: ok}}, a.state.Activities...)
	if len(a.state.Activities) > 120 {
		a.state.Activities = a.state.Activities[:120]
	}
	_ = a.saveLocked()
}
func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, err error) {
	w.WriteHeader(code)
	jsonOut(w, map[string]any{"ok": false, "error": err.Error()})
}
func parseJSON(r *http.Request, out any) error {
	if r.Method != "POST" {
		return errors.New("Use POST")
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 16384)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(out)
}
func (a *App) api(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Violet-Token") != a.token {
			fail(w, 403, errors.New("Unauthorized local request"))
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			fail(w, 403, errors.New("Unexpected origin"))
			return
		}
		if r.Method != "GET" && r.Method != "POST" {
			fail(w, 405, errors.New("Method not allowed"))
			return
		}
		a.mu.Lock()
		a.lastSeen = time.Now()
		a.mu.Unlock()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next(w, r)
	}
}
func (a *App) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, _ := assets.ReadFile("ui/index.html")
	html := strings.ReplaceAll(string(b), "__LOCAL_TOKEN__", a.token)
	// Embed only server-owned data; first UI render never waits on expensive hardware probes.
	a.mu.Lock()
	applied := make([]string, 0, len(a.state.Applied))
	for id := range a.state.Applied {
		applied = append(applied, id)
	}
	history := append([]Activity{}, a.state.Activities...)
	a.mu.Unlock()
	seed, _ := json.Marshal(map[string]any{"catalog": a.catalog, "applied": applied, "admin": a.admin, "windows": runtime.GOOS == "windows", "logs": history})
	html = strings.ReplaceAll(html, "__INITIAL_DATA__", string(seed))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	_, _ = io.WriteString(w, html)
}
func (a *App) static(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.URL.Path, "..") {
		http.NotFound(w, r)
		return
	}
	f, err := assets.Open(strings.TrimPrefix(r.URL.Path, "/"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	if strings.HasSuffix(r.URL.Path, ".js") {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	}
	if strings.HasSuffix(r.URL.Path, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, f)
}
func (a *App) init(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	applied := make([]string, 0, len(a.state.Applied))
	for k := range a.state.Applied {
		applied = append(applied, k)
	}
	events := append([]Activity{}, a.state.Activities...)
	backup := a.state.LastFortniteBackup
	a.mu.Unlock()
	jsonOut(w, map[string]any{"ok": true, "catalog": a.catalog, "applied": applied, "admin": a.admin, "windows": runtime.GOOS == "windows", "logs": events, "fortniteBackup": backup, "version": "1.0.3", "price": "FREE"})
}
func (a *App) status(w http.ResponseWriter, r *http.Request) {
	cpu, ramUsed, ramTotal := systemMetrics()
	gpuUsage, gpuTemp := nvidiaMetrics()
	a.mu.Lock()
	count := len(a.state.Applied)
	a.mu.Unlock()
	jsonOut(w, map[string]any{"ok": true, "cpu": cpu, "ramUsed": ramUsed, "ramTotal": ramTotal, "gpu": gpuUsage, "gpuTemp": gpuTemp, "applied": count, "admin": a.admin})
}
func runCmd(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if runtime.GOOS == "windows" {
		hideProcessWindow(cmd)
	}
	data, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(data), fmt.Errorf("Timed out after %s", timeout)
	}
	s := strings.TrimSpace(string(data))
	if len(s) > 20000 {
		s = s[len(s)-20000:]
	}
	return s, err
}
func isAdmin() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	_, err := runCmd(2*time.Second, "fltmc")
	return err == nil
}
func (a *App) toggle(w http.ResponseWriter, r *http.Request) {
	var p struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := parseJSON(r, &p); err != nil {
		fail(w, 400, err)
		return
	}
	t, ok := a.byID[p.ID]
	if !ok {
		fail(w, 404, errors.New("Unknown tweak"))
		return
	}
	if t.Kind != "registry" {
		a.record(t.Name, "PREVIEW — "+t.Explain, true)
		jsonOut(w, map[string]any{"ok": true, "preview": true, "message": t.Explain})
		return
	}
	if runtime.GOOS != "windows" {
		fail(w, 422, errors.New("Real tweaks require Windows"))
		return
	}
	if t.Admin && !a.admin {
		fail(w, 403, errors.New("Run VioletTweaks as administrator for this setting"))
		return
	}
	a.mu.Lock()
	backups, already := a.state.Applied[t.ID]
	a.mu.Unlock()
	if p.Enabled == already {
		jsonOut(w, map[string]any{"ok": true, "applied": already, "message": "Already in requested state"})
		return
	}
	if p.Enabled {
		backups = []RegBackup{}
		for _, c := range t.Changes {
			orig, _ := readReg(c.Key, c.Name)
			backups = append(backups, orig)
		}
		for i, c := range t.Changes {
			if err := writeReg(c); err != nil {
				for j := 0; j < i; j++ {
					_ = restoreReg(backups[j])
				}
				a.record(t.Name, "Could not apply: "+err.Error(), false)
				fail(w, 500, fmt.Errorf("Registry update failed: %w", err))
				return
			}
		}
		a.mu.Lock()
		a.state.Applied[t.ID] = backups
		err := a.saveLocked()
		a.mu.Unlock()
		if err != nil {
			fail(w, 500, err)
			return
		}
	} else {
		for _, c := range backups {
			if err := restoreReg(c); err != nil {
				a.record(t.Name, "Could not revert: "+err.Error(), false)
				fail(w, 500, fmt.Errorf("Could not revert: %w", err))
				return
			}
		}
		a.mu.Lock()
		delete(a.state.Applied, t.ID)
		err := a.saveLocked()
		a.mu.Unlock()
		if err != nil {
			fail(w, 500, err)
			return
		}
	}
	msg := "Applied"
	if !p.Enabled {
		msg = "Restored original setting"
	}
	if t.Reboot {
		msg += ". Restart required"
	}
	a.record(t.Name, msg, true)
	jsonOut(w, map[string]any{"ok": true, "applied": p.Enabled, "message": msg})
}

var regLine = regexp.MustCompile(`(?m)^\s*(.+?)\s{2,}(REG_(?:SZ|EXPAND_SZ|DWORD|QWORD|MULTI_SZ|BINARY))\s{2,}(.*?)\s*$`)

func readReg(key, name string) (RegBackup, error) {
	out, err := runCmd(5*time.Second, "reg", "query", key, "/v", name)
	b := RegBackup{Key: key, Name: name}
	if err != nil {
		return b, nil
	}
	matches := regLine.FindStringSubmatch(out)
	if len(matches) != 4 {
		return b, fmt.Errorf("Cannot parse registry query: %s", out)
	}
	b.Type = matches[2]
	b.Value = matches[3]
	b.Exists = true
	return b, nil
}
func writeReg(v RegChange) error {
	_, err := runCmd(5*time.Second, "reg", "add", v.Key, "/v", v.Name, "/t", v.Type, "/d", v.Value, "/f")
	return err
}
func restoreReg(b RegBackup) error {
	if !b.Exists {
		_, err := runCmd(5*time.Second, "reg", "delete", b.Key, "/v", b.Name, "/f")
		return err
	}
	return writeReg(RegChange{Key: b.Key, Name: b.Name, Type: b.Type, Value: b.Value})
}

type Process struct {
	PID  int     `json:"pid"`
	Name string  `json:"name"`
	MB   float64 `json:"mb"`
}

func fetchProcesses() ([]Process, error) {
	if runtime.GOOS != "windows" {
		return []Process{}, nil
	}
	ps := `@(Get-Process | Where-Object { $_.Id -gt 0 } | Sort-Object WorkingSet64 -Descending | Select-Object -First 26 @{N='pid';E={$_.Id}}, @{N='name';E={$_.ProcessName}}, @{N='mb';E={[math]::Round($_.WorkingSet64 / 1MB,1)}}) | ConvertTo-Json -Compress -Depth 3`
	out, err := runCmd(18*time.Second, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps)
	if err != nil {
		return nil, fmt.Errorf("Process query: %s %w", out, err)
	}
	var v []Process
	if err = json.Unmarshal([]byte(out), &v); err != nil {
		return nil, err
	}
	return v, nil
}
func (a *App) processes(w http.ResponseWriter, r *http.Request) {
	v, err := fetchProcesses()
	if err != nil {
		fail(w, 500, err)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "processes": v})
}

const startupKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

type StartupItem struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Enabled bool   `json:"enabled"`
}

func (a *App) readStartup() ([]StartupItem, error) {
	if runtime.GOOS != "windows" {
		return []StartupItem{}, nil
	}
	text, err := runCmd(8*time.Second, "reg", "query", startupKey)
	if err != nil && !strings.Contains(strings.ToLower(text), "unable to find") {
		return nil, err
	}
	items := []StartupItem{}
	for _, line := range strings.Split(text, "\n") {
		m := regLine.FindStringSubmatch(line)
		if len(m) == 4 {
			items = append(items, StartupItem{Name: strings.TrimSpace(m[1]), Command: strings.TrimSpace(m[3]), Enabled: true})
		}
	}
	a.mu.Lock()
	for k, v := range a.state.StartupDisabled {
		items = append(items, StartupItem{Name: k, Command: v.Value, Enabled: false})
	}
	a.mu.Unlock()
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	return items, nil
}
func (a *App) startup(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		v, err := a.readStartup()
		if err != nil {
			fail(w, 500, err)
			return
		}
		jsonOut(w, map[string]any{"ok": true, "items": v, "scope": "Current-user HKCU Run entries"})
		return
	}
	var p struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := parseJSON(r, &p); err != nil {
		fail(w, 400, err)
		return
	}
	if runtime.GOOS != "windows" {
		fail(w, 422, errors.New("Windows only"))
		return
	}
	if p.Name == "" || len(p.Name) > 180 {
		fail(w, 400, errors.New("Invalid entry name"))
		return
	}
	a.mu.Lock()
	saved, disabled := a.state.StartupDisabled[p.Name]
	a.mu.Unlock()
	if p.Enabled {
		if !disabled {
			fail(w, 400, errors.New("Entry not disabled by this app"))
			return
		}
		if err := restoreReg(saved); err != nil {
			fail(w, 500, err)
			return
		}
		a.mu.Lock()
		delete(a.state.StartupDisabled, p.Name)
		err := a.saveLocked()
		a.mu.Unlock()
		if err != nil {
			fail(w, 500, err)
			return
		}
	} else {
		if disabled {
			jsonOut(w, map[string]any{"ok": true})
			return
		}
		backup, err := readReg(startupKey, p.Name)
		if err != nil || !backup.Exists {
			fail(w, 400, errors.New("Startup entry not found"))
			return
		}
		if _, err := runCmd(6*time.Second, "reg", "delete", startupKey, "/v", p.Name, "/f"); err != nil {
			fail(w, 500, err)
			return
		}
		a.mu.Lock()
		a.state.StartupDisabled[p.Name] = backup
		err = a.saveLocked()
		a.mu.Unlock()
		if err != nil {
			fail(w, 500, err)
			return
		}
	}
	a.record("Startup: "+p.Name, map[bool]string{true: "Enabled", false: "Disabled"}[p.Enabled], true)
	jsonOut(w, map[string]any{"ok": true, "enabled": p.Enabled})
}

func (a *App) jobList(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	jobs := []Job{}
	for _, j := range a.jobs {
		jobs = append(jobs, *j)
	}
	a.mu.Unlock()
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Started > jobs[j].Started })
	jsonOut(w, map[string]any{"ok": true, "jobs": jobs})
}
func (a *App) startJob(name string, fn func() (string, error)) string {
	id := strconv.FormatInt(time.Now().UnixNano(), 36)
	j := &Job{ID: id, Name: name, Started: time.Now().Format("2006-01-02 15:04:05"), Output: "Running..."}
	a.mu.Lock()
	a.jobs[id] = j
	a.mu.Unlock()
	go func() {
		out, err := fn()
		a.mu.Lock()
		j.Output = out
		j.Finished = true
		if err != nil {
			j.Error = err.Error()
		}
		a.mu.Unlock()
		a.record(name, out, err == nil)
	}()
	return id
}
func (a *App) action(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Action string `json:"action"`
		ID     string `json:"id"`
	}
	if err := parseJSON(r, &p); err != nil {
		fail(w, 400, err)
		return
	}
	if runtime.GOOS != "windows" {
		fail(w, 422, errors.New("System operations require Windows"))
		return
	}
	if p.Action == "remove-app" {
		t, ok := a.byID[p.ID]
		if !ok || t.Kind != "appx" || t.Package == "" {
			fail(w, 400, errors.New("Unknown optional package"))
			return
		}
		pkg := t.Package // whitelist from embedded immutable catalog; never user-supplied code
		id := a.startJob(t.Name, func() (string, error) {
			cmd := `$ErrorActionPreference='Stop';$p=Get-AppxPackage -Name '` + pkg + `';if(-not $p){'Package not installed';exit 0};$p | Remove-AppxPackage -ErrorAction Stop;'Removed current-user package: ` + pkg + `'`
			return runCmd(90*time.Second, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", cmd)
		})
		jsonOut(w, map[string]any{"ok": true, "jobId": id, "message": "Removing optional package"})
		return
	}
	needsAdmin := map[string]bool{"sfc": true, "dism": true, "networkreset": true, "restore": true}
	if needsAdmin[p.Action] && !a.admin {
		fail(w, 403, errors.New("Run VioletTweaks as administrator to use this repair"))
		return
	}
	handlers := map[string]func() (string, error){
		"sfc": func() (string, error) { return runCmd(25*time.Minute, "sfc.exe", "/scannow") },
		"dism": func() (string, error) {
			return runCmd(30*time.Minute, "dism.exe", "/Online", "/Cleanup-Image", "/RestoreHealth")
		},
		"flushdns":     func() (string, error) { return runCmd(20*time.Second, "ipconfig.exe", "/flushdns") },
		"networkreset": func() (string, error) { return runCmd(25*time.Second, "netsh.exe", "winsock", "reset") },
		"restore": func() (string, error) {
			return runCmd(120*time.Second, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `Checkpoint-Computer -Description 'Before VioletTweaks changes' -RestorePointType MODIFY_SETTINGS -ErrorAction Stop; 'Restore point requested'`)
		},
		"cleartemp": clearOldTemp,
		"iconcache": func() (string, error) { return runCmd(12*time.Second, "ie4uinit.exe", "-show") },
		"chrome": func() (string, error) {
			return runCmd(8*time.Minute, "winget.exe", "install", "--id", "Google.Chrome", "-e", "--source", "winget", "--accept-package-agreements", "--accept-source-agreements")
		},
	}
	fn, ok := handlers[p.Action]
	if !ok {
		fail(w, 400, errors.New("Unknown repair"))
		return
	}
	labels := map[string]string{"sfc": "SFC system file check", "dism": "DISM component repair", "flushdns": "Flush DNS cache", "networkreset": "Reset Winsock stack (reboot required)", "restore": "Create system restore point", "cleartemp": "Clean old temp files", "iconcache": "Refresh icon cache", "chrome": "Install Google Chrome"}
	id := a.startJob(labels[p.Action], fn)
	jsonOut(w, map[string]any{"ok": true, "jobId": id, "message": "Started: " + labels[p.Action]})
}
func clearOldTemp() (string, error) {
	root := os.TempDir()
	count := 0
	var bytes int64
	deadline := time.Now().Add(-24 * time.Hour)
	// Keep directories and recent files, and never traverse symlinks or other paths.
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().After(deadline) {
			return nil
		}
		if err := os.Remove(path); err == nil {
			count++
			bytes += info.Size()
		}
		return nil
	})
	return fmt.Sprintf("Removed %d temporary files (older than 24 hours), reclaimed %.1f MB", count, float64(bytes)/(1024*1024)), nil
}

var protected = map[string]bool{"system": true, "idle": true, "registry": true, "svchost": true, "wininit": true, "services": true, "lsass": true, "csrss": true, "smss": true, "explorer": true, "dwm": true, "taskhostw": true, "winlogon": true, "fontdrvhost": true, "sihost": true, "shellexperiencehost": true, "startmenuexperiencehost": true, "searchhost": true, "securityhealthservice": true, "msmpeng": true, "runtimebroker": true, "audiodg": true, "fortniteclient-win64-shipping": true, "easyanticheat": true, "easyanticheat_eos": true, "beservice": true, "violettweaks": true, "powershell": true}

func (a *App) booster(w http.ResponseWriter, r *http.Request) {
	var p struct {
		PIDs []int `json:"pids"`
	}
	if err := parseJSON(r, &p); err != nil {
		fail(w, 400, err)
		return
	}
	if len(p.PIDs) > 20 {
		fail(w, 400, errors.New("Select a maximum of 20 apps"))
		return
	}
	current, err := fetchProcesses()
	if err != nil {
		fail(w, 500, err)
		return
	}
	byPid := map[int]Process{}
	for _, p := range current {
		byPid[p.PID] = p
	}
	out := []string{}
	killed := 0
	for _, pid := range p.PIDs {
		pr, ok := byPid[pid]
		if !ok || pid <= 4 || pid == os.Getpid() {
			continue
		}
		name := strings.ToLower(pr.Name)
		if protected[name] || strings.Contains(name, "anti-cheat") || strings.Contains(name, "anticheat") || strings.Contains(name, "fortnite") || strings.Contains(name, "defender") {
			out = append(out, pr.Name+": protected")
			continue
		}
		result, err := runCmd(8*time.Second, "taskkill.exe", "/PID", strconv.Itoa(pid), "/T")
		if err == nil {
			killed++
			out = append(out, pr.Name+": closed")
		} else {
			out = append(out, pr.Name+": "+result)
		}
	}
	message := fmt.Sprintf("Closed %d selected process(es). No automatic background termination is active. %s", killed, strings.Join(out, "; "))
	a.record("Game Booster", message, true)
	jsonOut(w, map[string]any{"ok": true, "closed": killed, "message": message})
}
func (a *App) ping(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS != "windows" {
		jsonOut(w, map[string]any{"ok": true, "latency": nil, "target": "1.1.1.1"})
		return
	}
	out, err := runCmd(4*time.Second, "ping.exe", "-n", "1", "-w", "1200", "1.1.1.1")
	var val any
	if err == nil {
		re := regexp.MustCompile(`(?i)(?:time|czas)\s*[=<]\s*(\d+)`)
		m := re.FindStringSubmatch(out)
		if len(m) == 2 {
			v, _ := strconv.Atoi(m[1])
			val = v
		}
	}
	jsonOut(w, map[string]any{"ok": true, "latency": val, "target": "1.1.1.1", "note": "Ping to Cloudflare DNS, not the Fortnite server"})
}

func (a *App) restorepoints(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS != "windows" {
		jsonOut(w, map[string]any{"ok": true, "items": []any{}, "note": "Windows only"})
		return
	}
	script := `$ErrorActionPreference='Stop';@(Get-ComputerRestorePoint | Sort-Object SequenceNumber -Descending | Select-Object -First 10 @{N='number';E={$_.SequenceNumber}},@{N='description';E={$_.Description}},@{N='created';E={$_.CreationTime}}) | ConvertTo-Json -Compress`
	out, err := runCmd(18*time.Second, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		jsonOut(w, map[string]any{"ok": true, "items": []any{}, "note": "Cannot read restore points: " + out})
		return
	}
	var items any
	if e := json.Unmarshal([]byte(out), &items); e != nil {
		items = []any{}
	}
	jsonOut(w, map[string]any{"ok": true, "items": items})
}
