# keen-manager — Roadmap / Дорожная карта

Unified VPN (Xray / AmneziaWG) + DPI-bypass (nfqws2) manager for Keenetic
routers. Single static Go binary = daemon + REST API + SSE + CLI, with an
embedded React/shadcn web UI. Target device: **KeeneticOS 5.1.0** (native AWG2).

Единый менеджер VPN (Xray / AmneziaWG) и обхода DPI (nfqws2) для роутеров
Keenetic. Один статический бинарь = демон + REST API + SSE + CLI со встроенной
веб-мордой на React/shadcn. Целевое устройство — **KeeneticOS 5.1.0** (нативный
AWG2).

---

## Status legend
`[x]` done · `[~]` partial / needs on-device validation · `[ ]` todo

---

## Session 12 — CLI + failover quality + nfqws2 parity + dashboard polish

Code-side polish toward stable (the router LAN is unreachable from the cloud
sandbox, so all on-device P0/P1 validation still belongs to the user). All in
`main`, green: `go build/vet/test` + mipsle/arm64 cross + web bundle (tsc +
vitest 17), CI bundle guard passing.

- [x] **Structured-nfqws CLI parity** (`feat(cli)` 3d7cb2e) — closes the last
  CLI-parity gap. `keen-manager nfqws config` prints the structured
  nfqws2.conf; `nfqws set <field> <value>` edits one typed field (tcp/udp
  ports, policy, nfqueue, log-level, ipv6, and the strategy arg blocks) through
  the same JSON overlay + lossless round-trip the web form uses. Pure
  `ParseConfField`/`ConfFieldHelp`, unit-tested (incl. a reflection guard that
  every field maps to a real Conf json tag).
- [x] **Through-tunnel reachability in select-best** (`feat(engine)` 74b0fd6) —
  the roadmap's "verify chain nodes end-to-end before selecting." SelectBest
  now ranks reachable servers by latency and tries them best-first, verifying
  each through the tunnel with Activate's verify-then-rollback deadman; the
  first that carries traffic wins, so a fast-pinging DPI-dead server no longer
  fails the whole action. Bounded (xray-core pre-ensured once, per-candidate
  verify cap, ≤5 candidates); rolls back to the previous active on total
  failure. Pure `rankByLatency` unit-tested. (The failover chain already did
  this via `activateWithin`.)
- [~] **nfqws2 parity — ISP-interface autodetect + ndm hook status**
  (`feat(engine)` 01877d7). `DetectISPInterface` picks the WAN uplink from the
  RCI interface listing (pure `PickWANInterface`: public, non-tunnel,
  connected, priority), authoritative even when a VPN owns the default route;
  `nfqws detect-isp` / `nfqws set isp-interface auto`. `HookStatus` reports the
  ndm netfilter hook present/wired/binary-present (`route status` CLI +
  `hook_installed` on `/api/state`). **Validate the chosen ISP interface
  on-device.** nfqws2 lua/log file management + self-update/version check still
  TODO.
- [x] **Firmware capabilities + traffic API** (`feat(api)` 9e76753). GET
  /api/health carries a capabilities block (native_awg2 / wireguard /
  proxy_client / dns_route + firmware); new GET /api/traffic returns per-iface
  rx/tx counters from /proc/net/dev (pure `parseProcNetDev`, unit-tested).
- [~] **Dashboard polish** (`feat(web)` 7af8c28). A capabilities bar badges
  native AWG2 / WireGuard / Proxy client / DNS routing / route hook; the WAN
  card shows live Download/Upload throughput (diffs /api/traffic). EN+RU,
  bundle rebuilt in-commit. **Quick connection switcher still TODO.**

---

## Session 11 — release hardening (P2) + install fix + CLI parity

- [x] **Installer resolves the newest release by SemVer** (`fix(install)`
  143209e). `install.sh` took the first `tag_name` from the GitHub `/releases`
  API assuming newest-first; the API is not ordered that way (v0.1.0-beta.10
  came back *behind* beta.9), so installs pulled the stale beta.9. Now pulls
  the whole list and picks the max SemVer in pure awk (busybox-safe). README
  (EN+RU) manual-download URLs and ARCHITECTURE.md updated (the
  `latest/download` shortcut 404s on a pre-release-only repo). Served from
  `main`, so the fix took effect with no new release.
