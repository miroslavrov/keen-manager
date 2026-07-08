package keenetic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// newTestClient wires a Client at srv's URL, using "/rci" as the RCI root so
// Post/Get exercise the same path-joining logic as production (BaseURL is
// never just the bare httptest origin).
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *capturingServer) {
	t.Helper()
	cs := &capturingServer{t: t, handler: handler}
	srv := httptest.NewServer(http.HandlerFunc(cs.serve))
	t.Cleanup(srv.Close)
	return New(srv.URL + "/rci"), cs
}

// capturedRequest is one recorded request/body pair.
type capturedRequest struct {
	method string
	path   string
	body   []byte
}

// capturingServer records every request's method/path/body (in order) so
// tests can assert on the exact JSON payload the package under test sent,
// even for calls (like SetASC) that issue more than one HTTP request.
type capturingServer struct {
	t        *testing.T
	handler  http.HandlerFunc
	requests []capturedRequest

	// lastPath/lastMethod mirror the most recent request for tests that only
	// care about a single call.
	lastPath   string
	lastMethod string
	lastBody   []byte
}

func (c *capturingServer) serve(w http.ResponseWriter, r *http.Request) {
	body := make([]byte, 0)
	if r.Body != nil {
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			if n > 0 {
				body = append(body, buf[:n]...)
			}
			if err != nil {
				break
			}
		}
	}
	c.requests = append(c.requests, capturedRequest{method: r.Method, path: r.URL.Path, body: body})
	c.lastPath = r.URL.Path
	c.lastMethod = r.Method
	c.lastBody = body
	c.handler(w, r)
}

// lastBodyJSON unmarshals the last captured request body into a generic map
// for structural assertions.
func (c *capturingServer) lastBodyJSON(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(c.lastBody, &m); err != nil {
		t.Fatalf("captured body is not a JSON object: %v\nbody: %s", err, c.lastBody)
	}
	return m
}

// lastBodyJSONForMethod finds the most recent captured request with the given
// HTTP method and unmarshals its body into a generic map. Used for call
// sequences (like SetASC) that mix a POST write with a GET readback, where a
// test wants to assert on the POST body specifically regardless of ordering.
func (c *capturingServer) lastBodyJSONForMethod(t *testing.T, method string) map[string]any {
	t.Helper()
	for i := len(c.requests) - 1; i >= 0; i-- {
		if c.requests[i].method == method {
			var m map[string]any
			if err := json.Unmarshal(c.requests[i].body, &m); err != nil {
				t.Fatalf("captured %s body is not a JSON object: %v\nbody: %s", method, err, c.requests[i].body)
			}
			return m
		}
	}
	t.Fatalf("no captured request with method %s (saw %d requests)", method, len(c.requests))
	return nil
}

// dig walks a chain of map keys, failing the test if any hop is missing or
// not a map[string]any. The last key's raw value is returned.
func dig(t *testing.T, m map[string]any, keys ...string) any {
	t.Helper()
	var cur any = m
	for i, k := range keys {
		asMap, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("dig: at key %q (index %d), expected map[string]any, got %T (path so far: %v)", k, i, cur, keys[:i])
		}
		v, ok := asMap[k]
		if !ok {
			t.Fatalf("dig: key %q not found (path: %v); available keys: %v", k, keys[:i+1], mapKeys(asMap))
		}
		cur = v
	}
	return cur
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// --- Post: NDMS-200-on-error envelope detection -----------------------------

func TestPost_ErrorEnvelope_ReturnsError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // NDMS: errors still come back as 200
		_, _ = w.Write([]byte(`{
			"interface": {
				"status": [
					{"status": "error", "code": "6553603", "ident": "Network::Interface::Repository", "message": "\"BadName\": invalid interface name."}
				]
			}
		}`))
	})

	_, err := c.Post(context.Background(), map[string]any{"interface": map[string]any{"BadName": map[string]any{"up": true}}})
	if err == nil {
		t.Fatal("expected an error for an NDMS error envelope on HTTP 200, got nil")
	}
	var rciErr *Error
	if !errors.As(err, &rciErr) {
		t.Fatalf("expected error to be a *keenetic.Error, got %T: %v", err, err)
	}
	if rciErr.Ident != "Network::Interface::Repository" {
		t.Errorf("Ident = %q, want Network::Interface::Repository", rciErr.Ident)
	}
	if rciErr.Code != "6553603" {
		t.Errorf("Code = %q, want 6553603", rciErr.Code)
	}
	if !strings.Contains(rciErr.Message, "invalid interface name") {
		t.Errorf("Message = %q, want it to contain 'invalid interface name'", rciErr.Message)
	}
}

