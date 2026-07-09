# keen-manager — Architecture

This document describes how keen-manager is built and how it behaves on a
Keenetic router. It is aimed at contributors and at operators who want to
understand exactly what the software does to their device before they trust it.

keen-manager is an **orchestrator**. It does not reimplement or redistribute the
VPN/DPI components it controls — it drives proven upstream tools (AmneziaWG,
Xray/xray-core, and the `nfqws2` DPI-bypass daemon) and adds the glue that ties
them into one UI, one CLI, one health/failover engine, and one safe apply model.

---

## 1. Single-binary design

keen-manager ships as **one statically linked Go binary** (`CGO_ENABLED=0`), with
no runtime dependencies beyond BusyBox and the standard Entware environment. That
is the right shape for MIPS/ARM routers with limited storage and no toolchain.

The binary contains three faces over the same core:

```
keen-manager (one static binary)
├── daemon      REST/JSON API + embedded web UI + health/failover engine
├── cli         the same operations, scriptable (status, conn, sub, nfqws, ...)
└── manages (drives, does not bundle)
    ├── Xray        generates config from subscriptions; observatory + balancer
    ├── AmneziaWG   parse/generate config (Jc/S1/H1.. obfuscation); awg-quick
    └── nfqws2      drives the nfqws2-keenetic init script + config + hostlists
```

- **Daemon.** `keen-manager daemon` runs in the foreground (the init script
  backgrounds it). It serves the JSON API and the embedded front-end, and runs
  the health-probe and failover loops. It honors `KEEN_ROOT` and `KEEN_DATA_DIR`
  for off-device testing.
- **Embedded UI.** The React/TypeScript front-end is built to static assets and
  embedded into the binary with `go:embed` (`internal/webui`, from
  `internal/webui/dist`). There is no separate web server to install; the daemon
  serves the UI same-origin with the API.
- **CLI.** Every daemon operation is also a CLI verb, so the tool is scriptable
  and debuggable over SSH without the UI.

### CLI contract (relied on by ops tooling)

The build/installer/init artifacts depend on this exact command surface:

| Command                      | Purpose                                                        |
| ---------------------------- | ------------------------------------------------------------- |
| `keen-manager daemon`        | Run the HTTP daemon in the foreground (init script backgrounds it). |
| `keen-manager version`       | Print the version line (`Version`/`Commit`/`Date`).           |
| `keen-manager status`        | Human-readable status for CLI users.                          |
| `keen-manager route reapply` | Reapply firewall/route rules — called by the ndm hook.        |
| `keen-manager install-hook`  | Write the `/opt/etc/ndm/netfilter.d` reapply hook.            |

Additional verbs (`conn`, `sub`, `nfqws`, `failover`, …) drive the managed
components but are not required by the ops layer.

---

## 2. Why it does not redistribute the components

keen-manager manages, but never ships, the heavy third-party pieces:

- **xray-core** (XTLS/Xray-core, MPL-2.0) — installed from its own upstream.
- **AmneziaWG** kernel modules and userspace tools — installed from upstream.
- **nfqws2** (from `bol-van/zapret2`) — installed via `nfqws2-keenetic`.

Keeping these out of the binary keeps licensing clean (keen-manager itself is
MIT) and lets each component be updated on its own cadence. keen-manager locates
them at their canonical Entware paths (`/opt/sbin/xray`, `/opt/sbin/awg`,
`/opt/usr/bin/nfqws2`, `/opt/etc/init.d/S51nfqws2`) and cooperates with their
existing init scripts and config directories rather than replacing them.

---

## 3. On-disk layout

Everything keen-manager owns lives under the Entware root, `/opt`. It never
writes to firmware partitions. The canonical layout (see
`internal/platform/paths.go`):

