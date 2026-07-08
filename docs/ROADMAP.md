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

## P1 — next / ближайшее

- [~] **Native AWG2 traffic routing — validate on-device.** Interface creation +
  bring-up + teardown are wired; the "make it the active internet route"
  step (`ip global`) is best-effort and needs confirmation on a real 5.1.0
  device (RCI field shape can vary by firmware). If traffic doesn't route,
  assign the created `WireguardN` connection a priority in the Keenetic UI — the
  tunnel itself comes up correctly.
- [ ] **nfqws2 structured form in the web UI + API.** The parser exists
  (`internal/nfqws/schema.go`, `Controller.Conf/SaveConf`); expose it via
  `GET/PUT /api/nfqws/config/structured` and a typed form (keep the raw editor
  as an "advanced" tab).
- [ ] **Per-connection fallback chain in the UI.** The model has
  `Connection.FallbackTo`; surface a per-connection fallback picker (VPN → other
  VPN → nfqws strategy → AWG → direct) alongside the global failover chain.
- [ ] **nfqws health → failover signal.** Detect a dead nfqws2 strategy (daemon
  down, or a background probe of known-should-bypass domains) and let it drive a
  fallback, so "strategy died → fall back to AWG" is automatic.
- [ ] **Kernel-module readiness for nfqws2.** Use `platform.KernelModuleDirs()`
  to verify `nfnetlink_queue` / `xt_NFQUEUE` before reporting nfqws2 healthy.

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