- [x] **`rollback_timeout_s=0` is the explicit default** (`fix(engine)`
  7354bc8). 0/negative → the 90s default; a tiny positive value is clamped to a
  10s floor so a rollback can't fire before one probe cycle completes. Pure
  `normalizeRollbackTimeout`, unit-tested.
- [x] **Single-instance daemon guard** (`feat(daemon)` 9dcf293). An advisory
  flock at `$RUN_DIR/keen-manager.lock` refuses a second daemon (naming the
  holder pid); the kernel releases it on death, so it never goes stale. A
  read-only lock dir is a warning, not fatal. `platform.Lock`, unit-tested +
  verified end-to-end.
- [x] **Failover backoff + jitter when the whole chain is down**
  (`feat(engine)` aaeba95). A "nothing reachable" round arms an exponential
  backoff (30s base, doubling, 5min cap) with full jitter instead of
  re-attempting every server every tick; cleared the instant we switch, the
  active recovers, or the config changes. Pure `backoffDelay`, unit-tested.
- [x] **Per-attempt timeout on failover-driven activations** (`feat(engine)`
  efb9d4c). `Activate` is threaded with a context; loop callers use
  `activateWithin` (rollback budget + 45s) so a hung bring-up (a stuck
  xray-core fetch or a verify that never passes) can't stall the shared health
  goroutine. Interactive activations stay unbounded. Unit-tested (`sleepCtx`,
  verify-on-cancel).
- [~] **CLI parity — failover** (`feat(cli)` 5d2ee2e). `keen-manager failover
  show|on|off|chain|interval|threshold|autoreturn|probe`; `chain` is validated
  against the configured connections (+ the `direct` sentinel). Pure
  `NormalizeFailoverChain`, unit-tested. **Structured-nfqws CLI still TODO.**
- [ ] **P0 on-device (carried, unchanged) — the gate for a true stable
  `v0.1.0`:** exact Proxy RCI shape + the "use for internet" flag read-back,
  and the tpws feed/desync validation. Both need the live router (HANDOFF §0);
  unreachable from the sandbox. Shipped `v0.1.0-rc.1` as the beta→stable bridge
  (believed feature-complete, pending this validation).

---

## Session 10 — nfqws as a routable interface via tpws (P1)