func TestPost_OkEnvelope_ReturnsNilError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"interface": {
				"status": [
					{"status": "message", "code": "6553601", "ident": "Network::Interface::Repository", "message": "\"Wireguard0\" interface created."}
				]
			}
		}`))
	})

	raw, err := c.Post(context.Background(), map[string]any{"interface": map[string]any{"Wireguard0": map[string]any{"up": true}}})
	if err != nil {
		t.Fatalf("expected nil error for a message-only status envelope, got: %v", err)
	}
	if len(raw) == 0 {
		t.Error("expected non-empty raw response body")
	}
}

func TestPost_WarningEnvelope_ReturnsNilError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":[{"status":"warning","code":"1","ident":"X","message":"already up"}]}`))
	})
	if _, err := c.Post(context.Background(), map[string]any{"interface": map[string]any{"Wireguard0": map[string]any{"up": true}}}); err != nil {
		t.Fatalf("warning-level status must not be treated as an error, got: %v", err)
	}
}

func TestPost_NestedErrorEnvelope_Detected(t *testing.T) {
	// The error can be nested arbitrarily deep (e.g. under the wireguard
	// sub-resource rather than at the top level).
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"interface": {
				"Wireguard0": {
					"wireguard": {
						"asc": {
							"status": [
								{"status": "error", "code": "1", "ident": "Wireguard::Asc", "message": "s3 not supported"}
							]
						}
					}
				}
			}
		}`))
	})
	_, err := c.Post(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected nested error envelope to be detected")
	}
	if !strings.Contains(err.Error(), "s3 not supported") {
		t.Errorf("error = %v, want it to mention 's3 not supported'", err)
	}
}

func TestPost_TransportLevelNon2xx_ReturnsError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`Unauthorized`))
	})
	if _, err := c.Post(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected an error for a non-2xx HTTP status")
	}
}

// --- Save --------------------------------------------------------------------

func TestSave_PayloadShape(t *testing.T) {
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := c.Save(context.Background()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if cs.lastMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", cs.lastMethod)
	}
	if cs.lastPath != "/rci/" {
		t.Errorf("path = %s, want /rci/", cs.lastPath)
	}
	body := cs.lastBodyJSON(t)
	if _, ok := dig(t, body, "system", "configuration", "save").(map[string]any); !ok {
		t.Errorf("expected system.configuration.save to be an empty object, body=%v", body)
	}
}

// --- Get / GET path construction ---------------------------------------------

func TestGet_PathConstruction(t *testing.T) {
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"release":"5.02"}`))
	})
	if _, err := c.Get(context.Background(), "/show/version"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cs.lastMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", cs.lastMethod)
	}
	if cs.lastPath != "/rci/show/version" {
		t.Errorf("path = %s, want /rci/show/version", cs.lastPath)
	}
}

// --- Parse ---------------------------------------------------------------

func TestParse_PayloadShape(t *testing.T) {
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	if _, err := c.Parse(context.Background(), "show version"); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	body := cs.lastBodyJSON(t)
	if got, _ := body["parse"].(string); got != "show version" {
		t.Errorf(`body["parse"] = %q, want "show version"`, got)
	}
}

// --- CreateInterface -----------------------------------------------------

func TestCreateInterface_PayloadShape(t *testing.T) {
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	cfg := InterfaceConfig{
		Description: "keen-manager tunnel",
		ListenPort:  51820,
		Address:     "10.10.0.1",
		Mask:        "255.255.255.0",
		MTU:         1420,
		Up:          true,
	}
	if err := CreateInterface(context.Background(), c, "Wireguard3", cfg); err != nil {
		t.Fatalf("CreateInterface: %v", err)
	}

	body := cs.lastBodyJSON(t)
	iface, ok := dig(t, body, "interface", "Wireguard3").(map[string]any)
	if !ok {
		t.Fatalf("interface.Wireguard3 missing or wrong type, body=%v", body)
	}
	if iface["description"] != "keen-manager tunnel" {
		t.Errorf("description = %v, want %q", iface["description"], "keen-manager tunnel")
	}
	if lp, _ := iface["listen-port"].(float64); int(lp) != 51820 {
		t.Errorf("listen-port = %v, want 51820", iface["listen-port"])
	}
	ip, ok := iface["ip"].(map[string]any)
	if !ok {
		t.Fatalf("ip missing or wrong type: %v", iface["ip"])
	}
	if ip["address"] != "10.10.0.1" {
		t.Errorf("ip.address = %v, want 10.10.0.1", ip["address"])
	}
	if ip["mask"] != "255.255.255.0" {
		t.Errorf("ip.mask = %v, want 255.255.255.0", ip["mask"])
	}
	if mtu, _ := iface["mtu"].(float64); int(mtu) != 1420 {
		t.Errorf("mtu = %v, want 1420", iface["mtu"])
	}
	if up, _ := iface["up"].(bool); !up {
		t.Errorf("up = %v, want true", iface["up"])
	}
}

