package engine

import (
	"context"
	"fmt"
	"net"
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

	// Index existing member connections by a stable endpoint key.
	existing := map[string]string{} // key -> connID
	for _, c := range st.Connections {
		if c.SubscriptionID == id && c.Xray != nil {
			existing[serverKey(*c.Xray)] = c.ID
		}
	}

	seen := map[string]bool{}
	vaultPuts := map[string]model.Server{}
	var newServerIDs []string
	var addConns []model.Connection
	now := time.Now()

	err = e.store.Mutate(func(s *model.State) error {
		for _, srv := range res.Servers {
			k := serverKey(srv)
			if cid, found := existing[k]; found {
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
				newServerIDs = append(newServerIDs, cid)
				continue
			}
			cid := newID("conn")
			srv.ID = cid
			vaultPuts[cid] = srv
			addConns = append(addConns, model.Connection{
				ID: cid, Type: model.ConnXray, Name: serverName(srv), Enabled: true,
				SubscriptionID: id, Xray: publicServer(&srv), CreatedAt: now,
			})
			newServerIDs = append(newServerIDs, cid)
		}
		s.Connections = append(s.Connections, addConns...)

		// Remove members that vanished from the provider (except the active one,
		// which we keep so a refresh never kills the live tunnel).
		var removeIDs []string
		for k, cid := range existing {
			if !seen[k] && cid != s.ActiveConnID {
				removeIDs = append(removeIDs, cid)
			} else if !seen[k] && cid == s.ActiveConnID {
				newServerIDs = append(newServerIDs, cid) // keep stale-but-active
			}
		}
		if len(removeIDs) > 0 {
			rm := map[string]bool{}
			for _, r := range removeIDs {
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
			for _, r := range removeIDs {
				s.Failover.Chain = removeString(s.Failover.Chain, r)
			}
		}

		for i := range s.Subscriptions {
			if s.Subscriptions[i].ID == id {
				s.Subscriptions[i].ServerIDs = newServerIDs
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
	// Drop vault + runtime for connections that were removed.
	current := map[string]bool{}
	for _, c := range e.store.Get().Connections {
		current[c.ID] = true
	}
	for k, cid := range existing {
		if !seen[k] && !current[cid] {
			e.vault.delete(cid)
			e.dropRuntime(cid)
		}
	}

	e.Logf("subscription refreshed: %s (%d servers)", sub.Name, len(res.Servers))
	e.publishState()
	go e.probeSubscription(id)
	st = e.store.Get()
	sv, _ := findSub(st, id)
	return e.subView(st, sv), nil
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
// fields — name, auto_select_best and update_interval_hours — and returns the
// refreshed view. Server membership is only ever changed by RefreshSubscription,
// so this is safe to call without touching the active tunnel.
func (e *Engine) UpdateSubscription(id string, fields map[string]any) (SubView, error) {
	if _, ok := findSub(e.store.Get(), id); !ok {
		return SubView{}, fmt.Errorf("subscription %s not found", id)
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
			// JSON numbers decode to float64.
			if v, ok := fields["update_interval_hours"].(float64); ok {
				if h := int(v); h >= 0 {
					s.Subscriptions[i].UpdateInterval = h
				}
			}
		}
		return nil
	}); err != nil {
		return SubView{}, err
	}
	e.Logf("subscription updated: %s", id)
	e.publishState()
	st := e.store.Get()
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
			Status:   string(model.StatusChecking),
		}
		if rs, ok := e.runtimeFor(c.ID); ok {
			sv.Status = statusStr(rs.Status)
			sv.LatencyMs = rs.LatencyMs
		}
		out = append(out, sv)
	}
	return out
}

// SelectBest pings all servers in a subscription and activates the fastest one
// that responds.
func (e *Engine) SelectBest(id string) (string, error) {
	st := e.store.Get()
	sub, ok := findSub(st, id)
	if !ok {
		return "", fmt.Errorf("subscription %s not found", id)
	}
	var members []model.Connection
	for _, c := range st.Connections {
		if c.SubscriptionID == id && c.Enabled {
			members = append(members, c)
		}
	}
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

	best := e.fastest(members)
	if best == "" {
		return "", fmt.Errorf("no reachable server in subscription %q", sub.Name)
	}
	e.Logf("select-best for %s -> %s", sub.Name, best)
	if err := e.Activate(best); err != nil {
		return "", err
	}
	return best, nil
}

// fastest concurrently TCP-pings the given connections and returns the id of the
// lowest-latency reachable one ("" if none respond).
func (e *Engine) fastest(members []model.Connection) string {
	type res struct {
		id  string
		ms  int
		ok  bool
	}
	results := make([]res, len(members))
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
			results[i] = res{id: id, ms: p.LatencyMs, ok: p.OK}
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

	best := ""
	bestMs := 1 << 30
	for _, r := range results {
		if r.ok && r.ms < bestMs {
			bestMs = r.ms
			best = r.id
		}
	}
	return best
}

// probeSubscription probes all of a subscription's servers (best-effort).
func (e *Engine) probeSubscription(id string) {
	if e.runner.DryRun {
		return
	}
	st := e.store.Get()
	var members []model.Connection
	for _, c := range st.Connections {
		if c.SubscriptionID == id && c.Enabled {
			members = append(members, c)
		}
	}
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

func isoPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return isoOrEmpty(*t)
}