```
/opt
├── bin/
│   └── keen-manager                     the static binary
├── etc/
│   ├── init.d/
│   │   ├── S99keen-manager              our init script (start/stop/status)
│   │   └── S51nfqws2                    nfqws2's init script (driven, not owned)
│   ├── ndm/
│   │   └── netfilter.d/
│   │       └── *keen-manager*           reapply hook (written by the binary)
│   ├── keen-manager/                    our data dir  (KEEN_DATA_DIR)
│   │   ├── state.json                   main persisted config document
│   │   ├── vault.json                   secrets (keys, subscription tokens)
│   │   ├── xray/                        generated Xray config dir
│   │   │   └── config.json              generated from subscriptions
│   │   └── backups/                     pre-change config + fw/route snapshots
│   └── nfqws2/                          nfqws2 conf + hostlists (driven)
│       ├── nfqws2.conf
│       └── lists/                       user / auto / exclude / ipset lists
└── var/
    ├── log/
    │   └── keen-manager.log             daemon stdout/stderr
    └── run/
        └── keen-manager.pid             init-script PID file
```

Notes:

- **`state.json`** is the single source of truth for connections,
  subscriptions, the failover chain, and nfqws2 settings.
- **`vault.json`** isolates secrets (private keys, Reality keys, subscription
  tokens) from the rest of the state so they can be handled with tighter
  permissions and kept out of logs and backups intended for sharing.
- **`backups/`** holds the previous config plus a firewall/route snapshot taken
  before every apply, which is what the rollback deadman restores.
- The **Xray config dir** and **nfqws2 conf/lists** are generated/edited by
  keen-manager but consumed by the upstream daemons unchanged.

---

## 4. Architecture (CPU) detection

Router CPUs vary; the correct binary must be selected without a compiler on the
device. keen-manager and its installer use the same resolution order (see
`internal/platform/arch.go` and `scripts/install.sh`):

1. **`opkg print-architecture`** — the authoritative source on Entware. The
   package arch names map as:
   - `aarch64*` → `arm64`
   - `mipselsf*` / `mipsel*` → `mipsle`
   - `mipssf*` / `mips*` → `mips`
   - `armv7*` / `arm*` → `arm`
   (`all` / `noarch` are ignored.)
2. **`uname -m`** — fallback. `aarch64`→`arm64`, `armv7*`/`arm*`→`arm`, and for
   bare `mips` the byte order decides `mips` vs `mipsle`.
3. **ELF endianness probe** — for MIPS, read the `EI_DATA` byte (offset 5) of a
   system binary (`/bin/sh`, `/bin/busybox`): `0x01` = little-endian (`mipsle`),
   `0x02` = big-endian (`mips`).

The build system produces one artifact per target; the installer downloads only
the one that matches.

| Detected arch | `GOARCH` | `GOMIPS`     | Release artifact             |
| ------------- | -------- | ------------ | ---------------------------- |
| `mipsle`      | `mipsle` | `softfloat`  | `keen-manager-mipsle.gz`     |
| `mips`        | `mips`   | `softfloat`  | `keen-manager-mips.gz`       |
| `arm64`       | `arm64`  | (unset)      | `keen-manager-arm64.gz`      |
| `arm`         | `arm`    | (unset)      | `keen-manager-arm.gz`        |

`GOMIPS=softfloat` is mandatory for both MIPS variants — these router SoCs have
no hardware FPU, and a hardfloat binary would trap/crash. All targets build with
`CGO_ENABLED=0 -trimpath` and `-ldflags "-s -w -X ...version.*"`.

---

## 5. Routing & TPROXY model

Transparent proxying (TPROXY) and the kill-switch are the only parts of
keen-manager that touch the device's routing and firewall. They are **disabled by
default** — every other feature (config generation, health, failover, nfqws2, a
local SOCKS proxy) works without them. The implementation
(`internal/route/route.go`) is deliberately conservative.

### Fixed identifiers (chosen outside Keenetic's reserved ranges)

| Identifier          | Value    | Role                                             |
| ------------------- | -------- | ------------------------------------------------ |
| fwmark              | `0x2333` | marks packets to be routed into the tunnel       |
| routing table       | `993`    | policy table that sends marked packets to TPROXY |
| private mangle chain| `KEENMGR_TPROXY` | our TPROXY capture rules (jumped from PREROUTING) |
| private filter chain| `KEENMGR_KILL`   | kill-switch drop rules (jumped from FORWARD)      |
| TPROXY inbound port | `12345`  | Xray's TPROXY listener                            |
| Xray self-mark      | `255`    | SO_MARK on Xray's own egress (excluded from capture) |

The fwmark and table were picked to sit **outside the ranges KeeneticOS reserves
for its own policy-based routing**. keen-manager only ever adds and removes its
*own* rules and chains — it never flushes a built-in chain — so its footprint is
fully reversible and cannot corrupt the firmware's routing policies.