// --- DeleteInterface -------------------------------------------------------

func TestDeleteInterface_PayloadShape(t *testing.T) {
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := DeleteInterface(context.Background(), c, "Wireguard3"); err != nil {
		t.Fatalf("DeleteInterface: %v", err)
	}
	body := cs.lastBodyJSON(t)
	iface, ok := dig(t, body, "interface", "Wireguard3").(map[string]any)
	if !ok {
		t.Fatalf("interface.Wireguard3 missing or wrong type, body=%v", body)
	}
	if no, _ := iface["no"].(bool); !no {
		t.Errorf(`expected interface.Wireguard3 = {"no": true}, got %v`, iface)
	}
}

// --- InterfaceUp / InterfaceDown -------------------------------------------

func TestInterfaceUpDown_PayloadShape(t *testing.T) {
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	if err := InterfaceUp(context.Background(), c, "Wireguard0"); err != nil {
		t.Fatalf("InterfaceUp: %v", err)
	}
	body := cs.lastBodyJSON(t)
	if up, _ := dig(t, body, "interface", "Wireguard0", "up").(bool); !up {
		t.Errorf("InterfaceUp: interface.Wireguard0.up = %v, want true", up)
	}

	if err := InterfaceDown(context.Background(), c, "Wireguard0"); err != nil {
		t.Fatalf("InterfaceDown: %v", err)
	}
	body = cs.lastBodyJSON(t)
	if up, ok := dig(t, body, "interface", "Wireguard0", "up").(bool); !ok || up {
		t.Errorf("InterfaceDown: interface.Wireguard0.up = %v, want false", up)
	}
}

// --- SetASC ----------------------------------------------------------------

