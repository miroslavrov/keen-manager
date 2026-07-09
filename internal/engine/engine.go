// Package engine is the orchestration core of keen-manager. It owns the
// persisted state, the credential vault, the event bus and the component
// controllers (Xray, AmneziaWG, nfqws2, routing), and exposes the high-level
// operations the HTTP API and CLI call. All device-mutating work goes through a
// platform.Runner so it can be exercised safely (dry-run) off-device.
package engine

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miroslavrov/keen-manager/internal/awg"
	"github.com/miroslavrov/keen-manager/internal/config"
	"github.com/miroslavrov/keen-manager/internal/keenetic"
	"github.com/miroslavrov/keen-manager/internal/model"
	"github.com/miroslavrov/keen-manager/internal/nfqws"
	"github.com/miroslavrov/keen-manager/internal/platform"
	"github.com/miroslavrov/keen-manager/internal/route"
	"github.com/miroslavrov/keen-manager/internal/version"
	"github.com/miroslavrov/keen-manager/internal/xray"
)

// Fixed local endpoints for the active Xray instance. The SOCKS inbound is used
// both as a LAN proxy and as the through-tunnel health probe target.
const (
	xraySocksHost = "127.0.0.1"
	xraySocksPort = 10808
)

// Engine ties every subsystem together. It is safe for concurrent use.
type Engine struct {
	Paths    platform.Paths
	Platform model.Platform

	runner *platform.Runner
	store  *config.Store
	vault  *vault
	bus    *bus
	logs   *logBuffer

	xray  *xray.Controller
	awg   *awg.Controller
	nfqws *nfqws.Controller
	route *route.Manager

	// keenetic is the RCI client for the native AWG2 path; caps records what
	// the firmware supports (filled best-effort at startup by detectKeenetic).
	keenetic *keenetic.Client
	caps     keenetic.Capabilities

	// proxyClientDown latches true (for the process lifetime) once creating a
	// KeeneticOS Proxy interface has been rejected, so xrayMode() stops
	// attempting the proxy-connection path and uses the TPROXY fallback instead.
	// Cleared when the user explicitly changes the XrayIntegration setting.
	// Guarded by mu.
	proxyClientDown bool

	mu      sync.RWMutex
	runtime map[string]*model.RuntimeStatus // per-connection health, by conn ID

	sessMu   sync.Mutex
	sessions map[string]time.Time // auth token -> expiry

	foMu   sync.Mutex
	foFail int // consecutive failover probe failures
	nfFail int // consecutive nfqws-guard unhealthy observations

	startTime time.Time

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New constructs an Engine, loading state and the vault from disk. dryRun makes
// every device-mutating command a no-op (used for tests and off-device runs).
func New(paths platform.Paths, dryRun bool) (*Engine, error) {
	if err := paths.EnsureDirs(); err != nil {
		return nil, err
	}
	runner := platform.NewRunner()
	runner.DryRun = dryRun

	store, err := config.Open(paths.StateFile(), paths.BackupDir)
	if err != nil {
		return nil, err
	}
	v, err := openVault(filepath.Join(paths.DataDir, "servers.json"))
	if err != nil {
		return nil, err
	}

	b := newBus()
	e := &Engine{
		Paths:     paths,
		Platform:  platform.Detect(),
		runner:    runner,
		store:     store,
		vault:     v,
		bus:       b,
		logs:      newLogBuffer(1000, b),
		xray:      xray.NewController(paths, runner),
		awg:       awg.NewController(paths, runner),
		nfqws:     nfqws.NewController(paths, runner),
		route:     route.New(paths, runner),
		keenetic:  keenetic.New(""),
		runtime:   map[string]*model.RuntimeStatus{},
		sessions:  map[string]time.Time{},
		startTime: time.Now(),
	}
	runner.Log = func(cmd string) { e.logs.appendf("exec: %s", cmd) }
	// Reinstate the persisted web UI password (kept in the 0600 vault, not in
	// state.json) and self-heal the "auth enabled but no password" lockout.
	e.loadAuthFromVault()
	// Best-effort RCI probe: learns firmware release + native AWG2 support.
	e.detectKeenetic()
	e.logs.appendf("keen-manager %s ready (arch=%s os=%q dry-run=%v)",
		version.Short(), e.Platform.Arch, e.Platform.OSVersion, dryRun)
	return e, nil
}

// Start launches the background loops (health, failover, auto-select, sub
// refresh). Call Stop to shut them down.
func (e *Engine) Start(ctx context.Context) {
	e.ctx, e.cancel = context.WithCancel(ctx)
	e.startLoops()
	// Boot reconciliation: after a daemon restart or router reboot, re-establish
	// whichever connection was active before, so the tunnel comes back on its
	// own (uninterrupted-connection promise). Runs off the hot path.
	go func() {
		select {
		case <-e.ctx.Done():
			return
		case <-time.After(3 * time.Second): // let the WAN settle first
		}
		e.reconcile()
		// Re-apply any service routes whose target tunnel is back up.
		e.reconcileRoutes()
	}()
}

// Stop cancels the loops and waits for them to finish.
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
}

