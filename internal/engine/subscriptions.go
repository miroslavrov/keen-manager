package engine

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miroslavrov/keen-manager/internal/health"
	"github.com/miroslavrov/keen-manager/internal/model"
	"github.com/miroslavrov/keen-manager/internal/subscription"
)

// Subscriptions returns the list view of every subscription.
func (e *Engine) Subscriptions() []SubView {
	st := e.store.Get()
	out := make([]SubView, 0, len(st.Subscriptions))
	for _, s := range st.Subscriptions {
		out = append(out, e.subView(st, s))
	}
	return out
}

func (e *Engine) subView(st model.State, s model.Subscription) SubView {
	v := SubView{
		ID:                  s.ID,
		Name:                s.Name,
		URL:                 s.URL,
		Host:                s.Host,
		Protocol:            e.subProtocol(st, s),
		ServerCount:         s.ServerCount,
		LastUpdate:          isoPtr(s.LastUpdate),
		UpdateIntervalHours: s.UpdateInterval,
		AutoSelectBest:      s.AutoSelectBest,
		Enabled:             s.Enabled,
	}
	if s.UserInfo != nil {
		v.UserInfo = &SubUserInfoView{
			UsedBytes:  s.UserInfo.UsedBytes,
			TotalBytes: s.UserInfo.TotalBytes,
			Expire:     isoPtr(s.UserInfo.Expire),
		}
	}
	return v
}

// subProtocol summarises the transport mix of a subscription's servers.
func (e *Engine) subProtocol(st model.State, s model.Subscription) string {
	set := map[string]struct{}{}
	for _, c := range st.Connections {
		if c.SubscriptionID == s.ID && c.Xray != nil {
			set[string(c.Xray.Protocol)] = struct{}{}
		}
	}
	switch len(set) {
	case 0:
		return "mixed"
	case 1:
		for k := range set {
			return k
		}
	}
	return "mixed"
}

// CreateSubscription fetches + parses a subscription URL and materialises one
// Xray connection per server.
func (e *Engine) CreateSubscription(name, url string) (SubView, error) {
	name = strings.TrimSpace(name)
	ctx, cancel := context.WithTimeout(e.baseCtx(), 35*time.Second)
	defer cancel()

	res, err := subscription.Fetch(ctx, url)
	if err != nil {
		return SubView{}, err
	}
	if len(res.Servers) == 0 {
		return SubView{}, fmt.Errorf("subscription contained no servers")
	}

	subID := newID("sub")
	sub := model.Subscription{
		ID:             subID,
		// Prefer the user-supplied name; otherwise adopt the panel's advertised
		// profile-title (parity with native clients), falling back to the host.
		Name:           firstNonEmpty(name, firstNonEmpty(res.Title, res.Host)),
		URL:            url,
		Host:           res.Host,
		ServerCount:    len(res.Servers),
		UpdateInterval: res.UpdateIntervalHours,
		UserInfo:       res.UserInfo,
		AutoSelectBest: true,
		Enabled:        true,
	}
	now := time.Now()
	sub.LastUpdate = &now

	newConns := make([]model.Connection, 0, len(res.Servers))
	vaultPuts := map[string]model.Server{}
	for _, s := range res.Servers {
		connID := newID("conn")
		s.ID = connID
		vaultPuts[connID] = s
		sub.ServerIDs = append(sub.ServerIDs, connID)
		newConns = append(newConns, model.Connection{
			ID:             connID,
			Type:           model.ConnXray,
			Name:           serverName(s),
			Enabled:        true,
			SubscriptionID: subID,
			Xray:           publicServer(&s),
			CreatedAt:      now,
		})
	}

	if err := e.store.Mutate(func(st *model.State) error {
		st.Subscriptions = append(st.Subscriptions, sub)
		st.Connections = append(st.Connections, newConns...)
		return nil
	}); err != nil {
		return SubView{}, err
	}
	for id, srv := range vaultPuts {
		e.vault.put(id, srv)
		e.setRuntime(id, model.RuntimeStatus{ConnID: id, Status: model.StatusChecking})
	}

	e.Logf("subscription added: %s (%d servers)", sub.Name, len(newConns))
	e.publishState()
	go e.probeSubscription(subID)
	return e.subView(e.store.Get(), sub), nil
}