func TestSetASC_PayloadShape_AWG1(t *testing.T) {
	callCount := 0
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			// Readback verification call.
			_, _ = w.Write([]byte(`{"jc":"5"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})

	p := ASCParams{Jc: 5, Jmin: 3, Jmax: 10, S1: 20, S2: 30, H1: "1", H2: "2", H3: "3", H4: "4"}
	caps := Capabilities{Release: "5.00.03", SupportsAWG2: false}

	if err := SetASC(context.Background(), c, "Wireguard0", p, caps); err != nil {
		t.Fatalf("SetASC: %v", err)
	}

	// SetASC issues a POST (to apply the asc) followed by one or more GET
	// readbacks (to verify it took); assert on the POST body specifically.
	body := cs.lastBodyJSONForMethod(t, http.MethodPost)
	asc, ok := dig(t, body, "interface", "Wireguard0", "wireguard", "asc").(map[string]any)
	if !ok {
		t.Fatalf("interface.Wireguard0.wireguard.asc missing or wrong type, body=%v", body)
	}
	wantStrs := map[string]string{"jc": "5", "jmin": "3", "jmax": "10", "s1": "20", "s2": "30", "h1": "1", "h2": "2", "h3": "3", "h4": "4"}
	for k, want := range wantStrs {
		got, _ := asc[k].(string)
		if got != want {
			t.Errorf("asc[%q] = %q, want %q", k, got, want)
		}
	}
	if _, present := asc["s3"]; present {
		t.Errorf("asc.s3 must be absent for an AWG1 request, got %v", asc["s3"])
	}
	if _, present := asc["s4"]; present {
		t.Errorf("asc.s4 must be absent for an AWG1 request, got %v", asc["s4"])
	}
	if callCount == 0 {
		t.Error("expected at least one request (POST + GET readback)")
	}
}

func TestSetASC_PayloadShape_AWG2(t *testing.T) {
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"jc":"7"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})

	s3, s4 := 40, 50
	p := ASCParams{Jc: 7, Jmin: 3, Jmax: 10, S1: 20, S2: 30, H1: "1", H2: "2", H3: "3", H4: "4", S3: &s3, S4: &s4}
	caps := Capabilities{Release: "5.01.A.3", SupportsAWG2: true}

	if err := SetASC(context.Background(), c, "Wireguard0", p, caps); err != nil {
		t.Fatalf("SetASC (AWG2): %v", err)
	}

	body := cs.lastBodyJSONForMethod(t, http.MethodPost)
	asc, ok := dig(t, body, "interface", "Wireguard0", "wireguard", "asc").(map[string]any)
	if !ok {
		t.Fatalf("interface.Wireguard0.wireguard.asc missing or wrong type, body=%v", body)
	}
	if asc["s3"] != "40" {
		t.Errorf("asc.s3 = %v, want \"40\"", asc["s3"])
	}
	if asc["s4"] != "50" {
		t.Errorf("asc.s4 = %v, want \"50\"", asc["s4"])
	}
}

func TestSetASC_AWG2FieldsOnOldFirmware_Errors(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("SetASC must not perform any RCI call when the capability gate rejects the request")
	})
	s3 := 40
	p := ASCParams{Jc: 5, S3: &s3}
	caps := Capabilities{Release: "5.00.03", SupportsAWG2: false}

	err := SetASC(context.Background(), c, "Wireguard0", p, caps)
	if err == nil {
		t.Fatal("expected an error when S3 is set but firmware does not support AWG2")
	}
	if !strings.Contains(err.Error(), "AWG2") {
		t.Errorf("error = %v, want it to mention AWG2", err)
	}
}

func TestSetASC_NeverVerifies_ReturnsError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			// Always report a stale value so verification never succeeds.
			_, _ = w.Write([]byte(`{"jc":"0"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	p := ASCParams{Jc: 9}
	caps := Capabilities{}
	err := SetASC(context.Background(), c, "Wireguard0", p, caps)
	if err == nil {
		t.Fatal("expected an error when the readback never confirms the applied jc value")
	}
}

// --- AddPeer / RemovePeer ----------------------------------------------------

func TestAddPeer_PayloadShape(t *testing.T) {
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	err := AddPeer(context.Background(), c, "Wireguard0", "PUBKEYBASE64==", "PSKBASE64==", []string{"10.0.0.2/32", "10.0.0.3"})
	if err != nil {
		t.Fatalf("AddPeer: %v", err)
	}

	body := cs.lastBodyJSON(t)
	peers, ok := dig(t, body, "interface", "Wireguard0", "wireguard", "peer").([]any)
	if !ok || len(peers) != 1 {
		t.Fatalf("expected exactly one peer entry, body=%v", body)
	}
	peer, ok := peers[0].(map[string]any)
	if !ok {
		t.Fatalf("peer entry is not an object: %v", peers[0])
	}
	if peer["key"] != "PUBKEYBASE64==" {
		t.Errorf("key = %v, want PUBKEYBASE64==", peer["key"])
	}
	if peer["preshared-key"] != "PSKBASE64==" {
		t.Errorf("preshared-key = %v, want PSKBASE64==", peer["preshared-key"])
	}
	if connect, _ := peer["connect"].(bool); !connect {
		t.Errorf("connect = %v, want true", peer["connect"])
	}
	allowed, ok := peer["allow-ips"].([]any)
	if !ok || len(allowed) != 2 {
		t.Fatalf("expected 2 allow-ips entries, got %v", peer["allow-ips"])
	}
	first, _ := allowed[0].(map[string]any)
	if first["address"] != "10.0.0.2" || first["mask"] != "255.255.255.255" {
		t.Errorf("allow-ips[0] = %v, want address=10.0.0.2 mask=255.255.255.255", first)
	}
	second, _ := allowed[1].(map[string]any)
	if second["address"] != "10.0.0.3" || second["mask"] != "255.255.255.255" {
		t.Errorf("allow-ips[1] (bare IP defaulting to /32) = %v, want address=10.0.0.3 mask=255.255.255.255", second)
	}
}

func TestAddPeer_NoAllowedIPs_OmitsField(t *testing.T) {
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := AddPeer(context.Background(), c, "Wireguard0", "KEY==", "", nil); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
	body := cs.lastBodyJSON(t)
	peer, _ := dig(t, body, "interface", "Wireguard0", "wireguard", "peer").([]any)
	peerObj, _ := peer[0].(map[string]any)
	if _, present := peerObj["allow-ips"]; present {
		t.Errorf("allow-ips should be omitted when none are given, got %v", peerObj["allow-ips"])
	}
	if _, present := peerObj["preshared-key"]; present {
		t.Errorf("preshared-key should be omitted when psk is empty, got %v", peerObj["preshared-key"])
	}
}

func TestRemovePeer_PayloadShape(t *testing.T) {
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := RemovePeer(context.Background(), c, "Wireguard0", "PUBKEYBASE64=="); err != nil {
		t.Fatalf("RemovePeer: %v", err)
	}
	body := cs.lastBodyJSON(t)
	peers, ok := dig(t, body, "interface", "Wireguard0", "wireguard", "peer").([]any)
	if !ok || len(peers) != 1 {
		t.Fatalf("expected exactly one peer entry, body=%v", body)
	}
	peer, _ := peers[0].(map[string]any)
	if peer["key"] != "PUBKEYBASE64==" {
		t.Errorf("key = %v, want PUBKEYBASE64==", peer["key"])
	}
	if no, _ := peer["no"].(bool); !no {
		t.Errorf(`expected {"key":..., "no": true}, got %v`, peer)
	}
}

// --- ASCFromAWGConfig --------------------------------------------------------

func TestASCFromAWGConfig(t *testing.T) {
	cfg := model.AWGConfig{
		Jc: 4, Jmin: 3, Jmax: 8, S1: 15, S2: 25,
		H1: 1234567890, H2: 2234567890, H3: 3234567890, H4: 4234567890,
		S3: 40, S4: 50,
	}

	t.Run("awg2 capable", func(t *testing.T) {
		p := ASCFromAWGConfig(cfg, Capabilities{SupportsAWG2: true})
		if p.Jc != 4 || p.Jmin != 3 || p.Jmax != 8 || p.S1 != 15 || p.S2 != 25 {
			t.Errorf("base fields mismatch: %+v", p)
		}
		if p.H1 != "1234567890" || p.H4 != "4234567890" {
			t.Errorf("H1/H4 mismatch: H1=%q H4=%q", p.H1, p.H4)
		}
		if p.S3 == nil || *p.S3 != 40 {
			t.Errorf("S3 = %v, want 40", p.S3)
		}
		if p.S4 == nil || *p.S4 != 50 {
			t.Errorf("S4 = %v, want 50", p.S4)
		}
	})

	t.Run("awg1 only firmware drops s3/s4", func(t *testing.T) {
		p := ASCFromAWGConfig(cfg, Capabilities{SupportsAWG2: false})
		if p.S3 != nil {
			t.Errorf("S3 = %v, want nil when firmware lacks AWG2 support", p.S3)
		}
		if p.S4 != nil {
			t.Errorf("S4 = %v, want nil when firmware lacks AWG2 support", p.S4)
		}
	})
}

// --- FindFreeIndex -----------------------------------------------------------

func TestFindFreeIndex_FreshDevice(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"GigabitEthernet0": {"up": true}, "Bridge0": {"up": true}}`))
	})
	n, err := FindFreeIndex(context.Background(), c)
	if err != nil {
		t.Fatalf("FindFreeIndex: %v", err)
	}
	if n != 0 {
		t.Errorf("n = %d, want 0 on a fresh device with no Wireguard interfaces", n)
	}
}

func TestFindFreeIndex_SkipsUsed(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"Wireguard0": {"up": true},
			"Wireguard1": {"up": true},
			"Wireguard3": {"up": false},
			"GigabitEthernet0": {"up": true}
		}`))
	})
	n, err := FindFreeIndex(context.Background(), c)
	if err != nil {
		t.Fatalf("FindFreeIndex: %v", err)
	}
	if n != 2 {
		t.Errorf("n = %d, want 2 (first gap in 0,1,3)", n)
	}
}

