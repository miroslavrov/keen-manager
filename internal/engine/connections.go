package engine

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/miroslavrov/keen-manager/internal/awg"
	"github.com/miroslavrov/keen-manager/internal/model"
	"github.com/miroslavrov/keen-manager/internal/subscription"
)

// Connections returns the list view of every configured connection.
func (e *Engine) Connections() []ConnView {
	return e.connViews(e.store.Get())
}

// connViews builds list items from persisted connections + live runtime status.
func (e *Engine) connViews(st model.State) []ConnView {
	out := make([]ConnView, 0, len(st.Connections))
	for _, c := range st.Connections {
		out = append(out, e.connView(st, c))
	}
	return out
}

func (e *Engine) connView(st model.State, c model.Connection) ConnView {
	v := ConnView{
		ID:             c.ID,
		Type:           c.Type,
		Name:           c.Name,
		Enabled:        c.Enabled,
		Active:         st.ActiveConnID == c.ID,
		SubscriptionID: c.SubscriptionID,
		FallbackTo:     c.FallbackTo,
		Endpoint:       e.endpointOf(c),
		Location:       locationOf(c),
	}

	// Individually disabled, or the whole subscription stream is off — either way
	// the server is out of the pool, so show it disabled (the UI can still tell
	// the two apart via the per-connection switch vs the subscription toggle).
	if !connEligible(st, c) {
		v.Status = string(model.StatusDisabled)
		return v
	}
	if rs, ok := e.runtimeFor(c.ID); ok {
		v.Status = statusStr(rs.Status)
		v.LatencyMs = rs.LatencyMs
		v.LastCheck = isoOrEmpty(rs.LastCheck)
	} else {
		v.Status = string(model.StatusChecking)
	}
	return v
}

// Connection returns the detailed view for a single connection.
func (e *Engine) Connection(id string) (ConnDetailView, error) {
	st := e.store.Get()
	c, ok := findConn(st, id)
	if !ok {
		return ConnDetailView{}, fmt.Errorf("connection %s not found", id)
	}
	d := ConnDetailView{ConnView: e.connView(st, c)}

	switch c.Type {
	case model.ConnAWG:
		d.Protocol = "AmneziaWG"
		if c.AWG != nil {
			d.ConfigPreview = awg.Generate(redactAWG(c.AWG))
		}
	case model.ConnXray:
		if srv, ok := e.vault.get(c.ID); ok {
			d.Protocol = protocolLabel(srv)
			d.ConfigPreview = xrayPreview(srv)
		} else if c.Xray != nil {
			d.Protocol = protocolLabel(*c.Xray)
		}
	}

	if rs, ok := e.runtimeFor(c.ID); ok {
		d.HandshakeAgeS = rs.HandshakeAge
		d.RxBytes = rs.RxBytes
		d.TxBytes = rs.TxBytes
	}
	d.Integration = e.integrationOf(c)
	return d, nil
}

