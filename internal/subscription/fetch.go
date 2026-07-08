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

// Result is the outcome of fetching and parsing a subscription.
type Result struct {
	Host     string
	Format   Format
	Servers  []model.Server
	UserInfo *model.SubUserInfo
	// UpdateIntervalHours from the profile-update-interval header, if present.
	UpdateIntervalHours int
}

// Fetch downloads a subscription URL and parses it. Network only; no device
// state is modified.
func Fetch(ctx context.Context, rawURL string) (*Result, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("invalid subscription url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("subscription url must be http(s)")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Accept", "*/*")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subscription returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8 MiB cap
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	servers, format, err := ParseBody(string(body))
	if err != nil {
		return nil, err
	}

	res := &Result{
		Host:                u.Hostname(),
		Format:              format,
		Servers:             servers,
		UserInfo:            parseUserInfo(resp.Header.Get("subscription-userinfo")),
		UpdateIntervalHours: atoiSafe(resp.Header.Get("profile-update-interval")),
	}
	return res, nil
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

func atoiSafe(s string) int {
	i, _ := strconv.Atoi(strings.TrimSpace(s))
	return i
}
