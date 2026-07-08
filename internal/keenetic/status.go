package keenetic

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
)

// neverHandshakeSentinel is the last-handshake value RCI reports for a peer
// that has never completed a handshake. KeeneticOS represents "unknown/never"
// using the largest signed 32-bit int (matching the C `INT32_MAX` sentinel
// used internally by NDMS's wireguard status structures), rather than a
// negative number or a null field.
const neverHandshakeSentinel = math.MaxInt32

// PeerStatus is the parsed live state of one WireGuard/AWG peer.
type PeerStatus struct {
	PublicKey string
	// Online is true when the peer has a recent handshake (i.e. its
	// last-handshake age is not the "never" sentinel). This package does not
	// impose its own staleness window on top of that -- callers that want a
	// tighter "still actively passing traffic" threshold should compare
	// LastHandshakeAgeS themselves.
	Online bool
	// LastHandshakeAgeS is seconds since the peer's last handshake. It is 0
	// when Online is false (i.e. when RCI reported the "never" sentinel);
	// check Online, not this field, to distinguish "never" from "just now".
	LastHandshakeAgeS int64
}

// InterfaceStatusResult is the parsed live state of a native Wireguard/AWG
// interface, as reported by "GET /show/interface/{name}" and returned by the
// InterfaceStatus function below.
type InterfaceStatusResult struct {
	Name  string
	Up    bool
	Peers []PeerStatus
}

// interfaceStatusResponse mirrors the subset of "GET /show/interface/{name}"
// fields this package reads. RCI's wireguard peer list has been observed
// under both "peer" (singular key, plural value) and "peers"; both are
// decoded into the same field via peerListResponse's UnmarshalJSON.
type interfaceStatusResponse struct {
	Up        bool `json:"up"`
	Wireguard struct {
		Peer peerListResponse `json:"peer"`
	} `json:"wireguard"`
}

// peerEntry is one element of the wireguard peer list.
type peerEntry struct {
	Key           string `json:"key"`
	Online        *bool  `json:"online"`
	LastHandshake *int64 `json:"last-handshake"`
}

// peerListResponse decodes RCI's peer list, which firmware has been observed
// to represent either as a JSON array (multiple peers) or, as an RCI-wide
// quirk, as a single object when there is exactly one peer.
type peerListResponse []peerEntry

func (p *peerListResponse) UnmarshalJSON(data []byte) error {
	trimmed := trimLeadingSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		*p = nil
		return nil
	}
	if trimmed[0] == '[' {
		var arr []peerEntry
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		*p = arr
		return nil
	}
	var single peerEntry
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*p = peerListResponse{single}
	return nil
}

func trimLeadingSpace(b []byte) []byte {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	return b[i:]
}

// InterfaceStatus reads "GET /show/interface/{name}" and extracts the
// interface's up state plus each peer's key/online/last-handshake.
//
// A last-handshake value equal to the "never" sentinel (see
// neverHandshakeSentinel) is reported as Online=false with
// LastHandshakeAgeS=0, matching a peer that has never completed a handshake.
func InterfaceStatus(ctx context.Context, c *Client, name string) (InterfaceStatusResult, error) {
	raw, err := c.Get(ctx, "/show/interface/"+name)
	if err != nil {
		return InterfaceStatusResult{}, fmt.Errorf("keenetic: interface status %s: %w", name, err)
	}

	var resp interfaceStatusResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return InterfaceStatusResult{}, fmt.Errorf("keenetic: decode interface status %s: %w", name, err)
	}

	st := InterfaceStatusResult{Name: name, Up: resp.Up}
	for _, p := range resp.Wireguard.Peer {
		ps := PeerStatus{PublicKey: p.Key}

		age := int64(0)
		if p.LastHandshake != nil {
			age = *p.LastHandshake
		}
		never := age == neverHandshakeSentinel
		switch {
		case p.Online != nil:
			ps.Online = *p.Online && !never
		default:
			ps.Online = !never
		}
		if !never {
			ps.LastHandshakeAgeS = age
		}

		st.Peers = append(st.Peers, ps)
	}
	return st, nil
}

// pingCheckInterfaceResponse mirrors "GET /show/interface/{name}" 's
// ping-check block.
type pingCheckInterfaceResponse struct {
	PingCheck *struct {
		Status string `json:"status"`
		Fails  *int   `json:"fails"`
	} `json:"ping-check"`
}

// PingCheck reads the Keenetic-native ping-check status for interface name,
// if one is bound (used as a liveness signal independent of the WireGuard
// handshake timer). It is deliberately best-effort: many AWG interfaces have
// no ping-check profile configured at all, and transport errors are just as
// uninformative to a caller polling status on a timer, so both cases degrade
// to a zero status/failCount rather than propagating an error. Callers that
// need to distinguish "no profile bound" from "transport error" should call
// Client.Get("/show/interface/"+name) directly.
func PingCheck(ctx context.Context, c *Client, name string) (status string, failCount int) {
	raw, err := c.Get(ctx, "/show/interface/"+name)
	if err != nil {
		return "", 0
	}

	var resp pingCheckInterfaceResponse
	if err := json.Unmarshal(raw, &resp); err != nil || resp.PingCheck == nil {
		return "", 0
	}

	fails := 0
	if resp.PingCheck.Fails != nil {
		fails = *resp.PingCheck.Fails
	}
	return resp.PingCheck.Status, fails
}