// integrationOf explains how a connection is exposed to the router — the
// answer to the common "I added it but nothing shows up in the Keenetic UI"
// confusion. It is deliberately honest about the transparent-proxy path.
func (e *Engine) integrationOf(c model.Connection) IntegrationView {
	switch c.Type {
	case model.ConnAWG:
		if iface, ok := e.nativeIface(c.ID); ok && iface != "" {
			return IntegrationView{
				Mode:            "native-interface",
				VisibleInRouter: true,
				Interface:       iface,
				RoutableTarget:  true,
				Summary:         "Shown in the Keenetic UI as interface " + iface + " (Other Connections). Give it a connection priority there, or send specific services through it from the Routes page.",
			}
		}
		if e.useNativeAWG() {
			return IntegrationView{
				Mode:            "native-interface",
				VisibleInRouter: true,
				RoutableTarget:  true,
				Summary:         "Activate this tunnel and it is created as a native AmneziaWG (Wireguard) interface, visible in the Keenetic UI and usable as a Routes target.",
			}
		}
		return IntegrationView{
			Mode:            "userspace-awg",
			VisibleInRouter: false,
			Summary:         "Runs as an Entware userspace tunnel (awg-quick). It is not shown in the Keenetic UI — native AmneziaWG needs KeeneticOS 5.1+.",
		}
	case model.ConnXray:
		if e.xrayProxyMode() {
			iv := IntegrationView{
				Mode:            "keenetic-proxy",
				VisibleInRouter: true,
				RoutableTarget:  true,
			}
			if p := e.managedProxyIface(); p != "" {
				iv.Interface = p
				iv.Summary = "Shown in the Keenetic UI as Proxy connection " + p + " (Other Connections → Proxy). One stable connection to keen-manager's local Xray (SOCKS5); switching server rewrites the tunnel under the hood without touching this interface. Make it your primary connection in Connection Priorities, or send specific services through it from the Routes page."
			} else {
				iv.Summary = "Activate an Xray connection and it is registered as a single KeeneticOS Proxy connection (SOCKS5 → keen-manager's local Xray), visible in the router UI and usable as a Routes target."
			}
			return iv
		}
		return IntegrationView{
			Mode:            "transparent-proxy",
			VisibleInRouter: false,
			Summary:         "vless/vmess connections run inside keen-manager's local Xray and capture traffic transparently (TPROXY). Nothing appears as a router interface — this is expected. Scope which traffic uses it with a kill switch or per-service rules. Install the Proxy client component (and set Xray integration to Proxy) to expose it as one visible connection instead.",
		}
	}
	return IntegrationView{Mode: "unknown", Summary: "Unknown connection type."}
}

// CreateConnection adds an AWG tunnel (from a pasted .conf) or an Xray outbound
// (from a share link). It validates the input before persisting anything.
func (e *Engine) CreateConnection(typ model.ConnType, name, awgConf, shareLink string) (ConnView, error) {
	name = strings.TrimSpace(name)
	id := newID("conn")

	c := model.Connection{
		ID:        id,
		Type:      typ,
		Name:      name,
		Enabled:   true,
		CreatedAt: time.Now(),
	}

	switch typ {
	case model.ConnAWG:
		cfg, err := awg.Parse(awgConf)
		if err != nil {
			return ConnView{}, fmt.Errorf("parse AmneziaWG config: %w", err)
		}
		if err := awg.Validate(cfg); err != nil {
			return ConnView{}, fmt.Errorf("invalid AmneziaWG config: %w", err)
		}
		c.AWG = cfg
		if name == "" {
			c.Name = awg.EndpointHost(cfg)
		}

	case model.ConnXray:
		srv, err := subscription.ParseLink(strings.TrimSpace(shareLink))
		if err != nil {
			return ConnView{}, fmt.Errorf("parse share link: %w", err)
		}
		srv.ID = id
		if name != "" {
			srv.Name = name
		} else if srv.Name != "" {
			c.Name = srv.Name
		} else {
			c.Name = net.JoinHostPort(srv.Address, strconv.Itoa(srv.Port))
		}
		e.vault.put(id, *srv)
		c.Xray = publicServer(srv)

	default:
		return ConnView{}, fmt.Errorf("unknown connection type %q", typ)
	}

	if err := e.store.Mutate(func(s *model.State) error {
		s.Connections = append(s.Connections, c)
		return nil
	}); err != nil {
		e.vault.delete(id)
		return ConnView{}, err
	}

	e.Logf("connection added: %s (%s)", c.Name, typ)
	e.setRuntime(id, model.RuntimeStatus{ConnID: id, Status: model.StatusChecking})
	go e.probeOne(id)
	e.publishState()
	return e.connView(e.store.Get(), c), nil
}

