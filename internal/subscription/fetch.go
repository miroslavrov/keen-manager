package subscription

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// DefaultUserAgent forces panels to return the base64 share-link list, which is
// the most robust format to parse into per-server Xray outbounds.
const DefaultUserAgent = "v2rayNG/1.9.5"

// userAgentSweep is tried in order. Many panels (3x-ui, Marzban, blancvpn, …)
// switch the response body format based on the client User-Agent: v2rayNG/Happ
// usually yield the base64 share-link list, clash clients yield Clash YAML, and
// a plain browser UA sometimes yields an HTML page. We prefer the base64 list,
// so v2rayNG-style agents come first and we only fall through on a body that
// parses to zero servers.
var userAgentSweep = []string{
	DefaultUserAgent,
	"Happ/1.6.0",
	"clash-verge/v1.7.7",
	"sing-box/1.9.0",
	"ClashForAndroid/2.5.12",
}

// maxAttemptsPerUA is how many times a single User-Agent is retried on a
// transient failure (network error or 5xx) before moving to the next agent.
const maxAttemptsPerUA = 2

// Result is the outcome of fetching and parsing a subscription.
type Result struct {
	Host     string
	Format   Format
	Servers  []model.Server
	UserInfo *model.SubUserInfo
	// UpdateIntervalHours from the profile-update-interval header, if present.
	UpdateIntervalHours int
}

// Fetch downloads a subscription URL and parses it, sweeping a set of client
// User-Agents and retrying transient failures until it obtains a body that
// parses to at least one server. Network only; no device state is modified.
func Fetch(ctx context.Context, rawURL string) (*Result, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("invalid subscription url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("subscription url must be http(s)")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		// Follow redirects (default policy allows up to 10); panels frequently
		// 302 to a CDN-hosted body.
	}

	var lastErr error
	// Remember the best non-empty parse across the sweep so a body that decodes
	// but yields zero servers (e.g. an expired sub) still surfaces a clear error
	// rather than a generic network failure.
	for _, ua := range userAgentSweep {
		for attempt := 1; attempt <= maxAttemptsPerUA; attempt++ {
			res, retriable, err := fetchOnce(ctx, client, u, ua)
			if err == nil {
				return res, nil
			}
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if !retriable {
				break // non-transient for this UA (e.g. parsed but empty) → next UA
			}
			// Exponential-ish backoff between retries of the same UA.
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 750 * time.Millisecond):
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("subscription produced no servers")
	}
	return nil, lastErr
}

// fetchOnce performs a single GET with the given User-Agent and parses the body.
// It returns (result, retriable, error). retriable is true when the caller
// should retry the same request (network error / 5xx); false means the request
// completed but is unusable for this UA (bad status, unparseable, or empty),
// in which case the caller should move on to the next UA.
func fetchOnce(ctx context.Context, client *http.Client, u *url.URL, ua string) (*Result, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("fetch: %w", err) // network error → retriable
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 5xx is transient; 4xx (auth/expired/not-found) is not.
		retriable := resp.StatusCode >= 500
		return nil, retriable, fmt.Errorf("subscription returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8 MiB cap
	if err != nil {
		return nil, true, fmt.Errorf("read body: %w", err)
	}

	servers, format, err := ParseBody(string(body))
	if err != nil {
		return nil, false, err // unparseable for this UA → try the next one
	}
	if len(servers) == 0 {
		return nil, false, fmt.Errorf("subscription parsed as %s but contained no servers", format)
	}

	return &Result{
		Host:                u.Hostname(),
		Format:              format,
		Servers:             servers,
		UserInfo:            parseUserInfo(resp.Header.Get("subscription-userinfo")),
		UpdateIntervalHours: hoursFromHeaders(resp.Header),
	}, false, nil
}

// parseUserInfo parses "upload=..; download=..; total=..; expire=.." headers.
func parseUserInfo(h string) *model.SubUserInfo {
	if h == "" {
		return nil
	}
	ui := &model.SubUserInfo{}
	found := false
	for _, part := range strings.Split(h, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		v, _ := strconv.ParseInt(strings.TrimSpace(kv[1]), 10, 64)
		switch strings.TrimSpace(kv[0]) {
		case "upload":
			ui.UploadBytes = v
			found = true
		case "download":
			ui.DownloadBytes = v
			found = true
		case "total":
			ui.TotalBytes = v
			found = true
		case "expire":
			if v > 0 {
				t := time.Unix(v, 0)
				ui.Expire = &t
			}
			found = true
		}
	}
	if !found {
		return nil
	}
	ui.UsedBytes = ui.UploadBytes + ui.DownloadBytes
	return ui
}

// hoursFromHeaders reads the update interval a panel advertises. Different
// panels use different headers/units:
//   - profile-update-interval: hours (common) — used as-is.
//   - profile-update-interval-seconds / interval: seconds — converted.
func hoursFromHeaders(h http.Header) int {
	if v := atoiSafe(h.Get("profile-update-interval")); v > 0 {
		return v
	}
	if v := atoiSafe(h.Get("profile-update-interval-seconds")); v > 0 {
		return maxInt(1, v/3600)
	}
	return 0
}

func atoiSafe(s string) int {
	i, _ := strconv.Atoi(strings.TrimSpace(s))
	return i
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