- [~] **DPI bypass exposed as ONE routable KeeneticOS Proxy interface**
  (`feat(tpws)` ddc0a8e + `feat(engine)` 32ac213 + `feat(web)` 6c32b96) —
  the user-clarified P1 ("nfqws like Xray: a routable IP:port, not a global
  inline NFQUEUE"). Implemented in backend + UI, pending on-device
  validation. Mirrors the Xray proxy-connection plumbing 1:1: keen-manager
  runs a local **tpws** (zapret's socket desync proxy) in SOCKS mode on
  `127.0.0.1:10809` and registers ONE managed `ProxyN` → that port
  (`State.ManagedBypassIface`); chosen domains are routed through it
  per-service via the same `dns-proxy route` stack as AWG/Xray. Domains are
  the SAME source as Routes (a route targets the reserved **`bypass`**
  target; picker group "DPI Bypass"); the tpws desync **strategy** lives on
  the Bypass page; default **Discord + YouTube** routes are seeded from
  `internal/presets` on first enable (guarded by `Bypass.Seeded`). SAME
  anti-loop rule as beta.9 — the bypass `ProxyN` is a per-service routing
  TARGET only, never `ip global`. Defensive: an absent tpws binary or Proxy
  client component degrades to a logged hint (never bricks); every device
  effect is dry-run aware. Model: `model.Bypass` + `State.ManagedBypassIface`;
  API `GET/PUT /api/bypass`. Unit-tested (tpws argv/init-script/detection;
  engine sentinel resolution, seed-once, port validation, teardown, view);
  `go build/vet/test` + mipsle/arm64 cross + web bundle (tsc + vitest 17) green.
- [ ] **P0 (still outstanding) — exact Proxy RCI shape + "internet" flag:**
  device read-back not yet captured (HANDOFF §0 command block; do NOT guess).
  `proxyInterfaceBody` remains the isolated best-guess shared by BOTH the Xray
  and bypass Proxy interfaces; a rejected write degrades to a hint/TPROXY.
- [ ] **On-device validation (session 10 — top priority):**
  (a) confirm `tpws` is in the opkg feeds (`opkg update; opkg list | grep -i
  tpws`) and install it — if absent, discuss with the user (no socket proxy →
  no routable bypass interface; global inline NFQUEUE remains but isn't what
  was asked). (b) enable the feature → one `ProxyN` in Other Connections →
  Proxy, upstream `127.0.0.1:10809`, socks5, connected. (c) KEY: tpws SOCKS
  actually desyncs — a domain blocked directly is reachable via
  `curl -x socks5h://127.0.0.1:10809 https://<blocked>/`. (d) a route on
  target "DPI Bypass" tunnels only those domains; the rest stays direct.
  (e) tune the tpws `--split-*/--disorder/--oob/...` strategy for the ISP.
  (f) disable → `ProxyN` removed, tpws stopped, routes kept (pending).

---

## Session 9 — on-device bug fixes (Xray proxy loop; route editing)

- [x] **Routing-loop fix (`fix(engine)`).** The managed `ProxyN` (SOCKS exit
  point) is no longer marked `ip global`/"use for internet access". Marking a
  SOCKS-proxy interface as the default connection looped the router's own DNS +
  Xray server-upstream back through the proxy (no endpoint pinning, unlike WG),
  and TCP-only SOCKS dropped UDP DNS — the on-device "storms / flaps / nothing
  loads / swallows all traffic" report. `ProxyN` is now a per-service routing
  target only (reached via explicit `dns-proxy route`, like AWG). AWG unchanged.
- [x] **Route editing (`feat(routes)`).** `GET /api/routes/{id}` returns the full
  domain/subnet membership; `PUT /api/routes/{id}` (`Engine.UpdateRoute`) edits
  name / domains / subnets / target and re-applies with a clean teardown of the
  old form. Unit-tested. Web editor UI pending the bundle rebuild.
- [x] **Web — Xray integration toggle (`feat(web)`)** landed on Settings
  (auto/proxy/tproxy), from `wip/web-xray-toggle` (branch deleted).
- [x] **Web — route editor (`feat(web)`)**: tap a rule to open a side-drawer with
  its full domain/subnet lists and save. Bundle rebuilt; CI guard green.
- [x] **Released `v0.1.0-beta.9`** (prerelease, assets published).
- [ ] **P0 — Proxy "use for internet" flag / exact RCI shape:** device read-back
  outstanding (see HANDOFF §0 command block; do not guess). Wire the flag at
  LOWEST priority (WAN stays default) + pin a host-route to the active server IP.
- [ ] **P0 — on-device validation** (XRAY-PROXY-PLAN §5); key test
  `curl -x socks5h://127.0.0.1:10808 https://api.ipify.org` = does the tunnel
  actually carry traffic (vs. DPI blocking reality/TLS).
- [ ] **P1 — nfqws as a routable "interface" (user-clarified).** User wants nfqws
  exposed like Xray (an IP:port that becomes a routable interface), not global
  inline NFQUEUE. Design: use `tpws` (zapret's socket-level desync proxy) on
  `127.0.0.1:<port>` → register ONE managed Keenetic `ProxyN` for it (reuse
  proxyiface.go + the proxyconn.go pattern) → Routes bind to it. Domains share the
  Routes source (add the bypass interface as a Routes target); default seed =
  Discord + YouTube presets from `internal/presets`. Verify `tpws` is in the opkg
  feeds on-device. Same anti-loop rule: do NOT mark it global.

## Session 8 — Xray as a single KeeneticOS "Proxy connection" (one exit point)

- [~] **Xray wired as ONE visible KeeneticOS Proxy connection** (SOCKS5),
  the user's actual goal — implemented in the backend, pending on-device
  validation. keen-manager registers one managed `ProxyN` → its local Xray
  SOCKS inbound `127.0.0.1:10808`; switching server / "select best" rewrites
  ONLY the Xray config under the hood, so the router keeps showing one stable
  connection. Per-service routing binds to `ProxyN` via the same `dns-proxy
  route` stack as native AWG (the in-Xray split from beta.6/7 is used only in
  the TPROXY fallback now). New `Settings.XrayIntegration` (auto|proxy|tproxy,
  default auto → proxy when the Proxy client component is present, else tproxy);
  `State.ManagedProxyIface`. TPROXY stays fully working as the fallback, and a
  rejected Proxy-interface RCI write degrades to it automatically (logged) so a
  wrong shape can never strand the router. Slices: `xray` SOCKS-only config,
  `keenetic` Proxy-interface RCI + capability, `engine` apply/routes/connections
  wiring. All unit-tested; `go build/vet/test` + mipsle/arm64 cross green.
- [ ] **On-device validation (session 8 — top priority):** (a) read back the
  real RCI shape of the user's hand-made Proxy connection
  (`curl .../rci/show/rc/interface/Proxy0`) and reconcile it with
  `keenetic/proxyiface.go::proxyInterfaceBody` (best-guess shape — the ONE spot
  to correct); (b) activate an Xray server → confirm a single `ProxyN` appears
  in Other Connections → Proxy, upstream `127.0.0.1:10808`, socks5, connected;
  (c) switch server → same `ProxyN`, exit IP changes; (d) a Route on the Xray
  connection creates `dns-proxy route → ProxyN` and only those domains tunnel;
  (e) delete the last Xray connection → `ProxyN` removed. See
  `docs/XRAY-PROXY-PLAN.md` §5.
- [ ] **Web Settings toggle for Xray integration** (auto/proxy/tproxy): page +
  types + i18n + mocks are written and pushed to branch **`wip/web-xray-toggle`**
  (the embedded bundle could NOT be rebuilt this session — npm registry blocked
  in the sandbox — so the branch must NOT be merged as-is). To land: approve
  `registry.npmjs.org`, `npm ci` + `npm run build` (Node 24), then commit
  `web/src` + the rebuilt `internal/webui/dist` in one `feat(web)` commit on main
  (CI guard) and delete the branch. The backend works without it (auto-detect +
  `PUT /api/settings {xray_integration}`).

---

## Session 7 — Xray activation fix + per-service Xray routing + one exit point

- [x] **Xray activation no longer fails on config format** (fix 572827c). The
  pre-apply temp config is written as `config.json.tmp`; Xray infers a config's
  format from its extension, so `xray -test -config config.json.tmp` bailed with
  *"Failed to get format of …/config.json.tmp"* — the on-device "xray config
  invalid" activation failure the user hit. `Controller.Validate` now passes
  `-format json` explicitly (keen-manager only ever emits JSON). The `.tmp`
  suffix is kept on purpose so `xray run -confdir` (which only merges
  `*.json/*.yaml/*.toml`) never loads a half-written temp; a stale temp is
  cleared before each write. Unit-tested (`internal/xray/control_test.go`).
- [x] **Xray single-server config no longer fails dependency resolution** (fix
  4adcb72). Once configs actually loaded, activation/select-best hit
  *"core: not all dependencies are resolved."* — the `api` block always listed
  `ObservatoryService`, but the observatory only exists in balancer mode, so a
  pinned single-server config advertised a gRPC service with no backing feature.
  `api.services` is now built from the features actually present (Handler/Stats/
  Routing always; Observatory only with the balancer). Regression-tested.
- [x] **Routes can target an Xray connection, not only AWG** (feat a9fbffe).
  When one or more enabled routes target an Xray connection, its active config
  is built in **split-tunnel** mode: only the routed domains/subnets egress
  through the server outbound and everything else falls through to a `direct`
  catch-all. With no routes it stays a full tunnel (unchanged). Domains map to
  Xray's `domain:` matcher (matches sub-domains); already-prefixed matchers pass
  through. Applying/removing a route on the *active* Xray connection rebuilds →
  re-validates → restarts Xray; on a non-active connection it stays pending and
  is compiled in at activation. Unit-tested (config generation + engine
  membership). **This is "routes work with Xray, not just AWG."**
- [x] **One exit point for native AWG** (feat a9fbffe). After a successful
  switch, the *previously* active connection's `WireguardN` interface (and any
  routes pinned to it) is torn down, so trying subscription locations no longer
  piles up interfaces on the router. Done only post-verify, so a failed switch
  never removes the working tunnel — rollback restores the previous one instead.
- [~] **Routes target picker offers Xray tunnels** (web). The Routes dropdown
  gains an "Xray tunnels" group alongside keen-manager AWG connections and live
  router interfaces; the DNS-routing warning is suppressed when only Xray targets
  exist (Xray routes don't use the dns-proxy stack). Needs the embedded bundle
  rebuilt + committed (blocked on npm registry access in the sandbox).
- [ ] **On-device validation (session 7 items):** confirm on the user's 5.1.0 /
  BlancVPN Xray subscription that (a) activation now passes `xray -test` and the
  tunnel carries traffic; (b) a route on the Xray connection sends only those
  services through it while the rest stays direct; (c) switching AWG locations
  leaves exactly one `WireguardN` on the router.
- [ ] **🔴 NEXT (top priority): Xray as a single KeeneticOS "Proxy connection".**
  The user's real goal — an Xray subscription should appear as ONE visible,
  stable connection in the router UI whose parameters change under the hood on
  server switch (not invisible TPROXY). KeeneticOS "Proxy client" (SOCKS5,
  interface type `Proxy`) is exactly this: register one `ProxyN` → keen-manager's
  local Xray SOCKS inbound `127.0.0.1:10808`; server switch rewrites only the
  Xray config; routes bind to `ProxyN` via the same dns-proxy stack as AWG. Full
  design, CLI/RCI contract, slices and validation in **`docs/XRAY-PROXY-PLAN.md`**.

---

## Done — landed this iteration / Сделано

- **Subscriptions parse like a native client.** base64 / plain / Clash-YAML /
  SIP008 lists + individual vless(reality/xtls/ws/grpc)/vmess/trojan/ss links.
  Verified against the real BlancVPN subscription: **63/63 vless links parsed**
  (reality + ws), locations extracted, quota + update-interval headers read.
  Subscriptions now auto-name from the `profile-title` header (base64-aware),
  like v2rayNG / Happ.
- **Native AWG2 on KeeneticOS 5.1.0 is wired.** The RCI client (previously dead
  code) now drives the apply pipeline: firmware/capabilities are detected at
  startup; AWG tunnels are provisioned by importing a full `.conf` over RCI
  (`{"interface":{"wireguard":{"import":…}}}`) so NDMS parses the whole
  obfuscation set (jc…h4, s1-s4, i1-i5) natively; falls back to Entware
  `awg-quick` on older firmware. Every native mutation is reversible and guarded
  by the existing verify-then-rollback deadman, so a misconfiguration can never
  strand the router.
- **Uninterrupted connection.** Boot reconciliation re-establishes the active
  connection after a restart/reboot. Failover engine + auto-best-by-ping already
  worked; fixed a real bug where AWG nodes were never considered reachable
  (TCP-ping on a UDP endpoint) — AWG liveness/fallback now uses the WireGuard
  handshake.
- **Xray actually installs.** xray-core is downloaded for the device arch
  (mipsle/mips/arm64/arm), extracted to `/opt/sbin/xray`, verified, and given an
  `S99xray` init script for autostart. TPROXY routing was already real.
- **nfqws2 structured config.** A quote-aware, multiline-safe parser exposes the
  real conf keys (interface, TCP/UDP ports, policy, custom/quic/udp args, mode,
  …) as typed fields with **lossless round-trip** render (untouched keys and
  comments are preserved byte-for-byte).
- **UI resilience + RU/EN i18n** (from the prior iteration): every route is
  wrapped in an error boundary and pages are null-safe, so one failing tab no
  longer blanks the whole app. Shipped by rebuilding the embedded bundle on
  release.

---

## Done — landed this iteration (web P1 + fixes)

- [x] **Routes / «Маршруты» web UI (`/routes`).** New nav entry + page: the
  81-service preset catalog grouped by category with search + multi-select, a
  target-connection picker (native AWG interfaces only), an active-routes list
  with per-route enable/apply/delete, a custom domains/subnets route builder, and
  a **remote list importer** (see below). Bilingual (`i18n/pages/routes.ts`).
- [x] **Integration panel on the connection detail sheet.** Renders
  `GET /api/connections/{id}` → `integration`: a "visible in router / not an
  interface" badge, the native `WireguardN` name, mode (native/userspace/proxy)
  and whether it can back a route — the direct answer to "I added a sub and see
  nothing in the router UI".
- [x] **Structured nfqws2 form** on the Bypass page. `GET/PUT
  /api/nfqws/config/structured` typed fields (mode AUTO/LIST/ALL, TCP/UDP ports,
  strategy arg blocks, policy, NFQUEUE, log level, IPv6) with the raw editor kept
  under an **Advanced** sub-tab. Lossless round-trip preserved by the backend.
- [x] **Per-connection fallback picker** — confirmed already shipped on the
  Connections row dropdown (`fallback_to` via `PUT /api/connections/{id}`).
- [x] **Remote domain-list import (v2fly / plain / hosts).** New `internal/listsrc`
  resolver + `POST /api/lists/resolve`: normalises GitHub blob→raw URLs, follows
  `include:` recursively with a cycle guard, honours `@attribute` filters, and
  flattens to a deduped domain set. Surfaced in the Bypass → Hostlists "Import
  from URL" dialog (append/replace) and in Routes → Custom & import.
- [x] **Login hard-gate fix.** `RequireAuth` no longer renders the protected
  tree while unauthenticated (was a soft gate + 30s stale cache → "sometimes lets
  me in without a password"); auth state is now revalidated fresh, and auth
  endpoints send `no-store`.

## Fixed — session 5 (backend) + session 6 (frontend) / было P0 из 4-й сессии

All four on-device bugs from session 4 are **fixed and in main**; the shipping
`5.01.C.0.0-1` firmware now detects `native-awg2=true`, which unblocks the P1
validation items below.

- [x] **Capabilities parser now understands the "C" release channel** (fix 9b34603).
  `parseKeeneticVersion` strips a leading `v` / trailing `-N` revision / extra
  segments and treats channel letters A=alpha, B=beta, C+=stable, so `5.01.C.0.0-1`
  (and `5.1.0`, `v5.1.0`, …) parse as ≥ `5.01.A.3` → `SupportsAWG2`/`SupportsDNSRoute`
  true. Regression cases cover C-channel, older-major C strings, and the plain dotted
  form. This restored native AWG2 + DNS routing on the user's device.
- [x] **Web-UI phantom lock-out fixed** (fix 03238d7). The PBKDF2 hash is persisted in
  the 0600 vault (`servers.json`, `auth` block) — never `state.json`. `loadAuthFromVault`
  reinstates it at startup and self-heals `auth_enabled && hash=="" ⇒ auth off`, so the
  UI is always reachable. New CLI `keen-manager passwd <new>` and `auth disable|status`
  for headless recovery.
- [x] **Router-interface listing shipped** (feat d042851 + session-6 web). `keenetic.
  ListInterfaces` over RCI `GET /show/interface/` → `GET /api/interfaces`; the Routes
  target picker now lists live router interfaces (routable WireGuard, incl. ones made
  in the Keenetic UI) grouped with keen-manager AWG connections. A route can bind
  directly to an interface via `target_iface`. Off-device returns an empty list + note.
- [x] **Select-best / activation now surface the real reason** (fix 16a1012 + session-6
  web). `verifyActive` returns the probe failure; `Activate` folds the probe target +
  reason into the error; `api.ts` parses the `{"error":…}` body so activation and
  select-best toasts show the cause verbatim (rollback reason / "no reachable server")
  instead of a generic toast. The probe target is editable on the Failover page. Note:
  Xray connections are transparent-proxy and never become router interfaces — by design.
  Root-causing the specific handshake failure (DPI vs. probe) remains an on-device task.

## P1 — next / ближайшее

- [~] **Native AWG2 traffic routing — validate on-device (now unblocked).** The
  channel-C capabilities fix means `native-awg2=true` is finally detected on the
  user's 5.1.0 firmware, so interface creation is no longer silently disabled.
  Interface creation + bring-up + teardown are wired; the "make it the active
  internet route" step (`ip global`) is best-effort and needs confirmation on a
  real device (RCI field shape can vary by firmware). If traffic doesn't route,
  assign the created `WireguardN` connection a priority in the Keenetic UI — the
  tunnel itself comes up correctly. **This is the "adding a server creates a
  router interface" behaviour the user asked for; validate it end-to-end.**
- [~] **Validate the Routes / DNS path on-device.** `object-group fqdn` +
  `dns-proxy route` shapes are ported from awg-manager but unverified on live
  5.1.0. **Chunking stays at 300/group** (`keenetic.MaxDomainsPerGroup`) —
  confirmed the firmware accepts 300-entry groups, so the earlier "~100-line
  per-list" note was wrong and no lowering is needed. Still to confirm on-device:
  that a route actually applies and that `WireguardN` shows up as a routable
  interface.
- [x] **nfqws health → failover signal.** `failoverTick` runs an nfqws-bypass
  guard first: on the direct path, a dead bypass (daemon down, NFQUEUE modules
  missing, or every probe domain failing directly) fails over to a configured
  tunnel. Model `Failover.NfqwsGuard/NfqwsFallbackTo/NfqwsProbeDomains`; UI on
  the Failover page. On-device switch behaviour still to validate.
- [x] **Kernel-module readiness for nfqws2.** `nfqws.KernelModulesStatus` checks
  `nfnetlink_queue` / `xt_NFQUEUE` (loaded via /proc/modules or loadable under
  `platform.KernelModuleDirs()`); `NfqwsStatusView.healthy` now means installed
  && running && kernel-ready, surfaced as a warning badge in the UI.
- [x] **Auto-split imported lists into ≤300-line hostlists.** `POST
  /api/nfqws/lists/import` resolves + splits a flat set across `user.list`,
  `user2.list`, … (≤300 each, matching the object-group cap), pruning stale
  siblings; the Bypass import dialog now uses it. Reload/pickup of `user2.list`
  by the daemon still to confirm on-device.

## P2 — robustness & parity / надёжность и паритет

- [x] Per-attempt timeout around `Activate()` inside the failover loop so one
  hanging bring-up can't stall the shared health goroutine. (session 11, efb9d4c)
- [x] Backoff/jitter when the whole failover chain is down (stop hammering every
  server every tick). (session 11, aaeba95)
- [x] Through-tunnel reachability for chain nodes (not just TCP/handshake) before
  selecting them. (session 12, 74b0fd6 — select-best; the failover chain already
  verified via activateWithin.)
- [~] nfqws2 parity: `ISP_INTERFACE` auto-detection + ndm netfilter hook status
  landed (session 12, 01877d7). Still TODO: arbitrary list/lua/log file
  management, self-update / version check, and confirming the hook actually
  *fires* on-device (presence/wiring is now checked).
- [ ] hysteria2 / tuic subscription protocols (model + Xray outbound) — not used
  by BlancVPN / 3x-ui today, so lower priority.
- [x] Honour `RollbackTimeoutS == 0` explicitly (was silently 90s). (session 11, 7354bc8)
- [x] Single-instance guard (pidfile/flock) around `state.json`. (session 11, 9dcf293)

## P3 — polish / релиз

- [~] Dashboard: live traffic counters + native-AWG2 capability badge landed
  (session 12, 7af8c28). Quick connection switcher still TODO.
- [x] CLI parity for structured nfqws config + failover editing. (failover
  session 11, 5d2ee2e; structured nfqws session 12, 3d7cb2e)
- [ ] Commit a freshly built embedded bundle for `go build`-from-source users
  (CI already rebuilds it on release).

---

## On-device validation checklist (KeeneticOS 5.1.0) / Проверка на устройстве

Because native RCI mutations can't be tested off-device, verify on a real
router:

1. `keen-manager status` shows the detected firmware release and
   `native-awg2=true`.
2. Add a subscription, activate a server, confirm `/api/state` shows it `up` and
   traffic egresses through the tunnel.
3. For an AWG connection: confirm a `WireguardN` interface appears in the
   Keenetic UI, has a recent handshake, and routes traffic (assign a priority if
   needed). Deleting the connection removes the interface and falls back to WAN.
4. Reboot the router; confirm boot reconciliation brings the tunnel back.
5. Kill the active server; confirm failover switches to the next chain node,
   including an AWG fallback.
