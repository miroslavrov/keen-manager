package subscription

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const fetchTestLink = "vless://839d4028-2984-4e66-8e62-f4c127b52f49@109.163.239.98:443?security=reality&encryption=none&type=tcp&flow=xtls-rprx-vision&sni=cdn3-87.yahoo.com&pbk=CMkW1axrhEXoiJ6anMz9XEjlfqlAtEZya7L0b5ZPMyw&sid=07ddc43269d197c0#NL Amsterdam"

// A v2rayNG-style client gets the base64 list; other UAs get an empty body so
// the test asserts we settle on the agent that actually yields servers.
func TestFetchUASweep(t *testing.T) {
	body := base64.StdEncoding.EncodeToString([]byte(fetchTestLink))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.UserAgent(), "v2rayNG") {
			w.Header().Set("subscription-userinfo", "upload=10; download=20; total=100; expire=0")
			w.Header().Set("profile-update-interval", "12")
			_, _ = w.Write([]byte(body))
			return
		}
		_, _ = w.Write([]byte("")) // empty for other agents
	}))
	defer srv.Close()

	res, err := Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if res.Format != FormatBase64List {
		t.Errorf("format = %s, want base64-list", res.Format)
	}
	if len(res.Servers) != 1 {
		t.Fatalf("got %d servers, want 1", len(res.Servers))
	}
	if res.UserInfo == nil || res.UserInfo.TotalBytes != 100 || res.UserInfo.UsedBytes != 30 {
		t.Errorf("userinfo = %+v", res.UserInfo)
	}
	if res.UpdateIntervalHours != 12 {
		t.Errorf("interval = %d, want 12", res.UpdateIntervalHours)
	}
}

// A 503 must be retried (transient) and then succeed once the server recovers.
func TestFetchRetriesOn5xx(t *testing.T) {
	var hits int32
	body := base64.StdEncoding.EncodeToString([]byte(fetchTestLink))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	res, err := Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(res.Servers) != 1 {
		t.Fatalf("got %d servers, want 1", len(res.Servers))
	}
	if atomic.LoadInt32(&hits) < 2 {
		t.Errorf("expected a retry after 503, hits=%d", hits)
	}
}

// A 403 (auth/expired) is NOT retried on the same UA; the sweep moves on and,
// finding no usable body, returns an error rather than hanging.
func TestFetchNonRetriable4xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	if _, err := Fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("expected error for a 403 subscription")
	}
	// One hit per UA in the sweep, no per-UA retries (403 is non-retriable).
	if got := int(atomic.LoadInt32(&hits)); got != len(userAgentSweep) {
		t.Errorf("hits = %d, want %d (one per UA, no retries)", got, len(userAgentSweep))
	}
}