// Logf records a keen-manager log line (also fanned out over SSE).
func (e *Engine) Logf(format string, args ...any) { e.logs.appendf(format, args...) }

// HookName is the ndm netfilter.d hook filename (contains "keen-manager" so the
// uninstaller's glob finds it).
const HookName = "50-keen-manager"

// InstallHook writes the ndm netfilter.d hook that reapplies keen-manager's
// routing rules whenever KeeneticOS rebuilds iptables on a topology change.
func (e *Engine) InstallHook() error {
	binPath, err := os.Executable()
	if err != nil || binPath == "" {
		binPath = filepath.Join(e.Paths.Root, "bin", "keen-manager")
	}
	dir := filepath.Join(e.Paths.NdmDir, "netfilter.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	hookPath := filepath.Join(dir, HookName)
	if err := os.WriteFile(hookPath, []byte(route.HookScript(binPath)), 0o755); err != nil {
		return err
	}
	e.Logf("installed ndm netfilter hook -> %s", hookPath)
	return nil
}

// UninstallHook removes the ndm netfilter.d hook (best-effort).
func (e *Engine) UninstallHook() error {
	return os.Remove(filepath.Join(e.Paths.NdmDir, "netfilter.d", HookName))
}

// ReapplyRoutes re-installs the transparent-proxy rules. It is invoked by the
// ndm netfilter.d hook (via `keen-manager route reapply`) after KeeneticOS
// rebuilds iptables on a topology change. Only meaningful when an Xray
// connection is active; a no-op otherwise.
func (e *Engine) ReapplyRoutes() error {
	st := e.store.Get()
	if c, ok := findConn(st, st.ActiveConnID); ok && c.Type == model.ConnXray {
		if err := e.route.Reapply(); err != nil {
			return err
		}
	}
	if st.KillSwitch {
		return e.route.EnableKillSwitch()
	}
	return nil
}

// ----- event bus -----

// Subscribe returns an SSE subscriber id + channel. Unsubscribe when done.
func (e *Engine) Subscribe() (int, <-chan Event) { return e.bus.subscribe() }

// Unsubscribe removes an SSE subscriber.
func (e *Engine) Unsubscribe(id int) { e.bus.unsubscribe(id) }

// publishState nudges every SSE client to refetch aggregate state.
func (e *Engine) publishState() { e.bus.publish(Event{Type: EventState}) }

// ----- system views -----

// Health is the liveness/version summary.
func (e *Engine) Health() HealthView {
	return HealthView{
		Status:        "ok",
		Version:       version.Short(),
		Arch:          e.Platform.Arch,
		OS:            firstNonEmpty(e.Platform.OSVersion, "unknown"),
		UptimeSeconds: int64(time.Since(e.startTime).Seconds()),
	}
}

// State builds the aggregate dashboard view.
func (e *Engine) State() StateView {
	st := e.store.Get()
	conns := e.connViews(st)

	fo := st.Failover
	if fo.Chain == nil {
		fo.Chain = []string{}
	}
	if fo.History == nil {
		fo.History = []model.FailoverEvent{}
	}

	return StateView{
		ActiveConnectionID: st.ActiveConnID,
		Connections:        conns,
		Nfqws:              e.nfqws.Status(),
		Failover:           fo,
		Wan:                e.detectWAN(),
		KillSwitch:         st.KillSwitch,
	}
}

// detectWAN reads the default-route interface, its IPv4 address and the system
// uptime. Everything is best-effort; fields stay empty on any failure.
func (e *Engine) detectWAN() WanView {
	w := WanView{UptimeSeconds: readUptime()}
	out, err := e.runner.Output(e.ipBin(), "route", "show", "default")
	if err != nil || out == "" {
		return w
	}
	fields := strings.Fields(out)
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			w.Interface = fields[i+1]
			break
		}
	}
	if w.Interface == "" {
		return w
	}
	if a, err := e.runner.Output(e.ipBin(), "-4", "-o", "addr", "show", "dev", w.Interface); err == nil {
		af := strings.Fields(a)
		for i, f := range af {
			if f == "inet" && i+1 < len(af) {
				w.IP = strings.SplitN(af[i+1], "/", 2)[0]
				break
			}
		}
	}
	return w
}

// ----- helpers -----

func (e *Engine) ipBin() string {
	if platform.FileExists(e.Paths.IPBin) {
		return e.Paths.IPBin
	}
	return "ip"
}

// runtimeFor returns a copy of the runtime status for a connection.
func (e *Engine) runtimeFor(id string) (model.RuntimeStatus, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	rs, ok := e.runtime[id]
	if !ok {
		return model.RuntimeStatus{}, false
	}
	return *rs, true
}

// setRuntime stores the runtime status for a connection.
func (e *Engine) setRuntime(id string, rs model.RuntimeStatus) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := rs
	e.runtime[id] = &cp
}

// dropRuntime removes runtime state for a deleted connection.
func (e *Engine) dropRuntime(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.runtime, id)
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// readUptime returns the system uptime in seconds from /proc/uptime (0 if N/A).
func readUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	f, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int64(f)
}

// newID returns a short, URL-safe unique id with the given prefix.
func newID(prefix string) string {
	return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}
