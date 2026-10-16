package netx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetSendsTheUserAgentAndReadsTheBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "Boombrick/") || !strings.Contains(ua, "github.com/ujjwal313/boombrick") {
			t.Errorf("user agent %q", ua)
		}
		w.Write([]byte("hello"))
	}))
	defer srv.Close()
	body, err := Get(context.Background(), NewClient(), srv.URL, 100)
	if err != nil || string(body) != "hello" {
		t.Fatalf("Get = %q, %v", body, err)
	}
}

func TestGetFollowsRedirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/front", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/image", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/image", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "Boombrick/") {
			t.Error("the user agent was lost on the redirect")
		}
		w.Write([]byte("jpeg"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	if body, err := Get(context.Background(), NewClient(), srv.URL+"/front", 100); err != nil || string(body) != "jpeg" {
		t.Fatalf("Get = %q, %v", body, err)
	}
}

func TestGetReportsTheStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/busy" {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	_, err := Get(context.Background(), NewClient(), srv.URL+"/missing", 100)
	if !IsNotFound(err) {
		t.Fatalf("404 gave %v", err)
	}
	_, err = Get(context.Background(), NewClient(), srv.URL+"/busy", 100)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 503 || se.RetryAfter != 7*time.Second {
		t.Fatalf("503 gave %v", err)
	}
	if IsNotFound(err) {
		t.Fatal("503 counted as not found")
	}
}

func TestGetRefusesLargeBodies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, 101))
	}))
	defer srv.Close()
	if _, err := Get(context.Background(), NewClient(), srv.URL, 100); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("a 101-byte body with a 100-byte limit gave %v", err)
	}
}

func TestGetStopsAtTheDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := Get(ctx, NewClient(), srv.URL, 100); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Get past its deadline = %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("Get took %v to give up", time.Since(start))
	}
}
