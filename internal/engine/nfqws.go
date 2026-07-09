package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/health"
	"github.com/miroslavrov/keen-manager/internal/model"
	"github.com/miroslavrov/keen-manager/internal/nfqws"
)

// Nfqws returns the nfqws2 service status.
func (e *Engine) Nfqws() model.NfqwsStatusView { return e.nfqws.Status() }

// NfqwsAction drives the nfqws2 service (start/stop/restart/reload/install).
func (e *Engine) NfqwsAction(action string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	var err error
	switch action {
	case "install":
		err = e.nfqws.Install()
	case "start", "stop", "restart", "reload":
		err = e.nfqws.Action(action)
	default:
		return fmt.Errorf("unknown nfqws action %q", action)
	}
	if err != nil {
		return err
	}
	e.Logf("nfqws2: %s", action)
	e.publishState()
	return nil
}

// NfqwsConfig returns the raw nfqws2.conf plus the detected mode.
func (e *Engine) NfqwsConfig() (NfqwsConfigView, error) {
	raw, err := e.nfqws.ReadConfigRaw()
	if err != nil {
		return NfqwsConfigView{}, err
	}
	mode, _ := e.nfqws.Mode()
	return NfqwsConfigView{Raw: raw, Mode: mode}, nil
}

// SaveNfqwsConfig writes a new nfqws2.conf and/or sets the mode macro, then
// reloads the service. Empty raw leaves the file untouched; empty mode leaves
// the mode untouched.
func (e *Engine) SaveNfqwsConfig(raw string, mode model.NfqwsMode) error {
	if strings.TrimSpace(raw) != "" {
		if err := e.nfqws.WriteConfigRaw(raw); err != nil {
			return err
		}
	}
	if mode != "" {
		if err := e.nfqws.SetModeOnDisk(mode); err != nil {
			return err
		}
	}
	// Reload is best-effort: config is saved regardless of the service state.
	if e.nfqws.Running() {
		_ = e.nfqws.Reload()
	}
	e.Logf("nfqws2 config saved (mode=%s)", mode)
	e.publishState()
	return nil
}

// NfqwsConfigStructured returns nfqws2.conf parsed into typed fields for the
// form editor (ports, interface, policy, mode, strategy blocks, …).
func (e *Engine) NfqwsConfigStructured() (nfqws.Conf, error) {
	return e.nfqws.Conf()
}

// SaveNfqwsConfigStructured merges a partial set of typed fields over the
// current nfqws2.conf and writes it back with lossless round-trip (only changed
// keys are rewritten; comments, ordering and untouched multiline strategy
// blocks are preserved byte-for-byte), then reloads the service if running.
func (e *Engine) SaveNfqwsConfigStructured(fields map[string]any) error {
	cur, err := e.nfqws.Conf()
	if err != nil {
		return err
	}
	// Overlay the incoming fields onto the current typed config via JSON so the
	// caller can send only the keys they changed.
	base, err := json.Marshal(cur)
	if err != nil {
		return err
	}
	var merged map[string]any
	if err := json.Unmarshal(base, &merged); err != nil {
		return err
	}
	for k, v := range fields {
		merged[k] = v
	}
	mb, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	var updated nfqws.Conf
	if err := json.Unmarshal(mb, &updated); err != nil {
		return fmt.Errorf("invalid nfqws config fields: %w", err)
	}
	if err := e.nfqws.SaveConf(updated); err != nil {
		return err
	}
	if e.nfqws.Running() {
		_ = e.nfqws.Reload()
	}
	e.Logf("nfqws2 structured config saved")
	e.publishState()
	return nil
}

// NfqwsLists returns the available hostlist file names.
func (e *Engine) NfqwsLists() ([]string, error) { return e.nfqws.Lists() }

// NfqwsList returns a single hostlist's content.
func (e *Engine) NfqwsList(name string) (NfqwsListView, error) {
	content, err := e.nfqws.ReadList(name)
	if err != nil {
		return NfqwsListView{}, err
	}
	return NfqwsListView{Name: name, Content: content}, nil
}

// SaveNfqwsList writes a hostlist and reloads nfqws2 so it takes effect.
func (e *Engine) SaveNfqwsList(name, content string) error {
	if err := e.nfqws.WriteList(name, content); err != nil {
		return err
	}
	if e.nfqws.Running() {
		_ = e.nfqws.Reload()
	}
	e.Logf("nfqws2 hostlist saved: %s", name)
	return nil
}

