package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/miroslavrov/keen-manager/internal/model"
)

// registerRoutes wires every JSON endpoint. Method+pattern routing is provided
// by net/http's ServeMux (Go 1.22+). Public endpoints (health, auth, login,
// logout) are open; everything else requires a session when auth is enabled.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Public.
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/auth", s.handleAuthState)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)

	// Live events (auth checked inside).
	mux.HandleFunc("GET /api/events", s.handleEvents)

	// Aggregate.
	mux.HandleFunc("GET /api/state", s.requireAuth(s.handleState))

	// Connections.
	mux.HandleFunc("GET /api/connections", s.requireAuth(s.handleConnections))
	mux.HandleFunc("POST /api/connections", s.requireAuth(s.handleCreateConnection))
	mux.HandleFunc("GET /api/connections/{id}", s.requireAuth(s.handleConnection))
	mux.HandleFunc("PUT /api/connections/{id}", s.requireAuth(s.handleUpdateConnection))
	mux.HandleFunc("DELETE /api/connections/{id}", s.requireAuth(s.handleDeleteConnection))
	mux.HandleFunc("POST /api/connections/{id}/{action}", s.requireAuth(s.handleConnectionAction))

	// Subscriptions.
	mux.HandleFunc("GET /api/subscriptions", s.requireAuth(s.handleSubscriptions))
	mux.HandleFunc("POST /api/subscriptions", s.requireAuth(s.handleCreateSubscription))
	mux.HandleFunc("PUT /api/subscriptions/{id}", s.requireAuth(s.handleUpdateSubscription))
	mux.HandleFunc("DELETE /api/subscriptions/{id}", s.requireAuth(s.handleDeleteSubscription))
	mux.HandleFunc("POST /api/subscriptions/{id}/refresh", s.requireAuth(s.handleRefreshSubscription))
	mux.HandleFunc("GET /api/subscriptions/{id}/servers", s.requireAuth(s.handleSubscriptionServers))
	mux.HandleFunc("POST /api/subscriptions/{id}/select-best", s.requireAuth(s.handleSelectBest))

	// Routes / "Маршруты" (per-service domain routing).
	mux.HandleFunc("GET /api/routes", s.requireAuth(s.handleRoutes))
	mux.HandleFunc("POST /api/routes", s.requireAuth(s.handleCreateRoute))
	mux.HandleFunc("GET /api/routes/presets", s.requireAuth(s.handleRoutePresets))
	mux.HandleFunc("PUT /api/routes/{id}/toggle", s.requireAuth(s.handleToggleRoute))
	mux.HandleFunc("DELETE /api/routes/{id}", s.requireAuth(s.handleDeleteRoute))

	// nfqws2.
	mux.HandleFunc("GET /api/nfqws", s.requireAuth(s.handleNfqws))
	mux.HandleFunc("POST /api/nfqws/action", s.requireAuth(s.handleNfqwsAction))
	mux.HandleFunc("GET /api/nfqws/config", s.requireAuth(s.handleNfqwsConfig))
	mux.HandleFunc("PUT /api/nfqws/config", s.requireAuth(s.handleSaveNfqwsConfig))
	mux.HandleFunc("GET /api/nfqws/lists", s.requireAuth(s.handleNfqwsLists))
	mux.HandleFunc("GET /api/nfqws/lists/{name}", s.requireAuth(s.handleNfqwsList))
	mux.HandleFunc("PUT /api/nfqws/lists/{name}", s.requireAuth(s.handleSaveNfqwsList))
	mux.HandleFunc("POST /api/nfqws/check-domain", s.requireAuth(s.handleCheckDomain))

	// Failover.
	mux.HandleFunc("GET /api/failover", s.requireAuth(s.handleFailover))
	mux.HandleFunc("PUT /api/failover", s.requireAuth(s.handleSaveFailover))

	// Settings.
	mux.HandleFunc("GET /api/settings", s.requireAuth(s.handleSettings))
	mux.HandleFunc("PUT /api/settings", s.requireAuth(s.handleSaveSettings))

	// Kill switch.
	mux.HandleFunc("POST /api/killswitch", s.requireAuth(s.handleKillSwitch))

	// Logs.
	mux.HandleFunc("GET /api/logs", s.requireAuth(s.handleLogs))

	// Catch-all for unknown API paths -> JSON 404 (keeps them out of the SPA).
	mux.HandleFunc("/api/", notFoundJSON)
}

// ----- system -----

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.Health())
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.State())
}

// ----- connections -----

