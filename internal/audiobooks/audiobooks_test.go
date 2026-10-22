package audiobooks

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ujjwal313/boombrick/internal/audible"
)

type fakeAPI struct {
	books    []audible.Book
	err      error
	licErr   error
	licenses int
	body     string
}

func (f *fakeAPI) Library(context.Context) ([]audible.Book, error) { return f.books, f.err }
func (f *fakeAPI) License(_ context.Context, asin string) (audible.License, error) {
	f.licenses++
	if f.licErr != nil {
		return audible.License{}, f.licErr
	}
	return audible.License{ASIN: asin, URL: "http://x/" + asin, ACR: "CR!" + asin, Key: "aa", IV: "bb"}, nil
}
func (f *fakeAPI) Fetch(_ context.Context, _ audible.License, w io.Writer, progress func(done, total int64)) error {
	for i := 1; i <= 4; i++ {
		io.WriteString(w, f.body)
		progress(int64(i*len(f.body)), int64(4*len(f.body)))
	}
	return nil
}

var maigret = audible.Book{ASIN: "B0FH5CCLRJ", Title: "La chiusa n.1: parte/due", Authors: "Georges Simenon, Altri", Length: 4 * time.Hour}

func newService(t *testing.T, api *fakeAPI, mod func(*Options)) (*Service, string, chan string) {
	t.Helper()
	dir := t.TempDir()
	filed := make(chan string, 4)
	o := Options{API: api, Dir: filepath.Join(dir, "Audiobooks"), Cache: filepath.Join(dir, "lib.json"),
		Filed: func(p string) { filed <- p },
		Open: func(_ context.Context, in, out, key, iv string) error {
			b, err := os.ReadFile(in)
			if err != nil {
				return err
			}
			if key != "aa" || iv != "bb" {
				return errors.New("wrong key")
			}
			return os.WriteFile(out, []byte("M4B:"+string(b)), 0o644)
		}}
	if mod != nil {
		mod(&o)
	}
	s := New(o)
	t.Cleanup(s.Close)
	return s, dir, filed
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(2 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestWithoutAnAccountThereIsNothingToDo(t *testing.T) {
	s := New(Options{Dir: t.TempDir()})
	defer s.Close()
	if st, _ := s.Status(); st != NotSignedIn || s.SignedIn() || len(s.Entries()) != 0 {
		t.Fatal("a service with no API isn't empty")
	}
	s.Refresh()
	s.Download("B0FH5CCLRJ") // nothing happens, and nothing breaks
}

func TestRefreshListsAndKeepsTheLibrary(t *testing.T) {
	api := &fakeAPI{books: []audible.Book{maigret}}
	s, dir, _ := newService(t, api, nil)
	if st, _ := s.Status(); st != Loading {
		t.Fatalf("before the first fetch: %v", st)
	}
	s.Refresh()
	waitFor(t, "the list", func() bool { st, _ := s.Status(); return st == OK })
	if es := s.Entries(); len(es) != 1 || es[0].Title != maigret.Title || es[0].State != Idle || es[0].Path != "" {
		t.Fatalf("entries %+v", es)
	}
	// A new run shows the list kept, even without Wi-Fi.
	s2 := New(Options{API: &fakeAPI{err: errors.New("down")}, Dir: filepath.Join(dir, "Audiobooks"), Cache: filepath.Join(dir, "lib.json"),
		Online: func() bool { return false }})
	defer s2.Close()
	if st, _ := s2.Status(); st != OK || len(s2.Entries()) != 1 {
		t.Fatalf("kept list: %v, %d", st, len(s2.Entries()))
	}
	s2.Refresh()
	waitFor(t, "the offline refresh", func() bool { return true })
	if st, _ := s2.Status(); st != OK {
		t.Fatalf("going offline lost the list: %v", st)
	}
}

func TestOfflineWithNoListSaysSo(t *testing.T) {
	s := New(Options{API: &fakeAPI{}, Dir: t.TempDir(), Online: func() bool { return false }})
	defer s.Close()
	s.Refresh()
	waitFor(t, "offline", func() bool { st, _ := s.Status(); return st == Offline })
}

func TestABrokenFetchSaysWhy(t *testing.T) {
	s, _, _ := newService(t, &fakeAPI{err: errors.New("audible: GET /1.0/library: 401 Unauthorized")}, nil)
	s.Refresh()
	waitFor(t, "the failure", func() bool { st, msg := s.Status(); return st == Broken && strings.Contains(msg, "401") })
}

func TestDownloadFilesTheBookWithItsSidecar(t *testing.T) {
	api := &fakeAPI{books: []audible.Book{maigret}, body: "0123456789"}
	s, dir, filed := newService(t, api, nil)
	s.Refresh()
	waitFor(t, "the list", func() bool { st, _ := s.Status(); return st == OK })
	s.Download(maigret.ASIN)
	s.Download(maigret.ASIN) // asked twice: one download
	select {
	case path := <-filed:
		want := filepath.Join(dir, "Audiobooks", "Georges Simenon", "La chiusa n.1- parte-due [B0FH5CCLRJ].m4b")
		if path != want {
			t.Fatalf("filed at %q, want %q", path, want)
		}
		if b, _ := os.ReadFile(path); string(b) != "M4B:"+strings.Repeat("0123456789", 4) {
			t.Fatalf("the book holds %q", b)
		}
		if acr := audible.SidecarACR(path); acr != "CR!B0FH5CCLRJ" {
			t.Fatalf("sidecar acr %q", acr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("never filed")
	}
	if api.licenses != 1 {
		t.Fatalf("%d licences asked for, want 1", api.licenses)
	}
	waitFor(t, "the entry to show the file", func() bool {
		es := s.Entries()
		return len(es) == 1 && es[0].Path != "" && es[0].State == Idle
	})
	// Nothing is left but the book and its sidecar.
	files, _ := filepath.Glob(filepath.Join(dir, "Audiobooks", "*", ".*"))
	if len(files) != 0 {
		t.Fatalf("leftovers %v", files)
	}
	s.Download(maigret.ASIN) // already there
	time.Sleep(20 * time.Millisecond)
	if api.licenses != 1 {
		t.Fatal("downloaded a book already on the card")
	}
}

func TestADeniedDownloadFailsWithAReasonAndCanBeTriedAgain(t *testing.T) {
	api := &fakeAPI{books: []audible.Book{maigret}, body: "x", licErr: &audible.Denied{Reason: "Audible refused the download", Throttled: true}}
	s, _, filed := newService(t, api, nil)
	s.Refresh()
	waitFor(t, "the list", func() bool { st, _ := s.Status(); return st == OK })
	s.Download(maigret.ASIN)
	waitFor(t, "the failure", func() bool { es := s.Entries(); return es[0].State == Failed })
	if e := s.Entries()[0]; !strings.Contains(e.Err, "wait") {
		t.Fatalf("error %q", e.Err)
	}
	if len(filed) != 0 {
		t.Fatal("a denied book was filed")
	}
	api.licErr = nil
	s.Download(maigret.ASIN)
	select {
	case <-filed:
	case <-time.After(3 * time.Second):
		t.Fatal("never filed after the retry")
	}
}

func TestSafeNameAndPath(t *testing.T) {
	if got := SafeName(`A: b/c?d. `); got != "A- b-c-d" {
		t.Fatalf("%q", got)
	}
	if got := SafeName(`???`); got != "-" {
		t.Fatalf("%q", got)
	}
	if got := PathFor("/m/Audiobooks", audible.Book{ASIN: "B000000001", Title: "T"}); got != "/m/Audiobooks/Unknown/T [B000000001].m4b" {
		t.Fatalf("%q", got)
	}
	if audible.ASINOf(PathFor("/m", maigret)) != maigret.ASIN {
		t.Fatal("a filed book's ASIN can't be read back from its name")
	}
}

func TestExistingFilesAreRecognised(t *testing.T) {
	api := &fakeAPI{books: []audible.Book{maigret}}
	dir := t.TempDir()
	p := PathFor(filepath.Join(dir, "Audiobooks"), maigret)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("x"), 0o644)
	s := New(Options{API: api, Dir: filepath.Join(dir, "Audiobooks")})
	defer s.Close()
	s.Refresh()
	waitFor(t, "the list", func() bool { return len(s.Entries()) == 1 })
	if e := s.Entries()[0]; e.Path != p {
		t.Fatalf("path %q, want %q", e.Path, p)
	}
}

func TestFriendlyTellsPlanFromThrottle(t *testing.T) {
	plan := &audible.Denied{Reason: "x", Reasons: []string{"Customer is not part of any plans"}, Throttled: true}
	if got := friendly(plan); !strings.Contains(got, "no plan") {
		t.Errorf("a refusal for want of a plan, which also mentions throttling, says %q", got)
	}
	if got := friendly(&audible.Denied{Throttled: true}); !strings.Contains(got, "wait") {
		t.Errorf("a refusal for throttling alone says %q", got)
	}
	if got := friendly(&audible.Denied{Reasons: []string{"Something else"}}); got != "Something else" {
		t.Errorf("another reason says %q", got)
	}
}
