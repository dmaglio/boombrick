package download

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/ujjwal313/boombrick/internal/netx"
)

// Installer keeps yt-dlp's unpacked build in Dir/yt-dlp: the program and its
// _internal folder, as the release's zip holds them, with the zip's SHA-256
// in Dir/yt-dlp/SHA256. The unpacked build starts faster than the one-file
// build, which unpacks itself on every run.
type Installer struct {
	HTTP  *http.Client
	Dir   string // bin/
	Asset string // the release's zip
	Base  string // where release files are fetched from
}

// NewInstaller returns an installer for this machine's build of yt-dlp.
func NewInstaller(c *http.Client, dir string) *Installer {
	asset := "yt-dlp_linux.zip"
	if runtime.GOARCH == "arm64" {
		asset = "yt-dlp_linux_aarch64.zip"
	}
	return &Installer{HTTP: c, Dir: dir, Asset: asset, Base: "https://github.com/yt-dlp/yt-dlp/releases/latest/download/"}
}

// Program is the installed yt-dlp, or "" when there is none.
func (in *Installer) Program() string {
	p := filepath.Join(in.Dir, "yt-dlp", strings.TrimSuffix(in.Asset, ".zip"))
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// The limits on a download: the zip is about 40 MB and unpacks to about
// 92 MB in a few hundred files.
var (
	maxZip      int64 = 200 << 20
	maxUnpacked int64 = 400 << 20
	maxEntries        = 10000
)

// Install fetches the latest release unless it is the one installed,
// reporting progress as the zip comes, and reports whether it installed
// anything. It must not run twice at once, nor while yt-dlp is running; the
// service calls it only from its scheduler.
func (in *Installer) Install(ctx context.Context, progress func(done, total int64)) (bool, error) {
	sums, err := netx.Get(ctx, in.HTTP, in.Base+"SHA2-256SUMS", 64<<10)
	if err != nil {
		return false, fmt.Errorf("yt-dlp checksums: %w", err)
	}
	want, ok := sumFor(sums, in.Asset)
	if !ok {
		return false, fmt.Errorf("yt-dlp checksums: no %s", in.Asset)
	}
	cur := filepath.Join(in.Dir, "yt-dlp")
	fresh, old := filepath.Join(in.Dir, "yt-dlp.new"), filepath.Join(in.Dir, "yt-dlp.old")
	zipPath := filepath.Join(in.Dir, "yt-dlp.zip.part")
	if have, _ := os.ReadFile(filepath.Join(cur, "SHA256")); in.Program() != "" && strings.TrimSpace(string(have)) == want {
		// What an interrupted update left behind is of no use now.
		os.RemoveAll(old)
		os.RemoveAll(fresh)
		os.Remove(zipPath)
		return false, nil
	}
	if err := os.MkdirAll(in.Dir, 0o755); err != nil {
		return false, err
	}
	defer os.Remove(zipPath)
	if err := in.fetch(ctx, zipPath, want, progress); err != nil {
		return false, err
	}
	// A .new from a crashed run is never trusted, and a .old with no folder
	// beside it is replaced by this download.
	if err := os.RemoveAll(fresh); err != nil {
		return false, err
	}
	if err := unzip(ctx, zipPath, fresh); err != nil {
		os.RemoveAll(fresh)
		return false, err
	}
	prog := filepath.Join(fresh, strings.TrimSuffix(in.Asset, ".zip"))
	if _, err := os.Stat(prog); err != nil {
		os.RemoveAll(fresh)
		return false, fmt.Errorf("yt-dlp: %s holds no program", in.Asset)
	}
	if err := errors.Join(os.Chmod(prog, 0o755), os.WriteFile(filepath.Join(fresh, "SHA256"), []byte(want+"\n"), 0o644)); err != nil {
		os.RemoveAll(fresh)
		return false, err
	}
	// FAT and exFAT can lose file data to a power cut after the renames; the
	// SHA256 file must not outlive the program it vouches for.
	syscall.Sync()
	// The old folder is moved aside before the new one takes its place, so a
	// crash leaves one install or none, never half of one.
	if err := os.RemoveAll(old); err != nil {
		os.RemoveAll(fresh)
		return false, err
	}
	if err := os.Rename(cur, old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		os.RemoveAll(fresh)
		return false, err
	}
	if err := os.Rename(fresh, cur); err != nil {
		os.Rename(old, cur)
		os.RemoveAll(fresh)
		return false, err
	}
	os.RemoveAll(old)
	return true, nil
}

// sumFor finds name's SHA-256 in a SHA2-256SUMS file.
func sumFor(sums []byte, name string) (string, bool) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return strings.ToLower(f[0]), true
		}
	}
	return "", false
}

// fetch streams the zip to path, checking it against want as it comes.
func (in *Installer) fetch(ctx context.Context, path, want string, progress func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.Base+in.Asset, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", netx.UserAgent())
	resp, err := in.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &netx.StatusError{Code: resp.StatusCode}
	}
	if resp.ContentLength > maxZip {
		return errors.New("yt-dlp: the download is too large")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	pw := &progressWriter{total: resp.ContentLength, report: progress}
	n, err := io.Copy(io.MultiWriter(f, h, pw), io.LimitReader(resp.Body, maxZip+1))
	if err != nil {
		return err
	}
	if n > maxZip {
		return errors.New("yt-dlp: the download is too large")
	}
	pw.flush()
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("yt-dlp: the download's checksum is %s, not %s", got, want)
	}
	return f.Close()
}

// progressWriter reports bytes as they pass, every 256 KB and at the end.
type progressWriter struct {
	done, total, told int64
	report            func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.done-p.told >= 256<<10 {
		p.flush()
	}
	return len(b), nil
}

func (p *progressWriter) flush() {
	if p.report != nil {
		p.report(p.done, p.total)
	}
	p.told = p.done
}

// unzip unpacks the zip at src into the folder dst, refusing entries that
// would land outside it, that are neither folders nor plain files, or that
// make it larger than a yt-dlp build can be.
func unzip(ctx context.Context, src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	if len(r.File) > maxEntries {
		return fmt.Errorf("yt-dlp: the zip holds %d entries", len(r.File))
	}
	left := maxUnpacked
	for _, f := range r.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := filepath.FromSlash(f.Name)
		if !filepath.IsLocal(name) {
			return fmt.Errorf("yt-dlp: the zip holds %q, outside its folder", f.Name)
		}
		p := filepath.Join(dst, name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
			continue
		}
		if f.Mode()&fs.ModeType != 0 {
			return fmt.Errorf("yt-dlp: the zip holds %q, which is not a plain file", f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		n, err := unzipFile(f, p, left)
		if err != nil {
			return err
		}
		left -= n
	}
	return nil
}

// unzipFile writes f to path, at most limit bytes, and returns how many.
func unzipFile(f *zip.File, path string, limit int64) (int64, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode().Perm()|0o600)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, io.LimitReader(rc, limit+1))
	if err == nil && n > limit {
		err = errors.New("yt-dlp: the zip unpacks to too much")
	}
	if err != nil {
		out.Close()
		return n, err
	}
	return n, out.Close()
}