func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.Connections())
}

func (s *Server) handleConnection(w http.ResponseWriter, r *http.Request) {
	d, err := s.eng.Connection(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleCreateConnection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type      model.ConnType `json:"type"`
		Name      string         `json:"name"`
		AWGConf   string         `json:"awg_conf"`
		ShareLink string         `json:"share_link"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	v, err := s.eng.CreateConnection(body.Type, body.Name, body.AWGConf, body.ShareLink)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleUpdateConnection(w http.ResponseWriter, r *http.Request) {
	var fields map[string]any
	if err := readJSON(r, &fields); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	v, err := s.eng.UpdateConnection(r.PathValue("id"), fields)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleDeleteConnection(w http.ResponseWriter, r *http.Request) {
	if err := s.eng.DeleteConnection(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) handleConnectionAction(w http.ResponseWriter, r *http.Request) {
	if err := s.eng.ConnectionAction(r.PathValue("id"), r.PathValue("action")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w)
}

// ----- subscriptions -----

func (s *Server) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.Subscriptions())
}

func (s *Server) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.URL) == "" {
		writeErr(w, http.StatusBadRequest, "subscription url is required")
		return
	}
	v, err := s.eng.CreateSubscription(body.Name, body.URL)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleUpdateSubscription(w http.ResponseWriter, r *http.Request) {
	var fields map[string]any
	if err := readJSON(r, &fields); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	v, err := s.eng.UpdateSubscription(r.PathValue("id"), fields)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	if err := s.eng.DeleteSubscription(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) handleRefreshSubscription(w http.ResponseWriter, r *http.Request) {
	v, err := s.eng.RefreshSubscription(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleSubscriptionServers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.SubscriptionServers(r.PathValue("id")))
}

func (s *Server) handleSelectBest(w http.ResponseWriter, r *http.Request) {
	id, err := s.eng.SelectBest(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "selected_id": id})
}

// ----- routes / "Маршруты" -----

func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.Routes())
}

func (s *Server) handleRoutePresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.RoutePresets())
}

func (s *Server) handleCreateRoute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string   `json:"name"`
		PresetID     string   `json:"preset_id"`
		Domains      []string `json:"domains"`
		Subnets      []string `json:"subnets"`
		TargetConnID string   `json:"target_conn_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.TargetConnID) == "" {
		writeErr(w, http.StatusBadRequest, "target_conn_id is required")
		return
	}
	v, err := s.eng.CreateRoute(body.Name, body.PresetID, body.Domains, body.Subnets, body.TargetConnID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleToggleRoute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.eng.SetRouteEnabled(r.PathValue("id"), body.Enabled); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) handleDeleteRoute(w http.ResponseWriter, r *http.Request) {
	if err := s.eng.DeleteRoute(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w)
}

// ----- nfqws2 -----

func (s *Server) handleNfqws(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.Nfqws())
}

func (s *Server) handleNfqwsAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.eng.NfqwsAction(body.Action); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) handleNfqwsConfig(w http.ResponseWriter, r *http.Request) {
	v, err := s.eng.NfqwsConfig()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleSaveNfqwsConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Raw  string          `json:"raw"`
		Mode model.NfqwsMode `json:"mode"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.eng.SaveNfqwsConfig(body.Raw, body.Mode); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) handleNfqwsLists(w http.ResponseWriter, r *http.Request) {
	lists, err := s.eng.NfqwsLists()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, lists)
}

func (s *Server) handleNfqwsList(w http.ResponseWriter, r *http.Request) {
	v, err := s.eng.NfqwsList(r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleSaveNfqwsList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Content string `json:"content"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.eng.SaveNfqwsList(r.PathValue("name"), body.Content); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) handleCheckDomain(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Domain string `json:"domain"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	writeJSON(w, http.StatusOK, s.eng.CheckDomain(body.Domain))
}

// ----- failover -----

func (s *Server) handleFailover(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.Failover())
}

func (s *Server) handleSaveFailover(w http.ResponseWriter, r *http.Request) {
	var fo model.Failover
	if err := readJSON(r, &fo); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.eng.SaveFailover(fo); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w)
}

// ----- settings -----

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.Settings())
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var fields map[string]any
	if err := readJSON(r, &fields); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.eng.SaveSettings(fields); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w)
}

func (s *Server) handleKillSwitch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.eng.SetKillSwitch(body.Enabled); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeOK(w)
}

// ----- logs -----

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	writeJSON(w, http.StatusOK, s.eng.Logs(service, lines))
}
