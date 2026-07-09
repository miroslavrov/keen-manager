package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/listsrc"
)

// ResolveList fetches a remote domain-list URL and flattens it into a deduped,
// lowercased domain slice suitable for an nfqws2 hostlist or a Routes custom
// domain set. It understands the v2fly domain-list-community format (include:
// directives and @attribute tags are expanded), plain newline domain lists, and
// hosts/AdBlock files.
//
// This is a read-only network fetch — it never mutates the device — so it runs
// the same on- and off-device (dry-run does not disable it). Callers surface the
// result for review before it is written to a list.
func (e *Engine) ResolveList(url, attr string) (listsrc.Result, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return listsrc.Result{}, fmt.Errorf("list url is required")
	}
	// A generous ceiling: recursive v2fly includes can fan out to dozens of
	// files. listsrc enforces its own MaxFiles/MaxDomains caps within this.
	ctx, cancel := context.WithTimeout(e.baseCtx(), 90*time.Second)
	defer cancel()

	res, err := listsrc.Resolve(ctx, url, listsrc.Options{
		AttrFilter: strings.TrimSpace(attr),
	})
	if err != nil {
		return listsrc.Result{}, err
	}
	e.Logf("list resolved: %s -> %d domains from %d source(s)", url, len(res.Domains), len(res.Sources))
	return res, nil
}
