// Package keenetic is a client for the Keenetic RCI (REST Core Interface,
// a.k.a. NDMS API) used to manage native AmneziaWG (AWG / AWG2) interfaces on
// KeeneticOS, in particular firmware 5.1.0+ which ships AWG2 support natively
// (no Entware/awg-quick required).
//
// # The NDMS-200-on-error quirk
//
// RCI almost always answers with HTTP 200, even when the request failed at
// the application level. A malformed interface name, a rejected ASC
// (obfuscation) parameter, or a "no such peer" all come back as a 200 whose
// JSON body carries an error envelope instead of (or alongside) the expected
// data. The envelope looks like:
//
//	{"status": [{"status": "error", "code": "...", "ident": "...", "message": "..."}]}
//
// and it is not limited to the top level: NDMS nests a "status" array under
// whichever sub-resource actually failed (e.g. under interface.<name>.wireguard
// when only the wireguard block was rejected), so callers must walk the whole
// response tree rather than checking a single well-known field. Client.Post
// does that walk and turns any "error" status entry into a Go error. We are
// deliberately lenient about what counts as an error (see isErrorEnvelope) so
// that ordinary "message"/"warning" status entries -- which are the normal,
// successful outcome of almost every RCI write -- are never misreported as
// failures.
//
// # AWG2 capability gate
//
// KeeneticOS only understands the AWG2 "asc" fields (s3, s4) starting with
// firmware 5.01.A.3 (see capabilities.go). Sending them to older firmware is
// rejected by RCI (again, as a 200-with-error-envelope). Every place in this
// package that could emit s3/s4 takes a Capabilities value and refuses to
// silently drop the fields; see SetASC in iface.go.
package keenetic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is the RCI endpoint reachable from the router itself (the
// loopback webadmin listener). It is used when New is called with an empty
// baseURL.
const DefaultBaseURL = "http://localhost:79/rci"

// defaultTimeout bounds every HTTP round trip made by Client. RCI calls are
// local (loopback) or LAN, so 30s is generous headroom rather than an
// expectation of slowness.
const defaultTimeout = 30 * time.Second

// Client is a small wrapper around http.Client that speaks the RCI JSON
// protocol: POST for actions/configuration changes, GET for "show" reads.
//
// Client is safe for concurrent use (http.Client is, and Client holds no
// other mutable state).
type Client struct {
	// BaseURL is the RCI root, e.g. "http://localhost:79/rci". It must not
	// have a trailing slash; Post and Get add the separators they need.
	BaseURL string

	HTTP *http.Client
}

// New returns a Client targeting baseURL. If baseURL is empty, DefaultBaseURL
// is used. The returned Client has a 30s per-request timeout; override
// c.HTTP.Timeout (or swap c.HTTP entirely) if a caller needs something else.
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: defaultTimeout},
	}
}

// Error is returned by Post/Get when RCI answers with an NDMS error
// envelope. It preserves the ident/code/message triple so callers can match
// on specific failures (e.g. "no such peer") without string-matching Error().
type Error struct {
	Status  string // "error" (Error is only constructed for error-level entries)
	Code    string
	Ident   string
	Message string
}

func (e *Error) Error() string {
	switch {
	case e.Ident != "" && e.Code != "":
		return fmt.Sprintf("rci: %s [%s/%s]", e.Message, e.Ident, e.Code)
	case e.Ident != "":
		return fmt.Sprintf("rci: %s [%s]", e.Message, e.Ident)
	default:
		return fmt.Sprintf("rci: %s", e.Message)
	}
}

// Post sends body (marshaled to JSON) to the RCI root ("{BaseURL}/") and
// returns the raw JSON response. Body is typically a map[string]any built ad
// hoc, e.g. map[string]any{"interface": map[string]any{...}}.
//
// Post treats a non-2xx HTTP status as a transport-level error. On a 2xx it
// additionally decodes the body and walks it for an NDMS error envelope (see
// the package doc comment); if one is found, the first error entry is
// returned as a *Error and the raw body is still available to the caller via
// errors.As for inspection if needed. When no error envelope is present, the
// raw JSON body is returned unchanged (message/warning status entries are not
// treated as errors).
func (c *Client) Post(ctx context.Context, body any) (json.RawMessage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("keenetic: marshal request: %w", err)
	}

	url := c.BaseURL + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("keenetic: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	raw, err := c.do(req)
	if err != nil {
		return nil, err
	}

	if rciErr := findErrorEnvelope(raw); rciErr != nil {
		return raw, rciErr
	}
	return raw, nil
}

// Get issues an RCI "show" read at "{BaseURL}{path}" and returns the raw JSON
// response, e.g. Get(ctx, "/show/version"). path must start with "/".
//
// Reads can carry error envelopes too (e.g. GET /show/interface/BadName0
// still answers 200 with a "no such interface" status), so Get applies the
// same envelope walk as Post.
func (c *Client) Get(ctx context.Context, path string) (json.RawMessage, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	url := c.BaseURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("keenetic: build request: %w", err)
	}

	raw, err := c.do(req)
	if err != nil {
		return nil, err
	}

	if rciErr := findErrorEnvelope(raw); rciErr != nil {
		return raw, rciErr
	}
	return raw, nil
}