// UpdateConnection changes mutable fields (name, enabled, fallback target).
func (e *Engine) UpdateConnection(id string, fields map[string]any) (ConnView, error) {
	var updated model.Connection
	err := e.store.Mutate(func(s *model.State) error {
		for i := range s.Connections {
			if s.Connections[i].ID != id {
				continue
			}
			if v, ok := fields["name"].(string); ok && strings.TrimSpace(v) != "" {
				s.Connections[i].Name = strings.TrimSpace(v)
			}
			if v, ok := fields["enabled"].(bool); ok {
				s.Connections[i].Enabled = v
				if !v && s.ActiveConnID == id {
					s.ActiveConnID = ""
				}
			}
			if v, ok := fields["fallback_to"]; ok {
				if sv, ok := v.(string); ok {
					s.Connections[i].FallbackTo = sv
				}
			}
			updated = s.Connections[i]
			return nil
		}
		return fmt.Errorf("connection %s not found", id)
	})
	if err != nil {
		return ConnView{}, err
	}
	e.publishState()
	return e.connView(e.store.Get(), updated), nil
}

// DeleteConnection tears the connection down (if running) and removes it, along
// with its vault entry and any references from subscriptions/failover.
func (e *Engine) DeleteConnection(id string) error {
	st := e.store.Get()
	c, ok := findConn(st, id)
	if !ok {
		return fmt.Errorf("connection %s not found", id)
	}
	// Tear down any service routes targeting this connection first (removes
	// their object-groups/dns-proxy routes from the router), then the tunnel.
	for _, r := range st.Routes {
		if r.TargetConnID == id {
			_ = e.unapplyRoute(r)
		}
	}
	// Best-effort teardown before removal.
	_ = e.bringDown(c)

	err := e.store.Mutate(func(s *model.State) error {
		out := s.Connections[:0]
		for _, cc := range s.Connections {
			if cc.ID != id {
				out = append(out, cc)
			}
		}
		s.Connections = out
		if s.ActiveConnID == id {
			s.ActiveConnID = ""
		}
		// Scrub references.
		for i := range s.Connections {
			if s.Connections[i].FallbackTo == id {
				s.Connections[i].FallbackTo = ""
			}
		}
		s.Failover.Chain = removeString(s.Failover.Chain, id)
		for i := range s.Subscriptions {
			s.Subscriptions[i].ServerIDs = removeString(s.Subscriptions[i].ServerIDs, id)
		}
		// Drop service routes that targeted this connection.
		if len(s.Routes) > 0 {
			routes := s.Routes[:0]
			for _, r := range s.Routes {
				if r.TargetConnID != id {
					routes = append(routes, r)
				}
			}
			s.Routes = routes
		}
		return nil
	})
	if err != nil {
		return err
	}
	e.vault.delete(id)
	e.dropRuntime(id)
	// If this was the last Xray connection, retire the shared managed Proxy
	// interface (the single exit point) so keen-manager cleans up after itself.
	if c.Type == model.ConnXray {
		e.teardownManagedProxyIfaceIfUnused()
	}
	e.Logf("connection deleted: %s", c.Name)
	e.publishState()
	return nil
}

// ConnectionAction performs an imperative action on a connection.
func (e *Engine) ConnectionAction(id, action string) error {
	st := e.store.Get()
	c, ok := findConn(st, id)
	if !ok {
		return fmt.Errorf("connection %s not found", id)
	}
	switch action {
	case "activate":
		return e.Activate(id)
	case "up":
		if err := e.bringUp(c); err != nil {
			return err
		}
		go e.probeOne(id)
		e.publishState()
		return nil
	case "down":
		if err := e.bringDown(c); err != nil {
			return err
		}
		if st.ActiveConnID == id {
			_ = e.store.Mutate(func(s *model.State) error { s.ActiveConnID = ""; return nil })
		}
		e.publishState()
		return nil
	case "test":
		e.probeOne(id)
		e.publishState()
		return nil
	default:
		return fmt.Errorf("unknown action %q", action)
	}
}

// ----- endpoint / label helpers -----