// RefreshSubscription re-fetches a subscription and reconciles its servers,
// preserving connection ids for servers that still exist (so failover chains
// and the active selection survive an update).
func (e *Engine) RefreshSubscription(id string) (SubView, error) {
	st := e.store.Get()
	sub, ok := findSub(st, id)
	if !ok {
		return SubView{}, fmt.Errorf("subscription %s not found", id)
	}

	ctx, cancel := context.WithTimeout(e.baseCtx(), 35*time.Second)
	defer cancel()
	res, err := subscription.Fetch(ctx, sub.URL)
	if err != nil {
		return SubView{}, err
	}

	var vaultPuts map[string]model.Server
	var removedIDs []string
	now := time.Now()

	err = e.store.Mutate(func(s *model.State) error {
		var serverIDs []string
		vaultPuts, removedIDs, serverIDs = reconcileSubscriptionMembers(s, id, res.Servers, now)
		for i := range s.Subscriptions {
			if s.Subscriptions[i].ID == id {
				s.Subscriptions[i].ServerIDs = serverIDs
				s.Subscriptions[i].ServerCount = len(res.Servers)
				s.Subscriptions[i].Host = res.Host
				s.Subscriptions[i].UpdateInterval = res.UpdateIntervalHours
				s.Subscriptions[i].UserInfo = res.UserInfo
				s.Subscriptions[i].LastUpdate = &now
			}
		}
		return nil
	})
	if err != nil {
		return SubView{}, err
	}

	for cid, srv := range vaultPuts {
		e.vault.put(cid, srv)
	}
	// Drop vault + runtime for the connections the reconcile removed.
	for _, cid := range removedIDs {
		e.vault.delete(cid)
		e.dropRuntime(cid)
	}

	e.Logf("subscription refreshed: %s (%d servers)", sub.Name, len(res.Servers))
	e.publishState()
	go e.probeSubscription(id)
	st = e.store.Get()
	sv, _ := findSub(st, id)
	return e.subView(st, sv), nil
}

// reconcileSubscriptionMembers reconciles subscription subID's member
// connections against a freshly-fetched server list, in place on s. It is the
// pure (no network, no device I/O) core of RefreshSubscription, so its central
// contract is unit-tested directly:
//
//   - A fetched server whose endpoint key (address|port|protocol, see serverKey)
//     matches an existing member KEEPS that member — same connection id AND its
//     per-connection Enabled flag. So a server the user switched off (e.g. a
//     home-country node they don't want auto-best to pick) STAYS off across a
//     refresh, and a refresh never silently re-enables it.
//   - A fetched server with no matching member is a genuinely new/changed
//     endpoint: it is added as a fresh, enabled connection (a changed endpoint is
//     treated as new, so its enabled state resets — which is acceptable).
//   - A member that vanished from the provider is removed, EXCEPT the active one
//     (kept, stale, so a refresh never tears down the live tunnel).
//
// It returns the vault entries to persist (connID -> server with secrets), the
// connection ids removed, and the subscription's new ServerIDs order.
func reconcileSubscriptionMembers(s *model.State, subID string, fetched []model.Server, now time.Time) (vaultPuts map[string]model.Server, removedIDs []string, serverIDs []string) {
	// Index existing member connections by a stable endpoint key.
	existing := map[string]string{} // key -> connID
	for _, c := range s.Connections {
		if c.SubscriptionID == subID && c.Xray != nil {
			existing[serverKey(*c.Xray)] = c.ID
		}
	}

	seen := map[string]bool{}
	vaultPuts = map[string]model.Server{}
	var addConns []model.Connection

	for _, srv := range fetched {
		k := serverKey(srv)
		if cid, found := existing[k]; found {
			// Unchanged endpoint → reuse the connection verbatim. Only the public
			// Xray fields (and a missing name) are refreshed; Enabled is left
			// untouched so the user's per-server choice survives the update.
			seen[k] = true
			srv.ID = cid
			vaultPuts[cid] = srv
			for i := range s.Connections {
				if s.Connections[i].ID == cid {
					s.Connections[i].Xray = publicServer(&srv)
					if s.Connections[i].Name == "" {
						s.Connections[i].Name = serverName(srv)
					}
				}
			}
			serverIDs = append(serverIDs, cid)
			continue
		}
		cid := newID("conn")
		srv.ID = cid
		vaultPuts[cid] = srv
		addConns = append(addConns, model.Connection{
			ID: cid, Type: model.ConnXray, Name: serverName(srv), Enabled: true,
			SubscriptionID: subID, Xray: publicServer(&srv), CreatedAt: now,
		})
		serverIDs = append(serverIDs, cid)
	}
	s.Connections = append(s.Connections, addConns...)

	// Remove members that vanished from the provider (except the active one,
	// which we keep so a refresh never kills the live tunnel).
	for k, cid := range existing {
		if !seen[k] && cid != s.ActiveConnID {
			removedIDs = append(removedIDs, cid)
		} else if !seen[k] && cid == s.ActiveConnID {
			serverIDs = append(serverIDs, cid) // keep stale-but-active
		}
	}
	if len(removedIDs) > 0 {
		rm := map[string]bool{}
		for _, r := range removedIDs {
			rm[r] = true
		}
		out := s.Connections[:0]
		for _, c := range s.Connections {
			if !rm[c.ID] {
				out = append(out, c)
			}
		}
		s.Connections = out
		for i := range s.Connections {
			if rm[s.Connections[i].FallbackTo] {
				s.Connections[i].FallbackTo = ""
			}
		}
		for _, r := range removedIDs {
			s.Failover.Chain = removeString(s.Failover.Chain, r)
		}
	}
	return vaultPuts, removedIDs, serverIDs
}