func TestFindFreeIndex_RequestsCorrectPath(t *testing.T) {
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	if _, err := FindFreeIndex(context.Background(), c); err != nil {
		t.Fatalf("FindFreeIndex: %v", err)
	}
	if cs.lastPath != "/rci/show/interface/" {
		t.Errorf("path = %s, want /rci/show/interface/", cs.lastPath)
	}
}

// --- InterfaceStatus / PeerStatus --------------------------------------------

func TestInterfaceStatus_ParsesPeersAndNeverSentinel(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"up": true,
			"wireguard": {
				"peer": [
					{"key": "AAA==", "online": true, "last-handshake": 42},
					{"key": "BBB==", "online": false, "last-handshake": 2147483647}
				]
			}
		}`))
	})
	st, err := InterfaceStatus(context.Background(), c, "Wireguard0")
	if err != nil {
		t.Fatalf("InterfaceStatus: %v", err)
	}
	if !st.Up {
		t.Error("Up = false, want true")
	}
	if len(st.Peers) != 2 {
		t.Fatalf("got %d peers, want 2", len(st.Peers))
	}
	if st.Peers[0].PublicKey != "AAA==" || !st.Peers[0].Online || st.Peers[0].LastHandshakeAgeS != 42 {
		t.Errorf("peer[0] = %+v", st.Peers[0])
	}
	if st.Peers[1].PublicKey != "BBB==" || st.Peers[1].Online || st.Peers[1].LastHandshakeAgeS != 0 {
		t.Errorf("peer[1] (never sentinel) = %+v, want Online=false LastHandshakeAgeS=0", st.Peers[1])
	}
}

func TestInterfaceStatus_SinglePeerObjectForm(t *testing.T) {
	// Some firmware collapses a one-element peer list to a bare object.
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"up": true, "wireguard": {"peer": {"key": "SOLO==", "online": true, "last-handshake": 5}}}`))
	})
	st, err := InterfaceStatus(context.Background(), c, "Wireguard0")
	if err != nil {
		t.Fatalf("InterfaceStatus: %v", err)
	}
	if len(st.Peers) != 1 || st.Peers[0].PublicKey != "SOLO==" {
		t.Errorf("Peers = %+v, want a single SOLO== peer", st.Peers)
	}
}

