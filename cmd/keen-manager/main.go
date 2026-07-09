// Command keen-manager is the single binary that runs the keen-manager daemon
// (REST API + embedded web UI + health/failover engine) and provides a
// scriptable CLI for the same operations.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/miroslavrov/keen-manager/internal/engine"
	"github.com/miroslavrov/keen-manager/internal/model"
	"github.com/miroslavrov/keen-manager/internal/platform"
	"github.com/miroslavrov/keen-manager/internal/server"
	"github.com/miroslavrov/keen-manager/internal/version"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}
	cmd, rest := args[0], args[1:]

	switch cmd {
	case "daemon", "serve":
		runDaemon()
	case "status":
		cmdStatus()
	case "conn", "connection":
		cmdConn(rest)
	case "sub", "subscription":
		cmdSub(rest)
	case "nfqws":
		cmdNfqws(rest)
	case "route":
		cmdRoute(rest)
	case "passwd", "password":
		cmdPasswd(rest)
	case "auth":
		cmdAuth(rest)
	case "install-hook":
		eng := openEngine()
		if err := eng.InstallHook(); err != nil {
			fatal("install-hook: %v", err)
		}
		fmt.Println("ndm netfilter hook installed")
	case "uninstall-hook":
		eng := openEngine()
		if err := eng.UninstallHook(); err != nil {
			fatal("uninstall-hook: %v", err)
		}
		fmt.Println("ndm netfilter hook removed")
	case "version", "-v", "--version":
		fmt.Println(version.String())
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`keen-manager — unified VPN / DPI control for Keenetic

USAGE:
  keen-manager <command> [args]

COMMANDS:
  daemon                       run the REST API + web UI + health/failover engine
  status                       print health and connection status (JSON)
  conn list                    list connections
  conn up|down|activate|test <id>
                               control a connection
  sub list                     list subscriptions
  sub add <name> <url>         add an Xray subscription
  sub refresh <id>             re-fetch a subscription
  sub best <id>                activate the fastest server in a subscription
  nfqws status                 show nfqws2 service status
  nfqws start|stop|restart|reload|install
                               control the nfqws2 service
  nfqws mode <MODE_AUTO|MODE_LIST|MODE_ALL>
                               set the nfqws2 mode
  passwd <new-password>        set the web UI password and enable auth
  auth disable                 turn off the web UI login (recover from a lockout)
  auth status                  show whether the web UI login is enabled
  route reapply                re-install transparent-proxy rules (ndm hook)
  install-hook                 install the ndm netfilter.d hook (done by installer)
  uninstall-hook               remove the ndm netfilter.d hook
  version                      print the version

ENVIRONMENT:
  KEEN_LISTEN   listen address for the daemon (default ":<settings.port>")
  KEEN_ROOT     Entware root (default /opt)
  KEEN_DRY_RUN  set to 1 to make all device commands no-ops (testing)
`)
}

// openEngine constructs an Engine for a one-shot CLI command (no loops).
func openEngine() *engine.Engine {
	eng, err := engine.New(platform.DefaultPaths(), dryRun())
	if err != nil {
		fatal("init: %v", err)
	}
	return eng
}

