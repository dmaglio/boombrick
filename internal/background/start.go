package background

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// readyEnv names the descriptor on which a background player says it plays.
const readyEnv = "BOOMBRICK_READY_FD"

// Start starts exe with args and env, detached in a session of its own so it
// outlives its starter, and waits up to wait for it to say it plays. One that
// doesn't by then, or dies first, is killed, and Start fails. A nil env is
// this process's own; env itself is never written to.
func Start(exe string, args, env []string, wait time.Duration) (int, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return 0, err
	}
	defer r.Close()
	cmd := exec.Command(exe, args...)
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = append(env[:len(env):len(env)], readyEnv+"=3")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	cmd.ExtraFiles = []*os.File{w} // descriptor 3
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	err = cmd.Start()
	w.Close() // the player's copy is the only one left: it closing, or dying, ends the read
	if err != nil {
		return 0, err
	}
	said := make(chan error, 1)
	go func() {
		b := make([]byte, 16)
		n, err := r.Read(b)
		switch {
		case strings.HasPrefix(string(b[:n]), "playing"):
			said <- nil
		case err != nil:
			said <- err
		default:
			said <- errors.New("background: an answer it doesn't know")
		}
	}()
	select {
	case err = <-said:
	case <-time.After(wait):
		err = errors.New("background: the player didn't say it plays in time")
	}
	if err != nil {
		cmd.Process.Kill()
		werr := cmd.Wait()
		if err == io.EOF {
			err = errors.New("background: the player quit before it played")
			if werr != nil {
				err = fmt.Errorf("%w: %v", err, werr)
			}
		}
		return 0, err
	}
	pid := cmd.Process.Pid
	// Nothing reaps the player while this process lives; Boombrick quits right
	// after starting one, and init reaps it then.
	cmd.Process.Release()
	return pid, nil
}

// Readier returns what tells the Boombrick that started this background
// player that the music plays: a func that says so on the descriptor Start
// passed it, once. Run by hand, with nothing waiting, the func does nothing.
// The descriptor is kept from programs the player starts, as the Spotify
// helper, so that only the player's own end can hold it open.
func Readier() func() {
	if os.Getenv(readyEnv) != "3" {
		return func() {}
	}
	os.Unsetenv(readyEnv)
	syscall.CloseOnExec(3)
	f := os.NewFile(3, "ready")
	var once sync.Once
	return func() {
		once.Do(func() {
			f.Write([]byte("playing\n"))
			f.Close()
		})
	}
}
