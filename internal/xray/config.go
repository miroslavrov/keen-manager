package xray

import (
	"encoding/json"
	"fmt"

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

	cfg := &Config{
		Log:   &Log{Loglevel: def(opts.LogLevel, "warning")},
		Stats: &struct{}{},
		API: &API{
			Tag:      "api",
			Listen:   fmt.Sprintf("127.0.0.1:%d", opts.APIPort),
			Services: []string{"HandlerService", "StatsService", "RoutingService", "ObservatoryService"},
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
			Settings:       dd,
			Sniffing:       &Sniffing{Enabled: true, DestOverride: []string{"http", "tls", "quic"}},
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

	if opts.EnableBalancer && len(tags) > 1 {
		cfg.Routing.Balancers = []Balancer{{
			Tag:         "auto",
			Selector:    []string{opts.TagPrefix},
			Strategy:    Strategy{Type: "leastPing"},
			FallbackTag: tags[0],
		}}
		cfg.Routing.Rules = append(cfg.Routing.Rules, Rule{
			Type: "field", InboundTag: inboundTags, BalancerTag: "auto",
		})
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
		cfg.Routing.Rules = append(cfg.Routing.Rules, Rule{
			Type: "field", InboundTag: inboundTags, OutboundTag: tags[0],
		})
	}

	return cfg, nil
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