func runDaemon() {
	paths := platform.DefaultPaths()

	// Single-instance guard: a second daemon writing the same state.json races
	// the first (and double-drives the router), so refuse to start when one is
	// already running. The flock releases itself if the holder dies, so a crash
	// never leaves a stale lock behind. A missing/read-only lock dir is not fatal
	// — we warn and continue rather than block startup on lock infrastructure.
	lock, lockErr := platform.AcquireLock(paths.LockFile("keen-manager"))
	switch {
	case errors.Is(lockErr, platform.ErrLocked):
		fatal("another keen-manager daemon is already running (%v); stop it first, or run `/opt/etc/init.d/S99keen-manager restart`", lockErr)
	case lockErr != nil:
		fmt.Fprintf(os.Stderr, "keen-manager: single-instance lock unavailable (%v); continuing without it\n", lockErr)
	default:
		defer lock.Release()
	}

	eng, err := engine.New(paths, dryRun())
	if err != nil {
		fatal("init: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	eng.Start(ctx)
	defer eng.Stop()

	addr := os.Getenv("KEEN_LISTEN")
	if addr == "" {
		addr = fmt.Sprintf(":%d", eng.Settings().Port)
	}
	srv := server.New(eng)
	eng.Logf("HTTP listening on %s", addr)
	fmt.Fprintf(os.Stderr, "keen-manager %s listening on %s\n", version.Short(), addr)
	if err := srv.ListenAndServe(ctx, addr); err != nil {
		fatal("serve: %v", err)
	}
}

// ----- CLI commands -----

func cmdStatus() {
	eng := openEngine()
	printJSON(map[string]any{
		"health":      eng.Health(),
		"active":      eng.State().ActiveConnectionID,
		"connections": eng.Connections(),
		"nfqws":       eng.Nfqws(),
	})
}

func cmdConn(args []string) {
	if len(args) == 0 {
		fatal("usage: keen-manager conn list|up|down|activate|test [id]")
	}
	eng := openEngine()
	switch args[0] {
	case "list", "ls":
		printJSON(eng.Connections())
	case "up", "down", "activate", "test":
		if len(args) < 2 {
			fatal("usage: keen-manager conn %s <id>", args[0])
		}
		if err := eng.ConnectionAction(args[1], args[0]); err != nil {
			fatal("%v", err)
		}
		fmt.Printf("conn %s: %s ok\n", args[1], args[0])
	default:
		fatal("unknown conn subcommand %q", args[0])
	}
}

func cmdSub(args []string) {
	if len(args) == 0 {
		fatal("usage: keen-manager sub list|add|refresh|best")
	}
	eng := openEngine()
	switch args[0] {
	case "list", "ls":
		printJSON(eng.Subscriptions())
	case "add":
		if len(args) < 3 {
			fatal("usage: keen-manager sub add <name> <url>")
		}
		v, err := eng.CreateSubscription(args[1], args[2])
		if err != nil {
			fatal("%v", err)
		}
		printJSON(v)
	case "refresh":
		if len(args) < 2 {
			fatal("usage: keen-manager sub refresh <id>")
		}
		v, err := eng.RefreshSubscription(args[1])
		if err != nil {
			fatal("%v", err)
		}
		printJSON(v)
	case "best":
		if len(args) < 2 {
			fatal("usage: keen-manager sub best <id>")
		}
		id, err := eng.SelectBest(args[1])
		if err != nil {
			fatal("%v", err)
		}
		fmt.Printf("activated %s\n", id)
	default:
		fatal("unknown sub subcommand %q", args[0])
	}
}

func cmdNfqws(args []string) {
	if len(args) == 0 {
		fatal("usage: keen-manager nfqws status|start|stop|restart|reload|install|mode")
	}
	eng := openEngine()
	switch args[0] {
	case "status":
		printJSON(eng.Nfqws())
	case "start", "stop", "restart", "reload", "install":
		if err := eng.NfqwsAction(args[0]); err != nil {
			fatal("%v", err)
		}
		fmt.Printf("nfqws2: %s ok\n", args[0])
	case "mode":
		if len(args) < 2 {
			fatal("usage: keen-manager nfqws mode <MODE_AUTO|MODE_LIST|MODE_ALL>")
		}
		if err := eng.SaveNfqwsConfig("", model.NfqwsMode(strings.ToUpper(args[1]))); err != nil {
			fatal("%v", err)
		}
		fmt.Printf("nfqws2 mode set to %s\n", args[1])
	default:
		fatal("unknown nfqws subcommand %q", args[0])
	}
}

func cmdRoute(args []string) {
	if len(args) == 0 || args[0] != "reapply" {
		fatal("usage: keen-manager route reapply")
	}
	eng := openEngine()
	if err := eng.ReapplyRoutes(); err != nil {
		fatal("%v", err)
	}
	fmt.Println("routes reapplied")
}

func cmdPasswd(args []string) {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		fatal("usage: keen-manager passwd <new-password>")
	}
	eng := openEngine()
	if err := eng.SetPassword(args[0]); err != nil {
		fatal("%v", err)
	}
	fmt.Println("web UI password set; auth enabled.")
	fmt.Println("restart the service for a running daemon to pick it up:")
	fmt.Println("  /opt/etc/init.d/S99keen-manager restart")
}

func cmdAuth(args []string) {
	if len(args) == 0 {
		fatal("usage: keen-manager auth <disable|status>")
	}
	eng := openEngine()
	switch args[0] {
	case "disable", "off":
		if err := eng.DisableAuth(); err != nil {
			fatal("%v", err)
		}
		fmt.Println("web UI auth disabled.")
		fmt.Println("restart the service for a running daemon to pick it up:")
		fmt.Println("  /opt/etc/init.d/S99keen-manager restart")
	case "status":
		printJSON(eng.AuthState(false))
	default:
		fatal("usage: keen-manager auth <disable|status>")
	}
}

// ----- helpers -----

func dryRun() bool {
	v := strings.ToLower(os.Getenv("KEEN_DRY_RUN"))
	return v == "1" || v == "true" || v == "yes"
}

func printJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fatal("encode: %v", err)
	}
	fmt.Println(string(b))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "keen-manager: "+format+"\n", args...)
	os.Exit(1)
}