// --- PingCheck ----------------------------------------------------------

func TestPingCheck_Bound(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"up": true, "ping-check": {"status": "up", "fails": 0}}`))
	})
	status, fails := PingCheck(context.Background(), c, "Wireguard0")
	if status != "up" || fails != 0 {
		t.Errorf("status=%q fails=%d, want up/0", status, fails)
	}
}

func TestPingCheck_NotBound(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"up": true}`))
	})
	status, fails := PingCheck(context.Background(), c, "Wireguard0")
	if status != "" || fails != 0 {
		t.Errorf("status=%q fails=%d, want empty/0 when no ping-check is bound", status, fails)
	}
}

// --- DetectCapabilities -------------------------------------------------

func TestDetectCapabilities(t *testing.T) {
	c, cs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"release": "5.01.A.3", "title": "KeeneticOS", "ndw": {"components": "wireguard,amneziawg,schedule"}}`))
	})
	caps, err := DetectCapabilities(context.Background(), c)
	if err != nil {
		t.Fatalf("DetectCapabilities: %v", err)
	}
	if cs.lastPath != "/rci/show/version" {
		t.Errorf("path = %s, want /rci/show/version", cs.lastPath)
	}
	if caps.Release != "5.01.A.3" {
		t.Errorf("Release = %q, want 5.01.A.3", caps.Release)
	}
	if !caps.SupportsAWG2 {
		t.Error("SupportsAWG2 = false, want true for 5.01.A.3")
	}
	if !caps.HasWireguard {
		t.Error("HasWireguard = false, want true (wireguard is in components)")
	}
	wantComponents := []string{"wireguard", "amneziawg", "schedule"}
	if len(caps.Components) != len(wantComponents) {
		t.Fatalf("Components = %v, want %v", caps.Components, wantComponents)
	}
	for i, w := range wantComponents {
		if caps.Components[i] != w {
			t.Errorf("Components[%d] = %q, want %q", i, caps.Components[i], w)
		}
	}
}

func TestDetectCapabilities_NoWireguardComponent(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"release": "4.2", "ndw": {"components": "schedule,vpn"}}`))
	})
	caps, err := DetectCapabilities(context.Background(), c)
	if err != nil {
		t.Fatalf("DetectCapabilities: %v", err)
	}
	if caps.HasWireguard {
		t.Error("HasWireguard = true, want false")
	}
	if caps.SupportsAWG2 {
		t.Error("SupportsAWG2 = true, want false for 4.2")
	}
}

// --- isAtLeast501A3 table ----------------------------------------------------

func TestIsAtLeast501A3(t *testing.T) {
	cases := []struct {
		release string
		want    bool
	}{
		{"4.2", false},
		{"5.00.03", false},
		{"5.01.A.2", false},
		{"5.01.A.3", true},
		{"5.01.A.7", true},
		{"5.01.B.1", true},
		{"5.01.03", true},
		{"5.02", true},
		{"6.0", true},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.release, func(t *testing.T) {
			if got := isAtLeast501A3(tc.release); got != tc.want {
				t.Errorf("isAtLeast501A3(%q) = %v, want %v", tc.release, got, tc.want)
			}
		})
	}
}
