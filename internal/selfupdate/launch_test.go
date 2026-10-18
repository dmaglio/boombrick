package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// The firmwares' launch.sh scripts are tested here, beside the code that
// stages what they apply.

// launchers are the firmwares' launch.sh scripts.
var launchers = []struct{ name, script string }{
	{"NextUI", filepath.Join("..", "..", "build", "pak", "launch.sh")},
	{"CrossMix", filepath.Join("..", "..", "build", "crossmix", "launch.sh")},
}

func write(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// installed is the app as a firmware holds it: script as its launch.sh, and
// a program that says which it is, with its folders.
func installed(t *testing.T, script string) string {
	t.Helper()
	body, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "launch.sh"), string(body), 0o755)
	write(t, filepath.Join(dir, "boombrick"), "#!/bin/sh\necho old > ran\n", 0o755)
	write(t, filepath.Join(dir, "bin", "qjs"), "old qjs", 0o755)
	write(t, filepath.Join(dir, "lib", "libold.so"), "old", 0o644)
	return dir
}

// launch runs the app's launch.sh as its firmware does, with stand-ins for
// the firmware's programs, and returns the folder it uses for /tmp.
func launch(t *testing.T, dir string) string {
	t.Helper()
	cmd, tmp := launcher(t, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("launch.sh: %v\n%s", err, out)
	}
	return tmp
}

// launcher is the command launch runs, and the folder it uses for /tmp. The
// stand-in poweroff, given by path as busybox's sh would run its own before
// one on PATH, leaves powered-off there.
func launcher(t *testing.T, dir string) (*exec.Cmd, string) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	stubs, tmp := t.TempDir(), t.TempDir()
	write(t, filepath.Join(stubs, "syncsettings.elf"), "#!/bin/sh\n", 0o755)
	write(t, filepath.Join(stubs, "poweroff"), "#!/bin/sh\ntouch \"$BOOMBRICK_TMP/powered-off\"\n", 0o755)
	cmd := exec.Command(sh, filepath.Join(dir, "launch.sh"))
	cmd.Env = append(os.Environ(), "LOGS_PATH="+t.TempDir(), "BOOMBRICK_TMP="+tmp,
		"BOOMBRICK_POWEROFF="+filepath.Join(stubs, "poweroff"),
		"PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	return cmd, tmp
}

func TestLaunchAppliesAStagedUpdate(t *testing.T) {
	for _, l := range launchers {
		t.Run(l.name, func(t *testing.T) {
			dir := installed(t, l.script)
			staged := filepath.Join(dir, ".update", "pak")
			write(t, filepath.Join(staged, "launch.sh"), read(t, filepath.Join(dir, "launch.sh")), 0o755)
			write(t, filepath.Join(staged, "boombrick"), "#!/bin/sh\necho new > ran\n", 0o755)
			write(t, filepath.Join(staged, "bin", "qjs"), "new qjs", 0o755)
			write(t, filepath.Join(staged, "lib", "libnew.so"), "new", 0o644)
			write(t, filepath.Join(dir, ".update", "ready"), "0.5.0\n", 0o644)
			launch(t, dir)
			if got := read(t, filepath.Join(dir, "ran")); got != "new\n" {
				t.Fatalf("the program that ran said %q", got)
			}
			if read(t, filepath.Join(dir, "bin", "qjs")) != "new qjs" || read(t, filepath.Join(dir, "lib", "libnew.so")) != "new" {
				t.Fatal("the new files weren't moved in")
			}
			if read(t, filepath.Join(dir, "lib", "libold.so")) != "old" {
				t.Fatal("a file the update doesn't have went")
			}
			if _, err := os.Stat(filepath.Join(dir, ".update")); !os.IsNotExist(err) {
				t.Fatal(".update is still there")
			}
		})
	}
}

func TestLaunchFinishesAnUpdateCutShort(t *testing.T) {
	for _, l := range launchers {
		t.Run(l.name, func(t *testing.T) {
			dir := installed(t, l.script)
			// The program was moved in before the power went; qjs wasn't.
			write(t, filepath.Join(dir, "boombrick"), "#!/bin/sh\necho new > ran\n", 0o755)
			write(t, filepath.Join(dir, ".update", "pak", "bin", "qjs"), "new qjs", 0o755)
			write(t, filepath.Join(dir, ".update", "ready"), "0.5.0\n", 0o644)
			launch(t, dir)
			if read(t, filepath.Join(dir, "ran")) != "new\n" || read(t, filepath.Join(dir, "bin", "qjs")) != "new qjs" {
				t.Fatal("the update wasn't finished")
			}
			if _, err := os.Stat(filepath.Join(dir, ".update")); !os.IsNotExist(err) {
				t.Fatal(".update is still there")
			}
		})
	}
}

