package keenetic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImportConfig(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"interface":{"wireguard":{"import":{"intersects":"","created":"Wireguard3","status":[{"status":"message","message":"Network::Interface::Wireguard: interface created."}]}}}}`)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	conf := "[Interface]\nPrivateKey = abc\nJc = 4\n[Peer]\nEndpoint = 1.2.3.4:51820\n"
	res, err := ImportConfig(context.Background(), c, []byte(conf), "km-test.conf")
	if err != nil {
		t.Fatalf("ImportConfig: %v", err)
	}
	if res.Created != "Wireguard3" {
		t.Fatalf("Created = %q, want Wireguard3", res.Created)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("Messages = %v, want 1 entry", res.Messages)
	}

	// The payload must carry a base64 import of our exact conf.
	iface, _ := gotBody["interface"].(map[string]any)
	wg, _ := iface["wireguard"].(map[string]any)
	imp, _ := wg["import"].(string)
	dec, err := base64.StdEncoding.DecodeString(imp)
	if err != nil {
		t.Fatalf("import field is not valid base64: %v", err)
	}
	if string(dec) != conf {
		t.Fatalf("decoded import payload = %q, want the original conf", string(dec))
	}
	if fn, _ := wg["filename"].(string); fn != "km-test.conf" {
		t.Fatalf("filename = %q, want km-test.conf", fn)
	}
}

func TestImportConfigNoCreated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A nested error status with no "created" interface.
		_, _ = io.WriteString(w, `{"interface":{"wireguard":{"import":{"intersects":"Wireguard0","created":"","status":[{"status":"error","message":"configuration already exists"}]}}}}`)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	_, err := ImportConfig(context.Background(), c, []byte("[Interface]\n"), "km.conf")
	if err == nil {
		t.Fatal("expected an error when the router returns no created interface")
	}
	if !strings.Contains(err.Error(), "import") {
		t.Fatalf("error should mention import, got: %v", err)
	}
}

func TestSetInterfaceGlobalPayload(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &gotBody)
		_, _ = io.WriteString(w, `{"status":[{"status":"message","message":"ok"}]}`)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}

	if err := SetInterfaceGlobal(context.Background(), c, "Wireguard3", true); err != nil {
		t.Fatalf("SetInterfaceGlobal: %v", err)
	}
	iface, _ := gotBody["interface"].(map[string]any)
	wg3, _ := iface["Wireguard3"].(map[string]any)
	ip, _ := wg3["ip"].(map[string]any)
	if g, ok := ip["global"].(bool); !ok || !g {
		t.Fatalf("expected interface.Wireguard3.ip.global=true, got body %v", gotBody)
	}
}