// ImportNfqwsList resolves a remote domain-list URL and writes it into the
// nfqws2 hostlists, auto-splitting a large set across numbered sibling files
// (base, base2, …) of at most nfqws.DefaultListSplit domains each so no single
// hostlist grows unwieldy. base defaults to "user.list".
//
//   - replace=true: the resolved set becomes the family's content (stale
//     higher-index siblings from a previous larger import are pruned).
//   - replace=false (append): the resolved set is unioned with whatever the
//     family already holds, then re-split.
//
// The remote fetch is read-only (dry-run safe); the local write is a plain file
// write under /opt, so this works the same on- and off-device. nfqws2 is
// reloaded (SIGHUP) when running so the new lists take effect immediately.
func (e *Engine) ImportNfqwsList(base, url, attr string, replace bool) (NfqwsImportView, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "user.list"
	}
	res, err := e.ResolveList(url, attr)
	if err != nil {
		return NfqwsImportView{}, err
	}

	domains := res.Domains
	mode := "replace"
	if !replace {
		mode = "append"
		existing, err := e.nfqws.ReadListFamily(base)
		if err != nil {
			return NfqwsImportView{}, err
		}
		domains = mergeDomains(existing, res.Domains)
	}
	if len(domains) == 0 {
		return NfqwsImportView{}, fmt.Errorf("no domains resolved from %s", url)
	}

	parts := nfqws.SplitDomains(base, domains, nfqws.DefaultListSplit)
	if _, err := e.nfqws.WriteSplit(base, domains, nfqws.DefaultListSplit); err != nil {
		return NfqwsImportView{}, err
	}
	if e.nfqws.Running() {
		_ = e.nfqws.Reload()
	}

	files := make([]NfqwsListFileView, 0, len(parts))
	for _, p := range parts {
		files = append(files, NfqwsListFileView{Name: p.Name, Count: len(p.Domains)})
	}
	e.Logf("nfqws2 list import (%s): %s -> %d domains across %d file(s)", mode, base, len(domains), len(files))
	e.publishState()
	return NfqwsImportView{
		Base:      nfqws.SplitListName(base, 0),
		Mode:      mode,
		Files:     files,
		Total:     len(domains),
		PerFile:   nfqws.DefaultListSplit,
		Truncated: res.Truncated,
		SkippedN:  res.SkippedN,
		Sources:   res.Sources,
	}, nil
}

// mergeDomains unions two domain slices into a deduped, lowercased, sorted set
// (deterministic ordering keeps the split stable across re-imports).
func mergeDomains(a, b []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, d := range list {
			d = strings.ToLower(strings.TrimSpace(d))
			if d == "" || seen[d] {
				continue
			}
			seen[d] = true
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

// CheckDomain probes a domain's reachability on the direct path (where nfqws2
// does its DPI-bypass) versus through the active VPN tunnel, to help decide
// whether a service needs the tunnel or nfqws2 alone is enough.
func (e *Engine) CheckDomain(domain string) DomainCheckView {
	domain = sanitizeDomain(domain)
	res := DomainCheckView{Domain: domain}
	if domain == "" {
		res.Note = "enter a domain, e.g. rutracker.org"
		return res
	}
	if e.runner.DryRun {
		res.DirectOK, res.BypassOK = true, true
		res.Note = "dry-run: synthetic result"
		return res
	}

	url := "https://" + domain + "/"
	ctx, cancel := context.WithTimeout(e.baseCtx(), 8*time.Second)
	defer cancel()

	res.DirectOK = health.DirectHTTP(ctx, url, 8*time.Second).OK

	// Through-tunnel path via the active Xray SOCKS inbound, if one is running.
	if e.hasActiveXray() {
		res.BypassOK = health.SOCKSHTTP(ctx,
			net.JoinHostPort(xraySocksHost, strconv.Itoa(xraySocksPort)), url, 8*time.Second).OK
	}

	switch {
	case res.DirectOK:
		res.Note = "reachable on the direct path" + nfqwsHint(e)
	case res.BypassOK:
		res.Note = "blocked directly, but reachable through the active tunnel — route this service via VPN"
	default:
		res.Note = "unreachable both directly and via tunnel"
	}
	return res
}

func nfqwsHint(e *Engine) string {
	if e.nfqws.Running() {
		return " (nfqws2 active)"
	}
	return ""
}

func (e *Engine) hasActiveXray() bool {
	st := e.store.Get()
	if st.ActiveConnID == "" {
		return false
	}
	c, ok := findConn(st, st.ActiveConnID)
	return ok && c.Type == model.ConnXray
}

func sanitizeDomain(d string) string {
	d = strings.TrimSpace(strings.ToLower(d))
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	if i := strings.IndexAny(d, "/:"); i >= 0 {
		d = d[:i]
	}
	return d
}