func TestLaunchDeletesAnIncompleteStage(t *testing.T) {
	for _, l := range launchers {
		t.Run(l.name, func(t *testing.T) {
			dir := installed(t, l.script)
			write(t, filepath.Join(dir, ".update", "pak", "boombrick"), "#!/bin/sh\necho new > ran\n", 0o755)
			launch(t, dir)
			if got := read(t, filepath.Join(dir, "ran")); got != "old\n" {
				t.Fatalf("the program that ran said %q", got)
			}
			if _, err := os.Stat(filepath.Join(dir, ".update")); !os.IsNotExist(err) {
				t.Fatal("the incomplete stage is still there")
			}
		})
	}
}

// CrossMix's launcher names the firmware, keeps the Brick awake while
// Boombrick runs, and keeps the early output in the app's logs folder.
func TestCrossMixLaunch(t *testing.T) {
	dir := installed(t, launchers[1].script)
	write(t, filepath.Join(dir, "boombrick"), `#!/bin/sh
echo "$BOOMBRICK_PLATFORM" > ran
[ -f "$BOOMBRICK_TMP/stay_awake" ] && echo awake >> ran
echo starting
`, 0o755)
	tmp := launch(t, dir)
	if got := read(t, filepath.Join(dir, "ran")); got != "crossmix\nawake\n" {
		t.Fatalf("the program saw %q", got)
	}
	if _, err := os.Stat(filepath.Join(tmp, "stay_awake")); !os.IsNotExist(err) {
		t.Fatal("stay_awake outlived Boombrick")
	}
	if got := read(t, filepath.Join(dir, "logs", "Boombrick.crash.txt")); got != "starting\n" {
		t.Fatalf("the early output is %q", got)
	}
	if _, err := os.Stat(filepath.Join(tmp, "powered-off")); !os.IsNotExist(err) {
		t.Fatal("powered off though Boombrick didn't ask")
	}
}

// Boombrick on CrossMix asks to be powered off with a flag; the launcher
// powers off once it has exited.
func TestCrossMixPowersOffWhenAsked(t *testing.T) {
	dir := installed(t, launchers[1].script)
	write(t, filepath.Join(dir, "boombrick"), "#!/bin/sh\ntouch \"$BOOMBRICK_TMP/boombrick-poweroff\"\n", 0o755)
	tmp := launch(t, dir)
	if _, err := os.Stat(filepath.Join(tmp, "powered-off")); err != nil {
		t.Fatal("the launcher didn't power off")
	}
	if _, err := os.Stat(filepath.Join(tmp, "boombrick-poweroff")); !os.IsNotExist(err) {
		t.Fatal("the flag is still there")
	}
}

// A flag left by a launcher that was killed before it read it doesn't power
// off the Brick at the next ordinary quit.
func TestCrossMixIgnoresAnOldFlag(t *testing.T) {
	dir := installed(t, launchers[1].script)
	cmd, tmp := launcher(t, dir)
	write(t, filepath.Join(tmp, "boombrick-poweroff"), "", 0o644)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("launch.sh: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(tmp, "powered-off")); !os.IsNotExist(err) {
		t.Fatal("powered off for an old flag")
	}
}

// A SIGTERM to CrossMix's launcher, which outlives Boombrick, goes on to
// Boombrick, so it quits as it does when NextUI's launcher, replaced by it,
// is sent one: saving the queue.
func TestCrossMixPassesOnSIGTERM(t *testing.T) {
	dir := installed(t, launchers[1].script)
	write(t, filepath.Join(dir, "boombrick"), `#!/bin/sh
trap 'echo stopped > ran; exit 0' TERM
touch running
while :; do sleep 0.05; done
`, 0o755)
	cmd, tmp := launcher(t, dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(dir, "running")); err == nil {
			break
		}
	}
	cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("the launcher didn't end")
	}
	if got := read(t, filepath.Join(dir, "ran")); got != "stopped\n" {
		t.Fatalf("Boombrick saw %q, want the SIGTERM", got)
	}
	if _, err := os.Stat(filepath.Join(tmp, "stay_awake")); !os.IsNotExist(err) {
		t.Fatal("stay_awake outlived Boombrick")
	}
}
