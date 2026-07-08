package keenetic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// ImportResult holds the parsed outcome of a WireGuard/AmneziaWG config import.
type ImportResult struct {
	// Created is the NDMS interface name the router assigned, e.g. "Wireguard1".
	Created string
	// Intersects names a pre-existing interface the imported config collided
	// with (empty when none).
	Intersects string
	// Messages are the human-readable status[] lines the router returned.
	Messages []string
}

// ImportConfig uploads a wg-quick / AmneziaWG .conf to NDMS, which parses it
// natively and creates a native Wireguard interface, returning that interface's
// name. On firmware >= 5.01.A.3 NDMS understands the full AWG/AWG2 obfuscation
// set (jc, jmin, jmax, s1-s4, h1-h4, i1-i5) directly from the imported config.
//
// This is the robust native path: because NDMS itself validates and parses
// every field (private key, endpoint, AllowedIPs and all obfuscation params),
// keen-manager does not have to reproduce a piecemeal CreateInterface + SetASC
// + AddPeer sequence (which additionally has no way to convey the interface
// private key, the peer endpoint, or i1-i5). The payload/response shapes mirror
// what has been verified against real KeeneticOS 5.01.A.x devices:
//
//	request : {"interface":{"wireguard":{"import":<b64 conf>,"name":"","filename":"..."}}}
//	response: {"interface":{"wireguard":{"import":{"intersects":"","created":"Wireguard3","status":[...]}}}}
//
// confData is the raw .conf body (NOT base64 — it is encoded internally).
func ImportConfig(ctx context.Context, c *Client, confData []byte, filename string) (ImportResult, error) {
	encoded := base64.StdEncoding.EncodeToString(confData)
	resp, err := c.Post(ctx, map[string]any{
		"interface": map[string]any{
			"wireguard": map[string]any{
				"import":   encoded,
				"name":     "",
				"filename": filename,
			},
		},
	})
	if err != nil {
		return ImportResult{}, fmt.Errorf("keenetic: import wireguard: %w", err)
	}

	var parsed struct {
		Interface struct {
			Wireguard struct {
				Import struct {
					Intersects string `json:"intersects"`
					Created    string `json:"created"`
					Status     []struct {
						Status  string `json:"status"`
						Message string `json:"message"`
					} `json:"status"`
				} `json:"import"`
			} `json:"wireguard"`
		} `json:"interface"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return ImportResult{}, fmt.Errorf("keenetic: import wireguard: decode: %w", err)
	}
	imp := parsed.Interface.Wireguard.Import

	var msgs []string
	for _, s := range imp.Status {
		if s.Message != "" {
			msgs = append(msgs, s.Message)
		}
	}
	if imp.Created == "" {
		// HTTP 200 with no created interface: the reason lives in the nested
		// status[] array. Surface it rather than an opaque failure.
		detail := strings.Join(msgs, "; ")
		if detail == "" {
			detail = "no status message"
		}
		return ImportResult{}, fmt.Errorf("keenetic: import returned no created interface (intersects=%q; status: %s)", imp.Intersects, detail)
	}
	return ImportResult{Created: imp.Created, Intersects: imp.Intersects, Messages: msgs}, nil
}

// SetInterfaceGlobal marks an interface as a global (internet-access)
// connection so KeeneticOS includes it in connection-priority routing: when
// global is true the interface participates in default-route selection per the
// device's priority list; false removes it.
//
// This is intentionally best-effort — some firmware variants expect a numeric
// priority object rather than a boolean here, so callers should treat an error
// as non-fatal: the interface still exists and up, and can be prioritised from
// the Keenetic web UI. keen-manager's activation probe decides success either
// way, and tearing the interface down fully reverses this.
func SetInterfaceGlobal(ctx context.Context, c *Client, name string, global bool) error {
	_, err := c.Post(ctx, map[string]any{
		"interface": map[string]any{
			name: map[string]any{
				"ip": map[string]any{
					"global": global,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("keenetic: set interface %s global=%v: %w", name, global, err)
	}
	return nil
}
