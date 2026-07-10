// Package xray generates Xray-core configuration from parsed servers and manages
// the Xray process. Config generation is pure and unit-tested.
package xray

import (
	"encoding/json"
	"fmt"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// selfMark is set on every outbound socket (SO_MARK) so Xray's own egress is not
// re-captured by the tproxy rules. Matches the XKeen convention.
const selfMark = 255

// Outbound is one Xray outbound.
type Outbound struct {
	Tag            string          `json:"tag"`
	Protocol       string          `json:"protocol"`
	Settings       json.RawMessage `json:"settings,omitempty"`
	StreamSettings *StreamSettings `json:"streamSettings,omitempty"`
}

// StreamSettings is the transport/security block of an outbound.
type StreamSettings struct {
	Network         string           `json:"network,omitempty"`
	Security        string           `json:"security,omitempty"`
	RealitySettings *RealitySettings `json:"realitySettings,omitempty"`
	TLSSettings     *TLSSettings     `json:"tlsSettings,omitempty"`
	WSSettings      *WSSettings      `json:"wsSettings,omitempty"`
	GRPCSettings    *GRPCSettings    `json:"grpcSettings,omitempty"`
	Sockopt         *Sockopt         `json:"sockopt,omitempty"`
}

type RealitySettings struct {
	ServerName  string `json:"serverName,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	PublicKey   string `json:"publicKey"`
	ShortID     string `json:"shortId,omitempty"`
	SpiderX     string `json:"spiderX,omitempty"`
}

type TLSSettings struct {
	ServerName    string   `json:"serverName,omitempty"`
	Fingerprint   string   `json:"fingerprint,omitempty"`
	ALPN          []string `json:"alpn,omitempty"`
	AllowInsecure bool     `json:"allowInsecure,omitempty"`
}

type WSSettings struct {
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type GRPCSettings struct {
	ServiceName string `json:"serviceName,omitempty"`
}

type Sockopt struct {
	Mark   int    `json:"mark,omitempty"`
	TProxy string `json:"tproxy,omitempty"`
	// TCPMaxSeg sets TCP_MAXSEG (the JSON key is Xray's `tcpMaxSeg`) on the
	// outbound socket, clamping the MSS the router advertises to the server. It
	// is the fix for the "reality handshake establishes but no payload flows"
	// class of bug on routers: Xray's egress to the server is a router-LOCAL
	// socket (OUTPUT chain), which — unlike LAN traffic FORWARDED through the
	// router — is not MSS-clamped-to-PMTU by KeeneticOS, so on a reduced-MTU or
	// TSPU-throttled WAN the small handshake packets get through while full-size
	// data segments blackhole. Clamping the MSS here makes the server send
	// segments that fit the real path MTU. 0 omits the field (no clamp).
	TCPMaxSeg int `json:"tcpMaxSeg,omitempty"`
}

// OutboundFor converts a Server into an Xray outbound with the given tag.
func OutboundFor(s model.Server, tag string) (*Outbound, error) {
	ob := &Outbound{Tag: tag, Protocol: string(s.Protocol)}
	ss := &StreamSettings{
		Network:  def(s.Network, "tcp"),
		Security: s.Security,
		Sockopt:  &Sockopt{Mark: selfMark},
	}

	// security block
	switch s.Security {
	case "reality":
		if s.PublicKey == "" {
			return nil, fmt.Errorf("%s: reality requires publicKey (pbk)", s.Name)
		}
		ss.RealitySettings = &RealitySettings{
			ServerName:  s.SNI,
			Fingerprint: def(s.Fingerprint, "chrome"),
			PublicKey:   s.PublicKey,
			ShortID:     s.ShortID,
			SpiderX:     def(s.SpiderX, "/"),
		}
	case "tls":
		ss.TLSSettings = &TLSSettings{
			ServerName:    s.SNI,
			Fingerprint:   s.Fingerprint,
			ALPN:          s.ALPN,
			AllowInsecure: s.AllowInsecure,
		}
	case "", "none":
		ss.Security = "none"
	}

	// transport block
	switch ss.Network {
	case "ws", "httpupgrade":
		w := &WSSettings{Path: def(s.Path, "/")}
		if s.Host != "" {
			w.Headers = map[string]string{"Host": s.Host}
		}
		ss.WSSettings = w
	case "grpc":
		ss.GRPCSettings = &GRPCSettings{ServiceName: s.Path}
	}

	// protocol settings
	var settings any
	switch s.Protocol {
	case model.ProtoVLESS:
		settings = map[string]any{
			"vnext": []map[string]any{{
				"address": s.Address,
				"port":    s.Port,
				"users": []map[string]any{{
					"id":         s.UUID,
					"encryption": def(protoField(s.Cipher), "none"),
					"flow":       s.Flow,
				}},
			}},
		}
	case model.ProtoVMess:
		settings = map[string]any{
			"vnext": []map[string]any{{
				"address": s.Address,
				"port":    s.Port,
				"users": []map[string]any{{
					"id":       s.UUID,
					"alterId":  s.AlterID,
					"security": def(s.Cipher, "auto"),
				}},
			}},
		}
	case model.ProtoTrojan:
		settings = map[string]any{
			"servers": []map[string]any{{
				"address":  s.Address,
				"port":     s.Port,
				"password": s.Password,
			}},
		}
	case model.ProtoSS:
		settings = map[string]any{
			"servers": []map[string]any{{
				"address":  s.Address,
				"port":     s.Port,
				"method":   s.Cipher,
				"password": s.Password,
				"uot":      true,
			}},
		}
	default:
		return nil, fmt.Errorf("unsupported protocol %q", s.Protocol)
	}

	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	ob.Settings = raw
	ob.StreamSettings = ss
	return ob, nil
}

// protoField normalizes vless encryption (empty for vless means "none").
func protoField(v string) string {
	if v == "auto" { // "auto" is a vmess cipher, not valid for vless encryption
		return "none"
	}
	return v
}

func def(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
