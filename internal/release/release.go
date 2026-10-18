// Package release finds a GitHub repository's latest release, for Settings to
// say when the downloader or Boombrick itself has a newer one.
package release

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ujjwal313/boombrick/internal/netx"
)

// API is GitHub's REST API.
const API = "https://api.github.com"

// Release is a GitHub release: its tag, and its files by name.
type Release struct {
	Tag    string
	Assets map[string]string // file name to download address
}

// Version is the tag without a leading "v".
func (r Release) Version() string { return strings.TrimPrefix(r.Tag, "v") }

// Latest asks GitHub for the latest release of repo, "owner/name". base is
// API, or a test server.
func Latest(ctx context.Context, c *http.Client, base, repo string) (Release, error) {
	body, err := netx.Get(ctx, c, base+"/repos/"+repo+"/releases/latest", 512<<10)
	if err != nil {
		return Release{}, err
	}
	var v struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return Release{}, fmt.Errorf("release of %s: %w", repo, err)
	}
	if v.Tag == "" {
		return Release{}, fmt.Errorf("release of %s: no tag", repo)
	}
	r := Release{Tag: v.Tag, Assets: map[string]string{}}
	for _, a := range v.Assets {
		r.Assets[a.Name] = a.URL
	}
	return r, nil
}

// Newer reports whether version a is newer than b, comparing their dotted
// numbers in turn: "0.10.0" is newer than "0.9.2", and "2026.10.11.1" than
// "2026.10.11". Anything is newer than a build without a version ("dev" or
// "").
func Newer(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	if _, err := strconv.Atoi(bs[0]); err != nil {
		_, aerr := strconv.Atoi(as[0])
		return aerr == nil
	}
	for i := range max(len(as), len(bs)) {
		x, y := part(as, i), part(bs, i)
		if x != y {
			return x > y
		}
	}
	return false
}

// part is the i-th number of a version, 0 past its end or where it isn't one.
func part(p []string, i int) int {
	if i >= len(p) {
		return 0
	}
	n, _ := strconv.Atoi(p[i])
	return n
}

// Checker keeps a repository's latest release for a screen that shows it:
// it asks GitHub when told to, at most every Every, and only online. The
// screen calls it from the UI thread and never waits; Ready receives a value
// when an answer is in.
type Checker struct {
	http   *http.Client
	base   string
	repo   string
	online func() bool
	every  time.Duration
	log    *slog.Logger
	ready  chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.Mutex
	latest Release
	have   bool
	next   time.Time // no asking before this
	asking bool
	closed bool
}

// retryFailed is how soon a check that failed may be made again.
const retryFailed = time.Minute

// NewChecker returns a checker for repo, through API.
func NewChecker(c *http.Client, repo string, online func() bool, every time.Duration, log *slog.Logger) *Checker {
	return newChecker(c, API, repo, online, every, log)
}

func newChecker(c *http.Client, base, repo string, online func() bool, every time.Duration, log *slog.Logger) *Checker {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Checker{http: c, base: base, repo: repo, online: online, every: every, log: log,
		ready: make(chan struct{}, 1), ctx: ctx, cancel: cancel}
}

// Check asks for the latest release unless one was asked for lately, or the
// device is offline. It returns at once; whether the device is online is
// read on the checker's goroutine.
func (c *Checker) Check() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.asking || time.Now().Before(c.next) {
		return
	}
	c.asking = true
	c.wg.Add(1)
	go c.ask()
}

func (c *Checker) ask() {
	defer c.wg.Done()
	if c.online != nil && !c.online() {
		c.mu.Lock()
		c.asking = false // asked again the next time
		c.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(c.ctx, 20*time.Second)
	defer cancel()
	r, err := Latest(ctx, c.http, c.base, c.repo)
	c.mu.Lock()
	c.asking = false
	if err != nil {
		c.next = time.Now().Add(retryFailed)
	} else {
		c.latest, c.have, c.next = r, true, time.Now().Add(c.every)
	}
	c.mu.Unlock()
	if err != nil {
		if c.ctx.Err() == nil {
			c.log.Warn("check for a release", "repo", c.repo, "err", err)
		}
		return
	}
	select {
	case c.ready <- struct{}{}:
	default:
	}
}

// Latest is the release last found, if one has been.
func (c *Checker) Latest() (Release, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest, c.have
}

// Ready receives a value when a release has been found. It is closed by
// Close.
func (c *Checker) Ready() <-chan struct{} { return c.ready }

// Close stops a check under way, waits for it and closes Ready.
func (c *Checker) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()
	c.cancel()
	c.wg.Wait()
	close(c.ready)
}
