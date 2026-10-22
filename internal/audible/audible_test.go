package audible

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k, string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}))
}

// rewrite sends the client's requests to srv whatever the host.
type rewrite struct{ to string }

func (r rewrite) RoundTrip(q *http.Request) (*http.Response, error) {
	q = q.Clone(q.Context())
	q.URL.Scheme, q.URL.Host = "http", strings.TrimPrefix(r.to, "http://")
	return http.DefaultTransport.RoundTrip(q)
}

func TestSignatureCoversTheRequest(t *testing.T) {
	k, pemKey := testKey(t)
	c, err := New("ADP-TOKEN", pemKey, "it", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return time.Date(2026, 10, 22, 10, 24, 0, 123456000, time.UTC) }
	h, err := c.sign("PUT", "/1.0/lastpositions/B0GV1924QD", []byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	sig, date, _ := strings.Cut(h.Get("x-adp-signature"), ":")
	if date != "2026-10-22T10:24:00.123456+00:00Z" {
		t.Fatalf("date %q", date)
	}
	raw, _ := base64.StdEncoding.DecodeString(sig)
	sum := sha256.Sum256([]byte("PUT\n/1.0/lastpositions/B0GV1924QD\n" + date + "\n{\"a\":1}\nADP-TOKEN"))
	if err := rsa.VerifyPKCS1v15(&k.PublicKey, crypto.SHA256, sum[:], raw); err != nil {
		t.Fatalf("the signature doesn't verify: %v", err)
	}
	if h.Get("x-adp-token") != "ADP-TOKEN" || h.Get("x-adp-alg") != "SHA256withRSA:1.0" {
		t.Fatalf("headers %v", h)
	}
}

func TestPositionRoundTrip(t *testing.T) {
	_, pemKey := testKey(t)
	var put map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-adp-signature") == "" {
			http.Error(w, "unsigned", 401)
			return
		}
		switch r.Method {
		case "GET":
			if r.URL.Path != "/1.0/annotations/lastpositions" || r.URL.Query().Get("asins") != "B0GV1924QD" {
				http.Error(w, "bad", 400)
				return
			}
			io.WriteString(w, `{"asin_last_position_heard_annots":[{"asin":"B0GV1924QD","last_position_heard":{"last_updated":"2026-10-19 09:27:16.727","position_ms":93500,"status":"Exists"}}]}`)
		case "PUT":
			b, _ := io.ReadAll(r.Body)
			json.Unmarshal(b, &put)
			io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()
	c, err := New("T", pemKey, "it", &http.Client{Transport: rewrite{srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	p, ok, err := c.RemotePosition(context.Background(), "B0GV1924QD")
	if err != nil || !ok || p.At != 93500*time.Millisecond || p.Updated.Year() != 2026 || p.Updated.Day() != 19 {
		t.Fatalf("got %+v, %v, %v", p, ok, err)
	}
	if err := c.SetPosition(context.Background(), "B0GV1924QD", "CR!X", 2*time.Minute+500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if put["acr"] != "CR!X" || put["asin"] != "B0GV1924QD" || put["position_ms"] != float64(120500) {
		t.Fatalf("put %v", put)
	}
}

func TestNoPositionWhenAudibleHasNone(t *testing.T) {
	_, ok, err := parsePosition([]byte(`{"asin_last_position_heard_annots":[{"asin":"X","last_position_heard":{"status":"DoesNotExist"}}]}`), "X")
	if ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestLoad(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "none.json"), nil); err != ErrNoAuth {
		t.Fatalf("a missing file gave %v", err)
	}
	_, pemKey := testKey(t)
	path := filepath.Join(t.TempDir(), "auth.json")
	b, _ := json.Marshal(map[string]string{"adp_token": "T", "device_private_key": pemKey, "locale_code": "it"})
	os.WriteFile(path, b, 0o600)
	if c, err := Load(path, nil); err != nil || c.host != "api.audible.it" {
		t.Fatalf("got %+v, %v", c, err)
	}
	if _, err := New("T", pemKey, "xx", nil); err != ErrNoAuth {
		t.Fatalf("an unknown marketplace gave %v", err)
	}
}

func TestASINAndSidecar(t *testing.T) {
	if got := ASINOf("/m/Audiobooks/La casa del giudice [B0GV1924QD].m4b"); got != "B0GV1924QD" {
		t.Fatalf("ASIN %q", got)
	}
	if got := ASINOf("/m/Audiobooks/Libro senza.m4b"); got != "" {
		t.Fatalf("ASIN %q", got)
	}
	dir := t.TempDir()
	book := filepath.Join(dir, "Libro [B0GV1924QD].m4b")
	if SidecarACR(book) != "" {
		t.Fatal("a sidecar out of nowhere")
	}
	os.WriteFile(filepath.Join(dir, "Libro [B0GV1924QD].audible.json"), []byte(`{"asin":"B0GV1924QD","acr":"CR!ABC"}`), 0o644)
	if got := SidecarACR(book); got != "CR!ABC" {
		t.Fatalf("sidecar %q", got)
	}
}