// do performs the HTTP round trip shared by Post/Get: send, check the
// transport-level status code, and read the body into a json.RawMessage.
func (c *Client) do(req *http.Request) (json.RawMessage, error) {
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("keenetic: %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("keenetic: read response body: %w", err)
	}

	// RCI's application-level errors ride on HTTP 200 (see package doc); a
	// non-2xx here means something failed below the RCI layer itself (auth,
	// proxy, webadmin down, ...).
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("keenetic: %s %s: unexpected HTTP status %d: %s",
			req.Method, req.URL.Path, resp.StatusCode, truncate(string(data), 500))
	}

	if len(data) == 0 {
		// Some RCI actions (e.g. certain "no": true deletes) legitimately
		// return an empty body; treat it as "null" JSON rather than an error.
		return json.RawMessage("null"), nil
	}
	return json.RawMessage(data), nil
}

// Parse is the CLI-string escape hatch: it POSTs {"parse": cli} so a caller
// can issue a raw Keenetic CLI command when there is no structured RCI verb
// for it yet.
func (c *Client) Parse(ctx context.Context, cli string) (json.RawMessage, error) {
	return c.Post(ctx, map[string]any{"parse": cli})
}

// Save persists the running configuration to startup-config, equivalent to
// the CLI's "system configuration save". Callers should invoke this after a
// batch of interface/peer changes they want to survive a reboot.
func (c *Client) Save(ctx context.Context) error {
	_, err := c.Post(ctx, map[string]any{
		"system": map[string]any{
			"configuration": map[string]any{
				"save": map[string]any{},
			},
		},
	})
	return err
}

// truncate keeps error messages embedding raw HTTP bodies from becoming
// unbounded (webadmin error pages can be large HTML documents).
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// --- NDMS error-envelope detection -----------------------------------------

// statusItem mirrors one element of an NDMS "status" array.
type statusItem struct {
	Status  string `json:"status"`
	Code    string `json:"code"`
	Ident   string `json:"ident"`
	Message string `json:"message"`
}

// findErrorEnvelope walks an arbitrary RCI JSON response looking for an NDMS
// error. It recognizes two shapes, both of which RCI may nest at any depth
// (per top-level command, per sub-resource):
//
//  1. {"status": [ {"status": "error", ...}, ... ]} -- the documented,
//     structured status-array form used by almost every write endpoint.
//  2. {"error": "...", "message": "..."} / {"error": {"message": "..."}} --
//     an unstructured top-level error some endpoints fall back to.
//
// We are deliberately lenient: only entries whose own "status" field is
// exactly "error" (case-insensitive) trigger a failure. "message" and
// "warning" entries -- which is what a normal, successful RCI write returns
// -- are always left alone. This avoids the false-positive trap of treating
// every response that merely contains the word "status" as an error.
func findErrorEnvelope(raw json.RawMessage) error {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		// Not JSON at all (or empty/null); nothing to walk.
		return nil
	}
	return walkForError(v)
}

func walkForError(v any) error {
	switch t := v.(type) {
	case map[string]any:
		// Shape 1: a "status" array.
		if statusRaw, ok := t["status"]; ok {
			if items, ok := statusRaw.([]any); ok {
				if err := errorFromStatusArray(items); err != nil {
					return err
				}
			}
			// If "status" is a bare string (rare, top-level shorthand some
			// firmwares use for simple failures), treat "error"/"failed" as
			// an error ident with whatever "message" sits alongside it.
			if statusStr, ok := statusRaw.(string); ok && isErrorIdent(statusStr) {
				return &Error{
					Status:  statusStr,
					Code:    stringField(t, "code"),
					Ident:   stringField(t, "ident"),
					Message: firstNonEmpty(stringField(t, "message"), statusStr),
				}
			}
		}

		// Shape 2: an unstructured top-level error/message pair.
		if errRaw, ok := t["error"]; ok {
			switch e := errRaw.(type) {
			case string:
				if e != "" {
					return &Error{Message: firstNonEmpty(stringField(t, "message"), e)}
				}
			case map[string]any:
				msg := firstNonEmpty(stringField(e, "message"), stringField(t, "message"))
				if msg != "" {
					return &Error{
						Code:    stringField(e, "code"),
						Ident:   stringField(e, "ident"),
						Message: msg,
					}
				}
			}
		}

		// Recurse into every field; the envelope can be nested arbitrarily
		// deep under whichever sub-resource failed.
		for _, child := range t {
			if err := walkForError(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range t {
			if err := walkForError(child); err != nil {
				return err
			}
		}
	}
	return nil
}

// errorFromStatusArray scans one "status": [...] array (already type-asserted
// to []any) for the first entry whose "status" field indicates an error.
func errorFromStatusArray(items []any) error {
	for _, itemRaw := range items {
		obj, ok := itemRaw.(map[string]any)
		if !ok {
			continue
		}
		si := statusItem{
			Status:  stringField(obj, "status"),
			Code:    stringField(obj, "code"),
			Ident:   stringField(obj, "ident"),
			Message: stringField(obj, "message"),
		}
		if isErrorIdent(si.Status) {
			return &Error{Status: si.Status, Code: si.Code, Ident: si.Ident, Message: si.Message}
		}
	}
	return nil
}

// isErrorIdent reports whether an NDMS status-level string identifies a
// genuine error. We intentionally do not treat "warning" as fatal: RCI uses
// warnings for things like "interface already up", which are not failures a
// caller needs to act on.
func isErrorIdent(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "error")
}

func stringField(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
