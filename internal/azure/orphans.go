package azure

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// loginProc is one az-login process found during the orphan sweep.
type loginProc struct {
	PID int
	Age time.Duration
}

// isAzLoginCmdline reports whether argv is an interactive `az login`
// invocation. `az` is a launcher for `python -m azure.cli`, so the visible
// cmdline varies by install: match when some token is the az launcher (`az`,
// `*/az`, or contains `azure.cli`) and a LATER token is exactly `login`.
// Rejects `az logout`, `azrl login` (azrl itself), and unrelated pythons.
func isAzLoginCmdline(argv []string) bool {
	azAt := -1
	for i, a := range argv {
		if a == "az" || strings.HasSuffix(a, "/az") || strings.Contains(a, "azure.cli") {
			azAt = i
			break
		}
	}
	if azAt < 0 {
		return false
	}
	for _, a := range argv[azAt+1:] {
		if a == "login" {
			return true
		}
	}
	return false
}

// classifyAzLogins walks the numeric entries of procRoot (production: /proc)
// and splits the given user's az-login processes into orphans — parent dead,
// reparented to PID 1 — and live ones. Malformed or unreadable entries are
// skipped; a missing procRoot (non-Linux) yields nothing.
func classifyAzLogins(procRoot string, uid int) (orphans, live []loginProc) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, nil
	}
	uptime := readUptime(procRoot)
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(procRoot, e.Name(), "cmdline"))
		if err != nil || len(raw) == 0 {
			continue
		}
		argv := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		if !isAzLoginCmdline(argv) {
			continue
		}
		puid, ppid, ok := readStatus(filepath.Join(procRoot, e.Name(), "status"))
		if !ok || puid != uid {
			continue
		}
		p := loginProc{PID: pid, Age: procAge(filepath.Join(procRoot, e.Name(), "stat"), uptime)}
		if ppid == 1 {
			orphans = append(orphans, p)
		} else {
			live = append(live, p)
		}
	}
	return orphans, live
}

// readStatus extracts the real UID and PPid from a /proc/<pid>/status file.
func readStatus(path string) (uid, ppid int, ok bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, false
	}
	uid, ppid = -1, -1
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "Uid:":
			uid, _ = strconv.Atoi(f[1])
		case "PPid:":
			ppid, _ = strconv.Atoi(f[1])
		}
	}
	return uid, ppid, uid >= 0 && ppid >= 0
}

// readUptime returns the system uptime in seconds, zero on any failure.
func readUptime(procRoot string) float64 {
	raw, err := os.ReadFile(filepath.Join(procRoot, "uptime"))
	if err != nil {
		return 0
	}
	f := strings.Fields(string(raw))
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64)
	return v
}

// procAge derives a process age from /proc/<pid>/stat field 22 (starttime, in
// clock ticks since boot) and the system uptime. The comm field (2) may
// contain spaces, so parsing anchors on the LAST ')'. Zero on any failure.
// ponytail: USER_HZ hardcoded to 100 (the Linux default); age is display-only.
func procAge(statPath string, uptime float64) time.Duration {
	raw, err := os.ReadFile(statPath)
	if err != nil || uptime == 0 {
		return 0
	}
	i := strings.LastIndexByte(string(raw), ')')
	if i < 0 {
		return 0
	}
	f := strings.Fields(string(raw)[i+1:])
	if len(f) < 20 {
		return 0
	}
	ticks, err := strconv.ParseFloat(f[19], 64)
	if err != nil {
		return 0
	}
	age := uptime - ticks/100
	if age < 0 {
		return 0
	}
	return time.Duration(age * float64(time.Second))
}

// formatAge renders a process age for the live-login warning line.
func formatAge(d time.Duration) string {
	switch {
	case d <= 0:
		return "age unknown"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// ProcFS and killProc are package seams so tests can redirect the sweep away
// from the real /proc and real signals. ProcFS is exported so cmd-package
// tests exercising the login path (which reaches CleanSlate -> the sweep) can
// point it at an empty temp dir instead of the real /proc. killProc uses
// os.Process.Signal (NOT syscall.Kill) so the package still compiles for
// windows/darwin release builds; on hosts without /proc it is never reached.
var (
	ProcFS   = "/proc"
	killProc = func(pid int) error {
		p, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		return p.Signal(syscall.SIGTERM)
	}
)

// SweepOrphanedLogins reaps the current user's orphaned az-login processes —
// parent dead, reparented to PID 1, typically leftovers of manual `az login`
// runs whose terminal died — and warns about live ones, which can steal the
// next login's browser callback. Best-effort: without /proc (macOS, Windows)
// and on any read or kill error it is a silent no-op, never blocking a login.
func SweepOrphanedLogins(out io.Writer) {
	orphans, live := classifyAzLogins(ProcFS, os.Getuid())
	for _, p := range orphans {
		if killProc(p.PID) == nil {
			fmt.Fprintf(out, "azrl: reaped orphaned az login (pid %d)\n", p.PID)
		}
	}
	for _, p := range live {
		fmt.Fprintf(out, "azrl: note: another az login is running (pid %d, %s) — it may steal the browser callback\n", p.PID, formatAge(p.Age))
	}
}
