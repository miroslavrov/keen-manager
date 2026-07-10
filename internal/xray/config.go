package xray

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// Config is a full Xray-core configuration document.
type Config struct {
	Log              *Log              `json:"log,omitempty"`
	API              *API              `json:"api,omitempty"`
	Stats            *struct{}         `json:"stats,omitempty"`
	Inbounds         []Inbound         `json:"inbounds"`
	Outbounds        []Outbound        `json:"outbounds"`
	Routing          *Routing          `json:"routing,omitempty"`
	Observatory      *Observatory      `json:"observatory,omitempty"`
	BurstObservatory *BurstObservatory `json:"burstObservatory,omitempty"`
}

type Log struct {
	Loglevel string `json:"loglevel"`
	Access   string `json:"access,omitempty"`
	Error    string `json:"error,omitempty"`
}

type API struct {
	Tag      string   `json:"tag"`
	Listen   string   `json:"listen,omitempty"`
	Services []string `json:"services"`
}

type Inbound struct {
	Tag            string          `json:"tag"`
	Listen         string          `json:"listen,omitempty"`
	Port           int             `json:"port"`
	Protocol       string          `json:"protocol"`
	Settings       json.RawMessage `json:"settings,omitempty"`
	Sniffing       *Sniffing       `json:"sniffing,omitempty"`
	StreamSettings *StreamSettings `json:"streamSettings,omitempty"`
}

