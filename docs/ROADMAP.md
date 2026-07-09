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

- [ ] Per-attempt timeout around `Activate()` inside the failover loop so one
  hanging bring-up can't stall the shared health goroutine.
- [ ] Backoff/jitter when the whole failover chain is down (stop hammering every
  server every tick).
- [ ] Through-tunnel reachability for chain nodes (not just TCP/handshake) before
  selecting them.
- [ ] nfqws2 parity: arbitrary list/lua/log file management, self-update /
  version check, `ISP_INTERFACE` auto-detection, verify the ndm netfilter hook
  is installed and fires.
- [ ] hysteria2 / tuic subscription protocols (model + Xray outbound) — not used
  by BlancVPN / 3x-ui today, so lower priority.
- [ ] Honour `RollbackTimeoutS == 0` explicitly (currently silently 90s).
- [ ] Single-instance guard (pidfile/flock) around `state.json`.

## P3 — polish / релиз

- [ ] Dashboard: live traffic counters, quick connection switcher, native-AWG2
  capability badge (firmware is already detected).
- [ ] CLI parity for structured nfqws config + failover editing.
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