// DeleteSubscription removes a subscription and all connections created from it.
func (e *Engine) DeleteSubscription(id string) error {
	st := e.store.Get()
	sub, ok := findSub(st, id)
	if !ok {
		return fmt.Errorf("subscription %s not found", id)
	}

	var removed []string
	for _, c := range st.Connections {
		if c.SubscriptionID == id {
			removed = append(removed, c.ID)
			_ = e.bringDown(c)
		}
	}

	err := e.store.Mutate(func(s *model.State) error {
		rm := map[string]bool{}
		for _, r := range removed {
			rm[r] = true
		}
		out := s.Connections[:0]
		for _, c := range s.Connections {
			if !rm[c.ID] {
				out = append(out, c)
			}
		}
		s.Connections = out
		if rm[s.ActiveConnID] {
			s.ActiveConnID = ""
		}
		for i := range s.Connections {
			if rm[s.Connections[i].FallbackTo] {
				s.Connections[i].FallbackTo = ""
			}
		}
		for _, r := range removed {
			s.Failover.Chain = removeString(s.Failover.Chain, r)
		}
		subs := s.Subscriptions[:0]
		for _, x := range s.Subscriptions {
			if x.ID != id {
				subs = append(subs, x)
			}
		}
		s.Subscriptions = subs
		return nil
	})
	if err != nil {
		return err
	}
	for _, r := range removed {
		e.vault.delete(r)
		e.dropRuntime(r)
	}
	e.Logf("subscription deleted: %s (%d connections removed)", sub.Name, len(removed))
	e.publishState()
	return nil
}

