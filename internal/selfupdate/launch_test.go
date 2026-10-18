package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The pak's launch.sh is tested here, beside the code that stages what it
// applies.

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

// installed is a pak as NextUI holds it: launch.sh and a program that says
// which it is, with its folders.
func installed(t *testing.T) string {
	t.Helper()
	script, err := os.ReadFile(filepath.Join("..", "..", "build", "pak", "launch.sh"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "launch.sh"), string(script), 0o755)
	write(t, filepath.Join(dir, "boombrick"), "#!/bin/sh\necho old > ran\n", 0o755)
	write(t, filepath.Join(dir, "bin", "qjs"), "old qjs", 0o755)
	write(t, filepath.Join(dir, "lib", "libold.so"), "old", 0o644)
	return dir
}

// launch runs the pak's launch.sh as NextUI does.
func launch(t *testing.T, dir string) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	stubs := t.TempDir()
	write(t, filepath.Join(stubs, "syncsettings.elf"), "#!/bin/sh\n", 0o755)
	cmd := exec.Command(sh, filepath.Join(dir, "launch.sh"))
	cmd.Env = append(os.Environ(), "LOGS_PATH="+t.TempDir(), "PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("launch.sh: %v\n%s", err, out)
	}
}

func TestLaunchAppliesAStagedUpdate(t *testing.T) {
	dir := installed(t)
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
}

func TestLaunchFinishesAnUpdateCutShort(t *testing.T) {
	dir := installed(t)
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
}

func TestLaunchDeletesAnIncompleteStage(t *testing.T) {
	dir := installed(t)
	write(t, filepath.Join(dir, ".update", "pak", "boombrick"), "#!/bin/sh\necho new > ran\n", 0o755)
	launch(t, dir)
	if got := read(t, filepath.Join(dir, "ran")); got != "old\n" {
		t.Fatalf("the program that ran said %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".update")); !os.IsNotExist(err) {
		t.Fatal("the incomplete stage is still there")
	}
}