### How capture works

1. A policy route is installed:
   `ip rule add fwmark 0x2333 lookup 993` plus
   `ip route add local default dev lo table 993`, so any packet carrying the mark
   is delivered locally for TPROXY.
2. In the `mangle` table's `PREROUTING`, a jump into `KEENMGR_TPROXY` runs the
   capture rules, which:
   - `RETURN` for Xray's own egress (mark `255`), preventing a routing loop;
   - `RETURN` for every **bypass** destination (RFC1918, loopback, link-local,
     CGNAT `100.64.0.0/10`, multicast, broadcast) so the router, LAN and the
     management UI stay reachable even with a broken tunnel;
   - `TPROXY --on-port 12345 --tproxy-mark 0x2333/0x2333` the rest (TCP + UDP)
     into Xray.
3. The **kill-switch** (`KEENMGR_KILL`, jumped from `FORWARD`) is opt-in: it
   `RETURN`s bypassed ranges and already-marked tunnel traffic, then `DROP`s
   everything else, so nothing leaks when every tunnel is down.

All chain operations use remove-then-add semantics, so applying and reapplying
are idempotent.

```
              LAN client
                  │  (packet)
                  ▼
        mangle PREROUTING ──jump──▶ KEENMGR_TPROXY
                  │                     │
                  │            ┌────────┼───────────────┐
                  │            ▼        ▼               ▼
                  │       mark==255  dst in bypass    else
                  │        RETURN     RETURN       TPROXY :12345
                  │           │         │           (mark 0x2333)
                  │           ▼         ▼               │
                  │      normal fwd  normal fwd         ▼
                  │                              ip rule fwmark 0x2333
                  │                                 → table 993
                  │                                 → local dev lo
                  │                                      │
                  │                                      ▼
                  │                                Xray TPROXY inbound
                  │                                      │
                  │                             outbound (SO_MARK 255,
                  │                              excluded from capture)
                  │                                      │
                  ▼                                      ▼
             WAN (direct)                          tunnel egress → Internet
```

---

## 6. The ndm netfilter reapply hook

**Problem:** KeeneticOS rebuilds iptables from scratch on essentially every
topology change (WAN up/down, reconnect, interface change, policy edit). Any
rules keen-manager installed are silently flushed.

**Solution:** keen-manager registers a hook in `/opt/etc/ndm/netfilter.d/`.
KeeneticOS's `ndm` runs every script in that directory after it (re)builds the
firewall, passing `type` and `table` in the environment. The hook filters for
`type=iptables` + `table=mangle` and then calls, in the background:

```
keen-manager route reapply
```

which re-installs the policy route and the `KEENMGR_*` chains. The **binary owns
the hook contents** (`route.HookScript()`); the installer merely triggers writing
it via `keen-manager install-hook`. If TPROXY is disabled, `route reapply` is a
cheap no-op, so leaving the hook in place is harmless.

```
KeeneticOS topology change
        │
        ▼
firmware rebuilds iptables  (our rules are gone)
        │
        ▼
ndm runs /opt/etc/ndm/netfilter.d/*        env: type=iptables, table=mangle
        │
        ▼
hook: keen-manager route reapply &         (re-adds KEENMGR_TPROXY + policy route)
        │
        ▼
tunnel routing restored
```

---

## 7. Safety model

keen-manager is designed so the worst realistic outcome is a **self-reverting
failed apply**, not a router you have to physically reset.

- **Validate before apply.** Generated configs are checked before they go live —
  `xray -test` for Xray, field validation for AmneziaWG. Invalid config is never
  activated.
- **Back up before every change.** The previous config and a firewall/route
  snapshot are written to `backups/` before any mutation.
- **Rollback deadman.** A risky apply arms a timer that restores the last-known-
  good state unless the change is explicitly confirmed within the timeout. If a
  change breaks connectivity, the device reverts itself.
- **Dry-run-aware mutations.** Every routing/firewall command goes through a
  `platform.Runner` that is dry-run aware, so the same code path is inert in
  tests and off-device, and auditable in logs on-device.
- **LAN-bound UI, outside the tunnel.** The web UI binds to the LAN (default
  port `47115`) and is explicitly excluded from capture via the bypass ranges, so
  a broken VPN/proxy can never lock you out of the manager.
