// Package netx is the HTTP client Boombrick's online features share: short
// timeouts, a user agent that names the app, and CA certificates compiled in
// for a device that has none of its own.
package netx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	// Used only when the system has no CA certificates.
	_ "golang.org/x/crypto/x509roots/fallback"

	"github.com/ujjwal313/boombrick/internal/version"
)

// UserAgent identifies Boombrick and where to find its authors, as
// MusicBrainz and LRCLIB ask of the apps that use them.
func UserAgent() string {
	return "Boombrick/" + version.Version + " (https://github.com/ujjwal313/boombrick)"
}

// NewClient returns a client that gives up connecting after 5 s and waiting
// for a response's headers after 10 s. Callers bound whole requests with
// their context.
func NewClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 5 * time.Second
	tr.ResponseHeaderTimeout = 10 * time.Second
	tr.MaxIdleConnsPerHost = 2
	tr.IdleConnTimeout = 30 * time.Second
	return &http.Client{Transport: tr}
}

// StatusError is a response other than 200 OK.
type StatusError struct {
	Code       int
	RetryAfter time.Duration // from Retry-After, when it gives seconds
}

func (e *StatusError) Error() string { return fmt.Sprintf("http status %d", e.Code) }

// IsNotFound reports whether err is a 404 response.
func IsNotFound(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == http.StatusNotFound
}

// ErrTooLarge is returned for a body longer than the caller allows.
var ErrTooLarge = errors.New("netx: response too large")

// Get fetches url and returns its body, which may be at most limit bytes.
// Any status but 200 is a *StatusError.
func Get(ctx context.Context, c *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return do(c, req, limit)
}

// Post sends body to url as contentType and returns the response's body, as
// Get does.
func Post(ctx context.Context, c *http.Client, url, contentType string, body []byte, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return do(c, req, limit)
}

// do sends req with Boombrick's user agent and reads the response.
func do(c *http.Client, req *http.Request, limit int64) ([]byte, error) {
	req.Header.Set("User-Agent", UserAgent())
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		e := &StatusError{Code: resp.StatusCode}
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
			e.RetryAfter = time.Duration(s) * time.Second
		}
		io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)) // so the connection can be reused
		return nil, e
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, ErrTooLarge
	}
	return body, nil
}