func (e *Engine) endpointOf(c model.Connection) string {
	switch c.Type {
	case model.ConnAWG:
		if c.AWG != nil {
			return c.AWG.Peer.Endpoint
		}
	case model.ConnXray:
		if c.Xray != nil {
			return net.JoinHostPort(c.Xray.Address, strconv.Itoa(c.Xray.Port))
		}
	}
	return ""
}

func locationOf(c model.Connection) string {
	if c.Type == model.ConnXray && c.Xray != nil {
		return c.Xray.Location
	}
	return ""
}

// endpointHostPort splits a connection's endpoint into host + port for probing.
func endpointHostPort(c model.Connection) (string, int) {
	switch c.Type {
	case model.ConnAWG:
		if c.AWG != nil {
			h, p, err := net.SplitHostPort(c.AWG.Peer.Endpoint)
			if err == nil {
				return h, atoi(p)
			}
		}
	case model.ConnXray:
		if c.Xray != nil {
			return c.Xray.Address, c.Xray.Port
		}
	}
	return "", 0
}

func protocolLabel(s model.Server) string {
	switch s.Protocol {
	case model.ProtoVLESS:
		if s.Security == "reality" {
			return "VLESS + REALITY"
		}
		if s.Security == "tls" {
			return "VLESS + TLS"
		}
		return "VLESS"
	case model.ProtoVMess:
		return "VMess"
	case model.ProtoTrojan:
		return "Trojan"
	case model.ProtoSS:
		return "Shadowsocks"
	default:
		return strings.ToUpper(string(s.Protocol))
	}
}

// publicServer returns a copy carrying only non-secret fields, for embedding in
// the persisted Connection (secrets live only in the vault).
func publicServer(s *model.Server) *model.Server {
	return &model.Server{
		ID:       s.ID,
		Name:     s.Name,
		Location: s.Location,
		Protocol: s.Protocol,
		Address:  s.Address,
		Port:     s.Port,
		Security: s.Security,
		Network:  s.Network,
	}
}

// redactAWG returns a copy of an AWG config with private material masked, for
// safe display in the UI config preview.
func redactAWG(cfg *model.AWGConfig) *model.AWGConfig {
	cp := *cfg
	cp.PrivateKey = redact(cp.PrivateKey)
	if cp.Peer.PresharedKey != "" {
		cp.Peer.PresharedKey = redact(cp.Peer.PresharedKey)
	}
	return &cp
}

// xrayPreview renders a compact, secret-redacted summary of an Xray server.
func xrayPreview(s model.Server) string {
	m := map[string]any{
		"protocol": string(s.Protocol),
		"address":  s.Address,
		"port":     s.Port,
	}
	if s.Network != "" {
		m["network"] = s.Network
	}
	if s.Security != "" {
		m["security"] = s.Security
	}
	if s.SNI != "" {
		m["sni"] = s.SNI
	}
	if s.Flow != "" {
		m["flow"] = s.Flow
	}
	if s.Fingerprint != "" {
		m["fp"] = s.Fingerprint
	}
	if s.UUID != "" {
		m["id"] = redact(s.UUID)
	}
	if s.Password != "" {
		m["password"] = redact(s.Password)
	}
	if s.PublicKey != "" {
		m["pbk"] = redact(s.PublicKey)
	}
	if s.Path != "" {
		m["path"] = s.Path
	}
	if s.Host != "" {
		m["host"] = s.Host
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return string(b)
}

// redact masks a secret, keeping a short head/tail hint for recognisability.
func redact(s string) string {
	if len(s) <= 8 {
		return "redacted"
	}
	return s[:4] + "…redacted…" + s[len(s)-4:]
}

func findConn(st model.State, id string) (model.Connection, bool) {
	for _, c := range st.Connections {
		if c.ID == id {
			return c, true
		}
	}
	return model.Connection{}, false
}

func removeString(list []string, target string) []string {
	out := list[:0]
	for _, s := range list {
		if s != target {
			out = append(out, s)
		}
	}
	return out
}

func atoi(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }
