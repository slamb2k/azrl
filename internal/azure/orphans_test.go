package azure

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIsAzLoginCmdline(t *testing.T) {
	cases := []struct {
		argv []string
		want bool
	}{
		{[]string{"az", "login"}, true},
		{[]string{"az", "login", "--tenant", "x"}, true},
		{[]string{"/usr/bin/python3", "/usr/bin/az", "login", "--tenant", "x"}, true},
		{[]string{"/opt/az/bin/python3", "-Im", "azure.cli", "login"}, true},
		{[]string{"az", "logout"}, false},
		{[]string{"az", "account", "show"}, false},
		{[]string{"azrl", "login", "work"}, false},
		{[]string{"/home/u/.local/bin/azrl", "login"}, false},
		{[]string{"bash"}, false},
		{[]string{}, false},
		{[]string{"login", "az"}, false}, // login must FOLLOW the az token
	}
	for _, c := range cases {
		if got := isAzLoginCmdline(c.argv); got != c.want {
			t.Errorf("isAzLoginCmdline(%q) = %v, want %v", c.argv, got, c.want)
		}
	}
}

// writeProcEntry fabricates /proc/<pid>/{cmdline,status,stat} under root.
// startTicks == "" omits the stat file (age parsing must degrade to zero).
func writeProcEntry(t *testing.T, root string, pid int, argv []string, uid, ppid int, startTicks string) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmdline := strings.Join(argv, "\x00")
	if len(argv) > 0 {
		cmdline += "\x00"
	}
	if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmdline), 0o644); err != nil {
		t.Fatal(err)
	}
	status := fmt.Sprintf("Name:\taz\nState:\tS (sleeping)\nPPid:\t%d\nUid:\t%d\t%d\t%d\t%d\n", ppid, uid, uid, uid, uid)
	if err := os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}
	if startTicks != "" {
		// Field 22 (starttime) is the 20th field after the ")" — comm contains
		// a space to prove the parser anchors on the LAST ")".
		stat := fmt.Sprintf("%d (python3 az) S %d 1 1 0 -1 4194304 0 0 0 0 0 0 0 0 20 0 1 0 %s 0 0",
			pid, ppid, startTicks)
		if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestClassifyAzLogins(t *testing.T) {
	root := t.TempDir()
	// uptime 5000s; ticks are USER_HZ=100.
	if err := os.WriteFile(filepath.Join(root, "uptime"), []byte("5000.00 9000.00\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	uid := 1000
	writeProcEntry(t, root, 101, []string{"az", "login", "--tenant", "x"}, uid, 1, "380000")         // orphan, started 3800s → age 1200s
	writeProcEntry(t, root, 102, []string{"/usr/bin/python3", "/usr/bin/az", "login"}, uid, 500, "") // live (parent 500), no stat → age 0
	writeProcEntry(t, root, 103, []string{"az", "login"}, 2000, 1, "380000")                         // wrong uid → skipped
	writeProcEntry(t, root, 104, []string{"bash"}, uid, 1, "380000")                                 // not az login → skipped
	// Malformed entry: empty cmdline.
	writeProcEntry(t, root, 105, nil, uid, 1, "")
	// Non-numeric entry must be ignored.
	if err := os.MkdirAll(filepath.Join(root, "self"), 0o755); err != nil {
		t.Fatal(err)
	}

	orphans, live := classifyAzLogins(root, uid)
	if len(orphans) != 1 || orphans[0].PID != 101 {
		t.Fatalf("orphans = %+v, want exactly pid 101", orphans)
	}
	if orphans[0].Age != 1200*time.Second {
		t.Errorf("orphan age = %s, want 20m0s", orphans[0].Age)
	}
	if len(live) != 1 || live[0].PID != 102 {
		t.Fatalf("live = %+v, want exactly pid 102", live)
	}
	if live[0].Age != 0 {
		t.Errorf("live age = %s, want 0 (missing stat degrades to zero)", live[0].Age)
	}
}

func TestClassifyAzLoginsMissingRoot(t *testing.T) {
	orphans, live := classifyAzLogins(filepath.Join(t.TempDir(), "nope"), 1000)
	if orphans != nil || live != nil {
		t.Fatalf("expected nil/nil on missing proc root, got %+v / %+v", orphans, live)
	}
}

func TestFormatAge(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "age unknown"},
		{45 * time.Second, "45s"},
		{20 * time.Minute, "20m"},
		{90 * time.Minute, "1h30m"},
	}
	for _, c := range cases {
		if got := formatAge(c.d); got != c.want {
			t.Errorf("formatAge(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}