- **Only writes under `/opt`.** No firmware partition is ever touched. Uninstall
  is a matter of removing files under `/opt` (see `scripts/uninstall.sh`).
- **Never flushes firmware chains.** keen-manager adds and removes only the
  chains and rules it owns.

---

## 8. Health probes & the failover chain

keen-manager continuously assesses whether the active path actually works and
switches away from dead ones.

### Probe strategy

Each connection type has an appropriate liveness probe:

- **Xray outbounds** — a SOCKS probe through Xray's local SOCKS inbound (an
  actual request egressed via the outbound), plus TCP reachability of the server
  endpoint. This tests the *whole* path, not just that a port is open.
- **AmneziaWG** — handshake age (time since the last successful WireGuard
  handshake) combined with a TCP ping through the tunnel.
- **Domain reachability** — for nfqws2 strategies, a reachability checker
  confirms that target domains resolve and connect once the bypass is applied.

Xray additionally runs `burstObservatory` + a `leastPing` balancer internally,
so among healthy servers it keeps routing through the lowest-latency one without
manual selection.

### Failover chain

Connections are arranged into an **ordered chain**, e.g.:

```
Xray (best, via balancer)  →  AmneziaWG  →  direct / kill-switch
```

The engine promotes the first healthy node in the chain to active. When the
active node — or its associated nfqws2 strategy — fails its probe, traffic falls
back to the next node; when a higher-priority node recovers, traffic returns to
it automatically. The terminal node can be a plain `direct` egress or the
kill-switch, depending on whether you prefer degraded connectivity or no leaks
when everything upstream is down.

```
        ┌─────────────┐  healthy   ┌──────────────┐  healthy   ┌───────────────────┐
active ▶│ Xray (best) │───────────▶│  AmneziaWG   │───────────▶│ direct/kill-switch│
        └─────┬───────┘            └──────┬───────┘            └───────────────────┘
              │ probe fail                │ probe fail
              ▼                           ▼
        demote, try next            demote, try next
              ▲                           ▲
              └── recover: return ────────┘
```

---

## 9. Build & release pipeline

The `Makefile` at the repo root drives builds:

- `make web` — `npm ci` (fallback `npm install`) then `npm run build`, emitting
  the front-end into `internal/webui/dist` (consumed by `go:embed`).
- `make build` — one target from `GOARCH`/`GOMIPS`, into `./build/keen-manager`.
- `make build-all` — cross-compile `mipsle`, `mips`, `arm64`, `arm` into
  `./build/keen-manager-<arch>`.
- `make dist` — `web` + `build-all`, then gzip each binary to
  `./build/keen-manager-<arch>.gz` for upload as GitHub release assets.
- `make test` / `make vet` / `make clean`.

Version metadata is injected at link time from git
(`git describe --tags --always --dirty`, short SHA, UTC timestamp) into
`internal/version` via `-ldflags -X`, with fallbacks when git is unavailable.

### Install flow

`scripts/install.sh` (piped from `curl`) performs, idempotently:

1. Require Entware (`/opt/bin` must exist).
2. Detect arch (opkg → uname → ELF probe, per section 4).
3. Resolve the newest release tag — highest SemVer over the `/releases` API,
   pre-releases included. The list API is **not** dependably newest-first (it is
   neither SemVer- nor publish-time-ordered — `beta.10` can come back behind
   `beta.9`), so `install.sh` picks the max itself in pure awk rather than
   trusting position `[0]`; it falls back to the `latest/download` redirect only
   when the API is unreachable. Then download `keen-manager-<arch>.gz` for that
   tag (overridable via `KEEN_VERSION`/`KEEN_URL`), verify it is a non-empty
   valid gzip, and install it atomically to `/opt/bin/keen-manager` — a failed
   download leaves any existing install untouched.
4. Install the init script to `/opt/etc/init.d/S99keen-manager`.
5. Best-effort `keen-manager install-hook` to register the ndm reapply hook.
6. Start (or restart, on upgrade) the service and print the LAN web-UI URL.

`scripts/uninstall.sh` reverses this (stop service, remove init script, ndm
hook, and binary), keeping `/opt/etc/keen-manager` unless `--purge` is given.
