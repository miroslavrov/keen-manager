# keen-manager

A single, unified control panel for VPN and DPI-bypass on **Keenetic** routers.

keen-manager brings **AmneziaWG**, **Xray** (with subscription support), and
**nfqws2** (DPI bypass / service splitting) under one clean web UI and one CLI —
with automatic best-location selection, health checks, and **fallback chains** so
your connection stays up even when a server is blocked or an operator rotates IPs.

> Status: **early alpha / work in progress.** The full stack is implemented and
> compiles to a single self-contained binary: the Go core (subscription
> parsing, Xray config generation, AWG config handling, health & failover
> logic), the daemon (REST/JSON API + SSE + embedded web UI), the CLI, and all
> seven web UI pages. Device-side actions are built to be safe
> (validate-before-apply, backups, rollback) but **must be tested on real
> hardware before you trust them on a router you can't physically reach.**

---

## Why

On Keenetic you usually end up juggling several separate tools:

- [`hoaxisr/awg-manager`](https://github.com/hoaxisr/awg-manager) for AmneziaWG,
- [`XKeen`](https://github.com/Skrill0/XKeen) / manual Xray for VLESS/Reality,
- [`nfqws2-keenetic`](https://github.com/nfqws/nfqws2-keenetic) +
  [`nfqws-keenetic-web`](https://github.com/nfqws/nfqws-keenetic-web) for DPI bypass.

Each has its own UI, its own config, its own service. keen-manager is one binary
that **orchestrates** them and adds the glue that was missing: paste a
subscription link, let it pick the fastest working location, and fall back
automatically when something dies.

## Features

- **Unified connections.** Manage AmneziaWG tunnels and Xray outbounds side by
  side. One list, one status view, one place to switch what's active.
- **Xray subscriptions.** Add a subscription URL (e.g.
  `https://host/s/<token>`). keen-manager fetches it, parses every server
  (`vless` / `vmess` / `trojan` / `ss`, base64 / Clash YAML / SIP008), and keeps
  it fresh on a schedule. Reality + `xtls-rprx-vision` fully supported.
- **Best-location, automatically.** Uses Xray's `burstObservatory` + a
  `leastPing` balancer to continuously route through the lowest-latency working
  server — no manual server picking.
- **Health checks.** Every connection is probed (SOCKS probe / TCP ping /
  handshake age for AWG). Dead servers are detected and skipped.
- **Fallback chains.** Define an ordered chain, e.g.
  `Xray (best) → AmneziaWG → direct/kill-switch`. If the active connection — or
  its nfqws2 strategy — dies, traffic falls back to the next node and returns
  automatically when the primary recovers.
- **nfqws2 integration.** Drive the existing `nfqws2-keenetic` service:
  start/stop/reload, switch mode (`AUTO`/`LIST`/`ALL`), edit strategies and
  hostlists (`user`/`auto`/`exclude`/`ipset`), and a domain-reachability checker
  — the same functionality as the original web UI, in one place.
- **Clean web UI + CLI.** A shadcn/ui front-end (dark, no emoji, lucide icons)
  embedded in the binary, plus a scriptable CLI for the same operations.
- **Built not to brick your router.** Validate configs before applying, back up
  before every change, and a test-and-rollback deadman that restores
  connectivity if an apply goes wrong. Only ever touches `/opt` (Entware).

## Architecture

A single, statically-linked Go binary (no runtime dependencies), which is the
right fit for MIPS/ARM routers:

```
keen-manager (one binary)
├── daemon         REST API + embedded web UI + health/failover engine
├── cli            same operations, scriptable
└── manages
    ├── Xray       generates config from subscriptions; observatory + balancer
    ├── AmneziaWG  config parse/gen (incl. Jc/S1/H1.. obfuscation); awg-quick
    └── nfqws2     drives the nfqws2-keenetic init script + config + hostlists
```

- **Backend:** Go, `CGO_ENABLED=0`, cross-compiled for `mipsle` (softfloat),
  `mips` (big-endian, softfloat), and `arm64`.
- **Front-end:** React + TypeScript + Vite + Tailwind + shadcn/ui, built to
  static assets and embedded via `go:embed`.
- **Cooperates with the firmware:** reinstalls its firewall rules from an
  `/opt/etc/ndm/netfilter.d` hook (Keenetic flushes iptables on every topology
  change), uses fwmarks/tables outside Keenetic's reserved ranges, and never
  flushes the firmware's own chains.

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the details (routing,
tproxy, safety model, arch detection).

## Install (Keenetic + Entware)

> Requires a Keenetic router with USB/NAND storage, **Entware (opkg)**, and the
> firmware components *IPv6 protocol* and *Netfilter subsystem kernel modules*.

```sh
opkg update && opkg install curl
curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/install.sh | sh
```

The installer detects your architecture, installs the right binary to
`/opt/bin/keen-manager`, sets up the init script and ndm hook, and prints the web
UI address (default `http://<router-lan-ip>:8088`).

## Build from source

```sh
# Front-end -> internal/webui/dist (requires Node 18+)
make web

# Cross-compile all router targets into ./build
make build-all

# Or a single target
make build GOARCH=mipsle GOMIPS=softfloat
```

## Safety

keen-manager is designed so the worst case is a **self-reverting failed apply**,
not a locked-out router:

- Config is **validated before it is applied** (`xray -test`, AWG field checks).
- Every change **backs up** the previous config and a firewall/route snapshot.
- A **rollback deadman** restores the last-known-good state unless the change is
  confirmed within a timeout.
- The web UI binds to the LAN, **outside** any tunnel/proxy it manages, so a
  broken VPN can never lock you out of the manager.
- It only writes under `/opt`. It never touches firmware partitions.

Still — this is alpha software that manipulates routing and firewall rules. Test
on hardware you can physically reach first.

## Licensing & credits

keen-manager is **MIT** licensed (see [`LICENSE`](LICENSE)).

It is an **orchestrator**: it manages proven components rather than re-bundling
them, which keeps licensing clean. Design and integration were informed by these
MIT-licensed projects (keen-manager is independent and not endorsed by them):

- [`hoaxisr/awg-manager`](https://github.com/hoaxisr/awg-manager) — MIT
- [`nfqws/nfqws2-keenetic`](https://github.com/nfqws/nfqws2-keenetic) — MIT
- [`nfqws/nfqws-keenetic-web`](https://github.com/nfqws/nfqws-keenetic-web) — MIT

Components keen-manager drives but does **not** redistribute (installed from
their own upstreams, under their own terms): the `nfqws2` daemon
(from `bol-van/zapret2`), AmneziaWG kernel modules/tools, and `xray-core`
(XTLS/Xray-core, MPL-2.0). See [`NOTICE`](NOTICE).

## Roadmap

- [x] Finish web UI feature pages (dashboard, connections, subscriptions, bypass, failover, logs, settings)
- [ ] Xray gRPC hot-reload (swap outbounds without a full restart)
- [ ] Policy-based routing per device group (Keenetic policy fwmark integration)
- [ ] IPK packaging + hosted opkg feed
- [ ] On-device integration tests

## Disclaimer

For personal use. You are responsible for complying with the laws and terms of
service that apply to you. No warranty — see the license.