type Sniffing struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"destOverride"`
	RouteOnly    bool     `json:"routeOnly,omitempty"`
}

type Routing struct {
	DomainStrategy string     `json:"domainStrategy,omitempty"`
	Rules          []Rule     `json:"rules"`
	Balancers      []Balancer `json:"balancers,omitempty"`
}

type Rule struct {
	Type        string   `json:"type"`
	InboundTag  []string `json:"inboundTag,omitempty"`
	OutboundTag string   `json:"outboundTag,omitempty"`
	BalancerTag string   `json:"balancerTag,omitempty"`
	IP          []string `json:"ip,omitempty"`
	Domain      []string `json:"domain,omitempty"`
}

type Balancer struct {
	Tag         string   `json:"tag"`
	Selector    []string `json:"selector"`
	Strategy    Strategy `json:"strategy"`
	FallbackTag string   `json:"fallbackTag,omitempty"`
}

type Strategy struct {
	Type string `json:"type"` // leastPing | leastLoad | roundRobin | random
}

type Observatory struct {
	SubjectSelector   []string `json:"subjectSelector"`
	ProbeURL          string   `json:"probeURL"`
	ProbeInterval     string   `json:"probeInterval"`
	EnableConcurrency bool     `json:"enableConcurrency"`
}

type BurstObservatory struct {
	SubjectSelector []string   `json:"subjectSelector"`
	PingConfig      PingConfig `json:"pingConfig"`
}

type PingConfig struct {
	Destination  string `json:"destination"`
	Interval     string `json:"interval"`
	Connectivity string `json:"connectivity,omitempty"`
	Timeout      string `json:"timeout"`
	Sampling     int    `json:"sampling"`
}

// Options controls config generation.
type Options struct {
	SocksPort       int
	HTTPPort        int
	EnableTProxy    bool
	TProxyPort      int
	EnableBalancer  bool
	ProbeURL        string
	PingDestination string
	APIPort         int
	TagPrefix       string
	LogLevel        string

	// LogError / LogAccess, when set, direct Xray to write its error (warning/
	// info/debug) and access logs to these files instead of stdout. keen-manager
	// points LogError at a known path so it can tail the tunnel's OWN failure
	// reason (a dial reset, a REALITY mismatch, an i/o timeout) and surface it in
	// the activation error — otherwise a failed bring-up only ever reports the
	// generic "the tunnel did not carry traffic". Empty leaves Xray on stdout.
	LogError  string
	LogAccess string

	// TCPMaxSeg clamps the MSS on every server outbound socket (see
	// Sockopt.TCPMaxSeg). 0 leaves it unset. Applied in both TPROXY and
	// proxy-connection modes because the egress to the server is router-local in
	// either case.
	TCPMaxSeg int

	// SplitDomains / SplitSubnets, when either is non-empty, switch the config
	// to per-service ("split tunnel") routing: only traffic whose destination
	// matches one of these domains (or subnets) is sent through the server
	// outbound; everything else is routed direct (freedom). This is how the
	// Routes feature sends selected services through an Xray connection while the
	// rest of the LAN egresses normally. When both are empty the config is a full
	// tunnel — all captured traffic goes through the server.
	SplitDomains []string
	SplitSubnets []string

	// ProxyConnMode builds the minimal SOCKS-only profile used when Xray is
	// wired to the router as a single KeeneticOS "Proxy connection" (interface
	// type Proxy → 127.0.0.1:<SocksPort>) rather than captured via TPROXY. In
	// this mode the config is just: the local SOCKS inbound + the single active
	// server outbound (+ direct/block). There is NO tproxy inbound, NO
	// observatory/balancer, and NO in-Xray split routing — per-service selection
	// is done by the router's own dns-proxy route bound to the Proxy interface
	// (exactly like native AWG), so a full tunnel here is correct. EnableTProxy,
	// EnableBalancer and Split* are all ignored when ProxyConnMode is set.
	ProxyConnMode bool
}

// Defaults returns sane default options.
func Defaults() Options {
	return Options{
		SocksPort:       10808,
		TProxyPort:      12345,
		ProbeURL:        "http://www.gstatic.com/generate_204",
		PingDestination: "https://www.gstatic.com/generate_204",
		APIPort:         10085,
		TagPrefix:       "srv-",
		LogLevel:        "warning",
	}
}

// BuildConfig produces a full config. When opts.EnableBalancer is true it wires
// a burstObservatory + leastPing balancer over all servers so Xray continuously
// picks the lowest-latency working location and fails over automatically. When
// false, the first server is the default outbound (used for pinning one server).
func BuildConfig(servers []model.Server, opts Options) (*Config, error) {
	if len(servers) == 0 {
		return nil, fmt.Errorf("no servers")
	}
	if opts.SocksPort == 0 {
		opts = mergeDefaults(opts)
	}

	// Proxy-connection mode: emit the minimal SOCKS-only profile (see
	// Options.ProxyConnMode). The router owns routing via a dns-proxy route on
	// the Proxy interface, so we neither capture nor split here.
	if opts.ProxyConnMode {
		return buildProxyConnConfig(servers[0], opts)
	}

	// The balancer (and its observatory) only exist with more than one server.
	useBalancer := opts.EnableBalancer && len(servers) > 1

	// api.services must only list services whose backing feature is present, or
	// xray-core aborts startup with "not all dependencies are resolved".
	// ObservatoryService depends on an observatory, which we emit ONLY in
	// balancer mode — so a pinned single-server config must not advertise it.
	apiServices := []string{"HandlerService", "StatsService", "RoutingService"}
	if useBalancer {
		apiServices = append(apiServices, "ObservatoryService")
	}

	cfg := &Config{
		Log:   &Log{Loglevel: def(opts.LogLevel, "warning"), Error: opts.LogError, Access: opts.LogAccess},
		Stats: &struct{}{},
		API: &API{
			Tag:      "api",
			Listen:   fmt.Sprintf("127.0.0.1:%d", opts.APIPort),
			Services: apiServices,
		},
	}

	// Inbounds: local SOCKS (for probing + LAN proxy), optional HTTP, optional tproxy.
	socksSettings, _ := json.Marshal(map[string]any{"auth": "noauth", "udp": true})
	cfg.Inbounds = append(cfg.Inbounds, Inbound{
		Tag:      "socks-in",
		Listen:   "127.0.0.1",
		Port:     opts.SocksPort,
		Protocol: "socks",
		Settings: socksSettings,
		Sniffing: &Sniffing{Enabled: true, DestOverride: []string{"http", "tls", "quic"}, RouteOnly: true},
	})
	if opts.HTTPPort > 0 {
		cfg.Inbounds = append(cfg.Inbounds, Inbound{
			Tag: "http-in", Listen: "127.0.0.1", Port: opts.HTTPPort, Protocol: "http",
		})
	}
	if opts.EnableTProxy {
		dd, _ := json.Marshal(map[string]any{"network": "tcp,udp", "followRedirect": true})
		cfg.Inbounds = append(cfg.Inbounds, Inbound{
			Tag: "tproxy-in", Port: opts.TProxyPort, Protocol: "dokodemo-door",
			Settings: dd,
			// routeOnly:true matches the XKeen canon (and our own socks-in): the
			// sniffed domain is used for routing decisions only — the connection
			// still goes to the original captured destination IP, never a
			// re-resolved address. Without it a transparent inbound can rewrite
			// the dial target from the sniff, which is both unnecessary here (the
			// real IP is already known) and a reliability hazard.
			Sniffing:       &Sniffing{Enabled: true, DestOverride: []string{"http", "tls", "quic"}, RouteOnly: true},
			StreamSettings: &StreamSettings{Sockopt: &Sockopt{TProxy: "tproxy"}},
		})
	}

	// Outbounds: one per server, then direct + block.
	inboundTags := []string{"socks-in"}
	if opts.HTTPPort > 0 {
		inboundTags = append(inboundTags, "http-in")
	}
	if opts.EnableTProxy {
		inboundTags = append(inboundTags, "tproxy-in")
	}

	var tags []string
	for _, s := range servers {
		tag := opts.TagPrefix + s.ID
		ob, err := OutboundFor(s, tag)
		if err != nil {
			return nil, fmt.Errorf("server %s: %w", s.Name, err)
		}
		applyMSSClamp(ob, opts.TCPMaxSeg)
		cfg.Outbounds = append(cfg.Outbounds, *ob)
		tags = append(tags, tag)
	}
	cfg.Outbounds = append(cfg.Outbounds,
		Outbound{Tag: "direct", Protocol: "freedom"},
		Outbound{Tag: "block", Protocol: "blackhole"},
	)

	// Routing
	cfg.Routing = &Routing{DomainStrategy: "IPIfNonMatch"}
	// API traffic to api outbound.
	cfg.Routing.Rules = append(cfg.Routing.Rules, Rule{Type: "field", InboundTag: []string{"api"}, OutboundTag: "api"})

	// Per-service split routing: when a domain/subnet set is supplied, only that
	// traffic goes through the tunnel (balancer or pinned server) and everything
	// else falls through to a direct catch-all. Order matters — Xray evaluates
	// rules top-down, so the matched rule precedes the direct catch-all.
	splitDomains := xrayDomainRules(opts.SplitDomains)
	splitSubnets := trimAll(opts.SplitSubnets)
	split := len(splitDomains) > 0 || len(splitSubnets) > 0

	if useBalancer {
		cfg.Routing.Balancers = []Balancer{{
			Tag:         "auto",
			Selector:    []string{opts.TagPrefix},
			Strategy:    Strategy{Type: "leastPing"},
			FallbackTag: tags[0],
		}}
		tunnel := Rule{Type: "field", InboundTag: inboundTags, BalancerTag: "auto"}
		if split {
			tunnel.Domain = splitDomains
			tunnel.IP = splitSubnets
			cfg.Routing.Rules = append(cfg.Routing.Rules, tunnel)
			cfg.Routing.Rules = append(cfg.Routing.Rules, Rule{Type: "field", InboundTag: inboundTags, OutboundTag: "direct"})
		} else {
			cfg.Routing.Rules = append(cfg.Routing.Rules, tunnel)
		}
		cfg.BurstObservatory = &BurstObservatory{
			SubjectSelector: []string{opts.TagPrefix},
			PingConfig: PingConfig{
				Destination: opts.PingDestination,
				Interval:    "5m",
				Timeout:     "5s",
				Sampling:    5,
			},
		}
	} else {
		// Pin to the first server (default outbound).
		tunnel := Rule{Type: "field", InboundTag: inboundTags, OutboundTag: tags[0]}
		if split {
			tunnel.Domain = splitDomains
			tunnel.IP = splitSubnets
			cfg.Routing.Rules = append(cfg.Routing.Rules, tunnel)
			cfg.Routing.Rules = append(cfg.Routing.Rules, Rule{Type: "field", InboundTag: inboundTags, OutboundTag: "direct"})
		} else {
			cfg.Routing.Rules = append(cfg.Routing.Rules, tunnel)
		}
	}

	return cfg, nil
}

// buildProxyConnConfig produces the minimal SOCKS-only config for
// proxy-connection mode (Options.ProxyConnMode): a single local SOCKS inbound
// forwarding a single pinned server outbound, plus direct/block. No tproxy
// inbound, no api/stats/observatory and no split rules — the router routes to
// the Proxy interface, and switching server just rewrites this file under the
// hood while the KeeneticOS Proxy interface stays put.
func buildProxyConnConfig(server model.Server, opts Options) (*Config, error) {
	tag := opts.TagPrefix + server.ID
	ob, err := OutboundFor(server, tag)
	if err != nil {
		return nil, fmt.Errorf("server %s: %w", server.Name, err)
	}
	applyMSSClamp(ob, opts.TCPMaxSeg)

	socksSettings, _ := json.Marshal(map[string]any{"auth": "noauth", "udp": true})
	cfg := &Config{
		Log: &Log{Loglevel: def(opts.LogLevel, "warning"), Error: opts.LogError, Access: opts.LogAccess},
		Inbounds: []Inbound{{
			Tag:      "socks-in",
			Listen:   "127.0.0.1",
			Port:     opts.SocksPort,
			Protocol: "socks",
			Settings: socksSettings,
			Sniffing: &Sniffing{Enabled: true, DestOverride: []string{"http", "tls", "quic"}, RouteOnly: true},
		}},
		Outbounds: []Outbound{
			*ob,
			{Tag: "direct", Protocol: "freedom"},
			{Tag: "block", Protocol: "blackhole"},
		},
		Routing: &Routing{
			DomainStrategy: "IPIfNonMatch",
			Rules: []Rule{
				{Type: "field", InboundTag: []string{"socks-in"}, OutboundTag: tag},
			},
		},
	}
	return cfg, nil
}

// applyMSSClamp sets TCP_MAXSEG on a server outbound's sockopt when mss > 0,
// creating the sockopt block if the outbound didn't already have one. A
// non-positive value is a no-op (leaves the MSS unclamped). See
// Sockopt.TCPMaxSeg for why this matters on a router.
func applyMSSClamp(ob *Outbound, mss int) {
	if ob == nil || mss <= 0 {
		return
	}
	if ob.StreamSettings == nil {
		ob.StreamSettings = &StreamSettings{}
	}
	if ob.StreamSettings.Sockopt == nil {
		ob.StreamSettings.Sockopt = &Sockopt{}
	}
	ob.StreamSettings.Sockopt.TCPMaxSeg = mss
}

// xrayDomainRules maps plain domain names onto Xray's "domain:" matcher (which
// matches the domain and all its subdomains). Entries that already carry an
// Xray matcher prefix (domain:/full:/keyword:/regexp:/geosite:) are passed
// through unchanged, so hand-written matchers still work.
func xrayDomainRules(domains []string) []string {
	if len(domains) == 0 {
		return nil
	}
	out := make([]string, 0, len(domains))
	for _, d := range domains {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if hasXrayMatcherPrefix(d) {
			out = append(out, d)
		} else {
			out = append(out, "domain:"+strings.ToLower(d))
		}
	}
	return out
}

func hasXrayMatcherPrefix(d string) bool {
	for _, p := range []string{"domain:", "full:", "keyword:", "regexp:", "geosite:", "ext:"} {
		if strings.HasPrefix(d, p) {
			return true
		}
	}
	return false
}

func trimAll(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Marshal renders the config as indented JSON.
func Marshal(cfg *Config) ([]byte, error) {
	return json.MarshalIndent(cfg, "", "  ")
}

func mergeDefaults(o Options) Options {
	d := Defaults()
	if o.SocksPort == 0 {
		o.SocksPort = d.SocksPort
	}
	if o.TProxyPort == 0 {
		o.TProxyPort = d.TProxyPort
	}
	if o.ProbeURL == "" {
		o.ProbeURL = d.ProbeURL
	}
	if o.PingDestination == "" {
		o.PingDestination = d.PingDestination
	}
	if o.APIPort == 0 {
		o.APIPort = d.APIPort
	}
	if o.TagPrefix == "" {
		o.TagPrefix = d.TagPrefix
	}
	if o.LogLevel == "" {
		o.LogLevel = d.LogLevel
	}
	return o
}