// UpdateSubscription applies a partial update to a subscription's editable
// fields — name, auto_select_best, update_interval_hours and the enabled
// (stream on/off) switch — and returns the refreshed view. Server membership is
// only ever changed by RefreshSubscription. Turning the stream OFF while one of
// its servers is the active tunnel tears that tunnel down and sends the LAN to
// the direct path (mirroring per-connection disable and the master connector),
// so a disabled subscription can never keep routing.
func (e *Engine) UpdateSubscription(id string, fields map[string]any) (SubView, error) {
	st := e.store.Get()
	sub, ok := findSub(st, id)
	if !ok {
		return SubView{}, fmt.Errorf("subscription %s not found", id)
	}

	// Detect a stream on->off transition, and whether the active tunnel belongs
	// to this subscription — if so we tear it down (outside the Mutate, since it
	// touches the device) after persisting the flag.
	disabling := false
	if v, ok := fields["enabled"].(bool); ok && sub.Enabled && !v {
		disabling = true
	}
	// Detect an auto-select-best OFF->ON transition so it can be applied right
	// away instead of the user waiting up to AutoSelectIntervalMin for the next
	// scheduled tick.
	enablingAuto := false
	if v, ok := fields["auto_select_best"].(bool); ok && !sub.AutoSelectBest && v {
		enablingAuto = true
	}
	var toDown *model.Connection
	if disabling && st.ActiveConnID != "" {
		if c, ok := findConn(st, st.ActiveConnID); ok && c.SubscriptionID == id {
			cc := c
			toDown = &cc
		}
	}

	if err := e.store.Mutate(func(s *model.State) error {
		for i := range s.Subscriptions {
			if s.Subscriptions[i].ID != id {
				continue
			}
			if v, ok := fields["name"].(string); ok {
				if n := strings.TrimSpace(v); n != "" {
					s.Subscriptions[i].Name = n
				}
			}
			if v, ok := fields["auto_select_best"].(bool); ok {
				s.Subscriptions[i].AutoSelectBest = v
			}
			if v, ok := fields["enabled"].(bool); ok {
				s.Subscriptions[i].Enabled = v
			}
			// JSON numbers decode to float64.
			if v, ok := fields["update_interval_hours"].(float64); ok {
				if h := int(v); h >= 0 {
					s.Subscriptions[i].UpdateInterval = h
				}
			}
		}
		if toDown != nil {
			s.ActiveConnID = ""
		}
		return nil
	}); err != nil {
		return SubView{}, err
	}

	if toDown != nil {
		e.revertRouting(*toDown)
		_ = e.bringDown(*toDown)
		e.setRuntime(toDown.ID, model.RuntimeStatus{
			ConnID: toDown.ID, Status: model.StatusDown, Active: false,
			LastCheck: time.Now(), Message: "subscription disabled",
		})
		e.foResetFail()
		e.foResetBackoff()
		e.Logf("subscription %q disabled — active server torn down, LAN on the direct path", sub.Name)
	}

	e.Logf("subscription updated: %s", id)
	e.publishState()
	// Apply a freshly-enabled auto-best immediately (async, non-blocking).
	// autoSelectTick is self-guarding: it no-ops in dry-run, when the connector is
	// paused, or when the active connection is not a member of THIS subscription,
	// and uses the same hysteresis as the loop — so it migrates to the fastest
	// ENABLED server only when that is a meaningful win, never a pointless switch.
	if enablingAuto && !e.runner.DryRun {
		go e.autoSelectTick()
	}
	st = e.store.Get()
	sv, _ := findSub(st, id)
	return e.subView(st, sv), nil
}

// SubscriptionServers returns the server list view for a subscription.
func (e *Engine) SubscriptionServers(id string) []ServerView {
	st := e.store.Get()
	out := []ServerView{}
	for _, c := range st.Connections {
		if c.SubscriptionID != id || c.Xray == nil {
			continue
		}
		sv := ServerView{
			ID:       c.ID,
			Name:     c.Name,
			Location: c.Xray.Location,
			Address:  c.Xray.Address,
			Port:     c.Xray.Port,
			Protocol: protocolLabel(*c.Xray),
			Active:   st.ActiveConnID == c.ID,
			Enabled:  c.Enabled,
			Status:   string(model.StatusChecking),
		}
		// A server the user switched off (or whose subscription stream is off) is
		// out of the auto-best / select-best pool — surface it as disabled so the
		// card shows exactly which servers auto-best will and won't consider.
		if !connEligible(st, c) {
			sv.Status = string(model.StatusDisabled)
		} else if rs, ok := e.runtimeFor(c.ID); ok {
			sv.Status = statusStr(rs.Status)
			sv.LatencyMs = rs.LatencyMs
		}
		out = append(out, sv)
	}
	return out
}

// maxSelectCandidates bounds how many latency-ranked servers select-best will
// try to bring up before giving up, so a subscription full of pingable-but-
// DPI-blocked servers can't turn one click into dozens of activation attempts.
const maxSelectCandidates = 5

// SelectBest ranks a subscription's servers by TCP latency and activates the
// fastest one whose tunnel actually carries traffic end-to-end. TCP reachability
// alone isn't enough — a server can answer on :443 while DPI tears down its
// reality/TLS session — so candidates are tried best-first and each is verified
// through the tunnel by Activate's verify-then-rollback deadman; the first that
// passes wins. This makes "select best" resilient to a fast-pinging dead server
// instead of failing the whole action on it, and rolls back to the previously
// active connection if every candidate fails.
func (e *Engine) SelectBest(id string) (string, error) {
	st := e.store.Get()
	sub, ok := findSub(st, id)
	if !ok {
		return "", fmt.Errorf("subscription %s not found", id)
	}
	if !sub.Enabled {
		return "", fmt.Errorf("subscription %q is disabled — enable it first", sub.Name)
	}
	members := subMembers(st, id)
	if len(members) == 0 {
		return "", fmt.Errorf("subscription %q has no enabled servers", sub.Name)
	}

	// Dry-run: no real pings — pick the first and (inertly) activate it.
	if e.runner.DryRun {
		best := members[0].ID
		if err := e.Activate(best); err != nil {
			return "", err
		}
		return best, nil
	}

	ranked := e.rankReachable(members)
	if len(ranked) == 0 {
		return "", fmt.Errorf("no reachable server in subscription %q", sub.Name)
	}

	// Provision xray-core once up front (bounded) so each candidate bring-up is
	// just a config swap + restart and the per-candidate verify budget isn't
	// eaten by a one-time xray-core download. Best-effort: bringUp re-checks.
	ictx, icancel := context.WithTimeout(e.baseCtx(), 4*time.Minute)
	_ = e.xray.Ensure(ictx)
	icancel()

	limit := len(ranked)
	if limit > maxSelectCandidates {
		limit = maxSelectCandidates
	}
	var lastErr error
	for tried, cid := range ranked[:limit] {
		c, ok := findConn(e.store.Get(), cid)
		if !ok {
			continue
		}
		e.Logf("select-best for %s: trying %s (%d/%d candidates)", sub.Name, c.Name, tried+1, limit)
		ctx, cancel := e.selectActivateCtx()
		err := e.activate(ctx, cid)
		cancel()
		if err == nil {
			e.Logf("select-best for %s -> %s", sub.Name, c.Name)
			return cid, nil
		}
		lastErr = err
		e.Logf("select-best: %s did not verify: %v", c.Name, err)
	}
	if lastErr != nil {
		return "", fmt.Errorf("no server in subscription %q carried traffic (tried %d of %d reachable); last error: %w", sub.Name, limit, len(ranked), lastErr)
	}
	return "", fmt.Errorf("no reachable server in subscription %q", sub.Name)
}

// selectVerifyCapS caps the per-candidate verify budget during select-best so
// trying several servers stays responsive for an interactive click; a working
// tunnel verifies on its first probe well within it.
const selectVerifyCapS = 25

// selectBringUpMarginS is headroom over the verify budget for the candidate
// bring-up itself (config swap + xray restart; xray-core is pre-ensured).
const selectBringUpMarginS = 15

// selectActivateCtx returns a bounded context for one select-best candidate
// activation. activate observes the context in bringUpCtx and verifyActive, so
// a dead candidate is abandoned at the cap instead of burning the full
// (interactive) rollback timeout per server.
func (e *Engine) selectActivateCtx() (context.Context, context.CancelFunc) {
	budget := e.rollbackTimeout()
	if budget > selectVerifyCapS {
		budget = selectVerifyCapS
	}
	return context.WithTimeout(e.baseCtx(), time.Duration(budget+selectBringUpMarginS)*time.Second)
}

// fastest returns the id of the lowest-latency reachable connection ("" if none
// respond). Retained for the background subscription probe.
func (e *Engine) fastest(members []model.Connection) string {
	if ranked := e.rankReachable(members); len(ranked) > 0 {
		return ranked[0]
	}
	return ""
}

// connLatency is one server's TCP-ping outcome, ranked by rankByLatency.
type connLatency struct {
	id string
	ms int
	ok bool
}

// rankByLatency returns the ids of the reachable (ok) results ordered
// fastest-first. Pure and order-stable (ties keep input order), so the ranking
// is unit-tested without real network probes.
func rankByLatency(in []connLatency) []string {
	ranked := make([]connLatency, 0, len(in))
	for _, r := range in {
		if r.ok {
			ranked = append(ranked, r)
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].ms < ranked[j].ms })
	ids := make([]string, len(ranked))
	for i, r := range ranked {
		ids[i] = r.id
	}
	return ids
}

// rankReachable concurrently TCP-pings the given connections, records each
// result as runtime status, and returns the ids of the reachable ones ordered
// fastest-first. Off-device endpoints (no host/port) are skipped. Shared by
// select-best (which tries them best-first through the tunnel) and the
// background subscription probe.
func (e *Engine) rankReachable(members []model.Connection) []string {
	results := make([]connLatency, len(members))
	var wg sync.WaitGroup
	for i, c := range members {
		host, port := endpointHostPort(c)
		if host == "" || port == 0 {
			continue
		}
		wg.Add(1)
		go func(i int, id, host string, port int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(e.baseCtx(), 6*time.Second)
			defer cancel()
			p := health.TCPPing(ctx, host, port, 6*time.Second)
			results[i] = connLatency{id: id, ms: p.LatencyMs, ok: p.OK}
			rs := model.RuntimeStatus{ConnID: id, LastCheck: time.Now(), LatencyMs: p.LatencyMs}
			if p.OK {
				rs.Status = model.StatusUp
			} else {
				rs.Status = model.StatusDown
			}
			e.setRuntime(id, rs)
		}(i, c.ID, host, port)
	}
	wg.Wait()
	return rankByLatency(results)
}

// probeSubscription probes all of a subscription's servers (best-effort). A
// disabled subscription is skipped — its servers are ineligible, and probeAll
// already marks them disabled.
func (e *Engine) probeSubscription(id string) {
	if e.runner.DryRun {
		return
	}
	st := e.store.Get()
	if sub, ok := findSub(st, id); ok && !sub.Enabled {
		return
	}
	members := subMembers(st, id)
	if len(members) > 0 {
		e.fastest(members)
		e.publishState()
	}
}

// ----- helpers -----

func serverName(s model.Server) string {
	if strings.TrimSpace(s.Name) != "" {
		return s.Name
	}
	if s.Location != "" {
		return s.Location
	}
	return net.JoinHostPort(s.Address, strconv.Itoa(s.Port))
}

func serverKey(s model.Server) string {
	return strings.ToLower(s.Address) + "|" + strconv.Itoa(s.Port) + "|" + string(s.Protocol)
}

func findSub(st model.State, id string) (model.Subscription, bool) {
	for _, s := range st.Subscriptions {
		if s.ID == id {
			return s, true
		}
	}
	return model.Subscription{}, false
}

// subEnabled reports whether the subscription stream with the given id is on. A
// blank id (a manually-added connection with no subscription) is always on; an
// unknown id is treated as on (defensive — a dangling reference must not silently
// disable a connection).
func subEnabled(st model.State, subID string) bool {
	if subID == "" {
		return true
	}
	for _, s := range st.Subscriptions {
		if s.ID == subID {
			return s.Enabled
		}
	}
	return true
}

// connEligible reports whether a connection may be brought up or selected by the
// engine: it must be individually enabled AND (when it came from a subscription)
// its subscription stream must be enabled. This is the single predicate the
// candidate pools, background loops and activation share, so the three toggle
// levels — master connector / subscription stream / per-connection — compose.
func connEligible(st model.State, c model.Connection) bool {
	return c.Enabled && subEnabled(st, c.SubscriptionID)
}

// subMembers returns the connections of subscription subID that are currently in
// the pool for activation / select-best / auto-select-best: the per-server
// switch (Connection.Enabled) is on AND the subscription stream is on
// (connEligible). This is the SINGLE predicate select-best, the auto-select loop
// and the background probe share, so a server the user switched off is uniformly
// excluded from every "pick the best" path — the whole point of the per-server
// toggle (drop a home-country node so auto-best never migrates onto it just
// because its ping is lowest).
func subMembers(st model.State, subID string) []model.Connection {
	var out []model.Connection
	for _, c := range st.Connections {
		if c.SubscriptionID == subID && connEligible(st, c) {
			out = append(out, c)
		}
	}
	return out
}

// shouldMigrateAfterDisable reports whether switching off the server that was
// active should hand off to the best REMAINING enabled server of subscription
// subID (rather than dropping to the direct path). It does so only when that
// subscription is in auto-select-best mode and on, and the master connector is
// not paused — i.e. the user has asked to be kept on the best server, so
// removing the one they don't want should move them onto the best one they do.
// Pure (no engine state) so the decision is unit-tested directly.
func shouldMigrateAfterDisable(st model.State, subID string) bool {
	if st.TunnelPaused || subID == "" {
		return false
	}
	sub, ok := findSub(st, subID)
	return ok && sub.AutoSelectBest && sub.Enabled
}

func isoPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return isoOrEmpty(*t)
}
