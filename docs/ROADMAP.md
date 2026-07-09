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

## P0 — bugs found on-device (session 4) / баги с устройства

- [ ] **Capabilities parser rejects the "C" release channel.**
  `internal/keenetic/capabilities.go` `parseKeeneticVersion` only knows the `A`/`B`
  channels, so a real device reporting `release="5.01.C.0.0-1"` falls through to the
  `default` branch → `ok=false` → `SupportsAWG2=false`, `SupportsDNSRoute=false`
  (daemon logs `native-awg2=false`). This silently disables native AWG2 interface
  creation and the whole native DNS-routing / Routes path on capable firmware. Fix:
  tolerate the `5.01.C.0.0-1` shape (trim the `-N` tail, ignore extra segments) and
  treat any non-`A` letter channel (`B`/`C`/…) as ≥ beta; AWG2 = `major>5` or
  (`major==5 && minor>=1`, excluding early `5.01.A.0..A.2`). Add regression cases to
  `capabilities_test.go`. **Do this first** — it unblocks items in P1 below on the
  user's actual device.
- [ ] **Web-UI password is a phantom lock-out (auth persistence bug).**
  `model.Settings.PasswordHash` is tagged `json:"-"`, so `config.Store.save()` never
  writes it to `state.json`, but `auth_enabled` *is* persisted. After a daemon
  restart the UI demands a password that can never validate (hash is gone). No CLI to
  reset. Fix: persist the hash in a 0600 vault file, self-heal on load
  (`auth_enabled && hash=="" ⇒ auth off`), and add `keen-manager passwd` /
  `auth disable`.
- [ ] **No router-interface listing.** There is no `GET /api/interfaces`; the Routes
  target picker only filters existing `awg`-type connections client-side, so it is
  empty for Xray-only users and nothing is pulled live from KeenOS. Add
  `keenetic.ListInterfaces` over RCI `GET /show/interface/` → `GET /api/interfaces` →
  a real interface dropdown in the UI. (awg-manager research for the exact `show
  interface` shape did not complete this session — verify against
  github.com/hoaxisr/awg-manager and/or a live `curl localhost:79/rci/show/interface/`.)
- [ ] **Select-best / activation surfaces only a generic error.** Xray activation that
  fails verification (`verifyActive` probe through the tunnel) rolls back to "no active
  connection", and the frontend (`use-actions.tsx`, `SubscriptionsPage` select-best
  mutation) swallows the real backend error into a generic toast. Surface the real
  error, log `xray` stderr on bring-up failure, and make the probe target configurable
  (the default gstatic probe may itself be DPI-blocked). Root-cause the tunnel
  handshake failure on-device (`xray -test` + run logs). Note: Xray connections are
  transparent-proxy and never become router interfaces — this is by design, not a bug.

## P1 — next / ближайшее

- [~] **Native AWG2 traffic routing — validate on-device.** Interface creation +
  bring-up + teardown are wired; the "make it the active internet route"
  step (`ip global`) is best-effort and needs confirmation on a real 5.1.0
  device (RCI field shape can vary by firmware). If traffic doesn't route,
  assign the created `WireguardN` connection a priority in the Keenetic UI — the
  tunnel itself comes up correctly.
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
