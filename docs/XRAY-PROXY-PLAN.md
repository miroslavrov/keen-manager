# Plan — Xray as a single KeeneticOS "Proxy connection" (one exit point)

> RU: Xray должен подключаться к роутеру как ОДНО «Прокси-подключение» KeeneticOS
> (SOCKS5), а не через TPROXY. keen-manager держит локальный Xray с SOCKS-инбаундом
> и регистрирует один интерфейс `ProxyN` → `127.0.0.1:10808`. Смена сервера меняет
> только конфиг Xray под капотом; интерфейс в роутере не трогается. Маршруты вешаются
> на `ProxyN` штатным `dns-proxy route`, как для AWG.

Status: **researched, not yet implemented.** This is the top priority for the next
session. Written session 7 (2026-07-09).

---

## 1. Why (the user's actual requirement)

The user is on an **Xray** subscription (BlancVPN `blanc`, vless/reality). Today an
active Xray connection is captured via **TPROXY** — it works but is **invisible in the
KeeneticOS UI** and is all-or-nothing. The user wants what they get when they add a
proxy by hand in the router:

> **One** connection in KeeneticOS (Other Connections → **Proxy Connections**), whose
> parameters keen-manager changes **under the hood** when the active server changes.
> "One exit point; the manager does all the work behind it."

KeeneticOS has exactly this: the **Proxy client** component (since KeeneticOS 3.9) —
"connection via proxy servers using HTTP, HTTPS and SOCKS5" on the *Other Connections*
page. A proxy connection is a first-class **interface** (type `Proxy`, e.g. `Proxy0`):
it shows in the UI, can be made the primary connection in Connection Policies, and can
be a routing target — i.e. it behaves like the AWG `WireguardN` interface we already
route to.

So the model that satisfies the user:

```
LAN ──(Keenetic dns-proxy route / policy)──▶ Proxy0 (SOCKS5 → 127.0.0.1:10808)
                                                     │
                                          keen-manager local Xray (SOCKS inbound)
                                                     │
                                          active server outbound (vless/reality/…)
                                                     ▼
                                                  Internet
```

- **Proxy0 is created once and never churns.** Its upstream is always
  `127.0.0.1:10808` (keen-manager's local Xray SOCKS inbound).
- **Switching server / "select best" only rewrites the local Xray config** (which
  outbound the SOCKS proxy uses). The Keenetic Proxy interface is untouched → the
  router shows one stable connection whose "parameters change under the hood".
- **Per-service routing becomes Keenetic-native**, identical to AWG: a `dns-proxy
  route` (object-group fqdn) bound to `Proxy0`. The in-Xray split-routing added in
  beta.6/beta.7 is NOT needed in this mode (keep it only for the legacy TPROXY mode).

This unifies both tunnel types under "one exit point = one Keenetic interface":
AWG → `WireguardN`, Xray → `ProxyN`, both routable via the same dns-proxy stack.

---

## 2. Keenetic CLI contract (authoritative — from the 4.1 Command Reference)

Interface type `Proxy` (`Proxy0`, `Proxy1`, …). Config-if commands:

| Command | Meaning | Our value |
|---|---|---|
| `proxy upstream <host> [<port>]` | proxy server address:port | `127.0.0.1 10808` |
| `proxy protocol socks5` | protocol (default `http`) | `socks5` |
| `proxy socks5-udp` | enable SOCKS5 UDP mode (KOS 4.1+) | enable (see DNS/UDP note) |
| `proxy udpgw-upstream <host> [<port>]` | UDP gateway upstream (with socks5-udp) | see note |
| `proxy connect [via <iface>]` | start connecting (default: any interface) | connect |
| `interface security-level (public\|private\|protected)` | zone | `public` (internet egress) |
| interface `up` / `ip global` | admin-up / eligible for default-route policy | up; global best-effort |

Requires the **Proxy client** system component (firmware component, installed from the
web UI: General System Settings → KeeneticOS Update and Component Options → Component
options). The user already has it installed (they added a proxy connection by hand).

DNS/UDP note: the KB warns that DNS via a proxy needs **DoT/DoH** enabled, or UDP
(`socks5-udp`, possibly with a `udpgw-upstream` running badvpn-udpgw). keen-manager's
Xray SOCKS inbound already sets `udp:true` (UDP ASSOCIATE). Start TCP-only (no
`socks5-udp`) for a first cut; validate DNS; then add UDP if needed.

---

## 3. RCI shapes — how to get them EXACTLY (do this first)

The CLI manual documents commands, **not** the RCI JSON. RCI mirrors the CLI, but the
argument nesting must be confirmed on-device. The user **already created a proxy
connection in the UI**, so the fastest authoritative path is a read-back:

```sh
# On the router (RCI is loopback, no auth from the device itself):
curl -s http://localhost:79/rci/show/rc/interface | \
  jq 'to_entries | map(select(.key|test("^Proxy"))) | from_entries'
# or, if you know the name:
curl -s http://localhost:79/rci/show/rc/interface/Proxy0
```

Mirror the returned object shape for the writes. Best-guess starting point (VERIFY
against the read-back before shipping — do not ship guessed shapes to the live router):

```json
{"interface": {"Proxy0": {
  "description": "keen-manager",
  "proxy": {
    "upstream": {"host": "127.0.0.1", "port": 10808},
    "protocol": "socks5",
    "connect": true
  },
  "security-level": "public",
  "up": true
}}}
```

Single-positional commands (`protocol <p>`) are often `{"protocol":"socks5"}`; toggle
commands (`socks5-udp`, `connect`) are often `true`/absent; multi-arg (`upstream host
[port]`) is the one most likely to differ (could be `{"upstream":{"host":…,"port":…}}`
or a flat `{"upstream":"127.0.0.1","port":10808}`). The read-back is authoritative.

Every write goes through `internal/keenetic/client.go` `Post` (RCI answers HTTP 200 even
on error — `findErrorEnvelope` parses the status envelope). Follow with a
`{"system":{"configuration":{"save":{}}}}` save.

---

## 4. Implementation plan (keen-manager)

Keep it dry-run aware and reversible (mirror the native-AWG path in
`engine/awgnative.go`). Suggested slices:

1. **`internal/keenetic/proxyiface.go`** — new RCI helpers (mirror `iface.go`):
   - `FindFreeProxyIndex(ctx, c)` — scan `GET /show/interface/` for the first free
     `ProxyN` (reuse the `parseWireguardIndex` pattern, prefix `Proxy`).
   - `CreateProxyInterface(ctx, c, name, ProxyConfig{Upstream, Port, Protocol, UDP,
     SecurityLevel, Description, Up})` — POST the interface object (shape from §3).
   - `ProxyConnect(ctx, c, name, via string)` / delete via existing
     `DeleteInterface` (works for any interface name).
   - `SetProxyUpstream(ctx, c, name, host, port)` — used on server switch if we ever
     want to re-point (normally NOT needed; upstream is always 127.0.0.1:10808).

2. **`internal/xray/config.go`** — add a SOCKS-only profile for this mode:
   `Options.ProxyConnMode bool` → build a config with the SOCKS inbound + the single
   active server outbound only (no TPROXY inbound, no observatory, no split rules,
   no api/stats needed). Simpler and smaller than the TPROXY config.

3. **`internal/engine/apply.go` (bringUp / bringDown, ConnXray)** — when the device
   supports the Proxy client component (see capability detection below) AND the mode
   is proxy-connection:
   - `bringUp`: ensure local Xray is running with the SOCKS-only config for the active
     server (`xray.Apply`); ensure the single managed `ProxyN` exists and is up (create
     once, record its name like `NativeIfaces`/a new `ManagedProxyIface` in state);
     `ip global` best-effort; save. Do NOT touch TPROXY.
   - Server switch: only `xray.Apply` the new server's SOCKS-only config. Leave ProxyN
     as-is. (This is the "parameters change under the hood" behaviour.)
   - `bringDown`: stop Xray; optionally leave ProxyN (so routes survive) or delete it
     on full teardown/DeleteConnection.
   - `verifyActive` (ConnXray): probe through the local SOCKS as today
     (`health.SOCKSHTTP` 127.0.0.1:10808). Optionally also check the Proxy interface
     status via `GET /show/interface/Proxy0`.

4. **Routes** — make an Xray connection resolve to its `ProxyN` interface so the
   EXISTING dns-proxy path applies (this replaces the beta.6/7 in-xray split for this
   mode):
   - `engine/routes.go` `resolveRouteIface`: for an Xray connection in proxy-conn mode,
     return the managed `ProxyN` name (so `applyRoute` does object-group + dns-proxy
     route → ProxyN, exactly like AWG). Keep `applyXrayRoute` (in-xray split) only for
     the TPROXY fallback mode.

5. **Views / integration** — `engine/connections.go` `integrationOf` (ConnXray): when
   proxy-conn mode, return `Mode:"keenetic-proxy"`, `VisibleInRouter:true`,
   `Interface: ProxyN`, `RoutableTarget:true`, summary "shown as Proxy connection
   ProxyN". `engine/interfaces.go` + `keenetic/interfaces.go`: recognise `Proxy` type
   as routable and reverse-map to the managed Xray connection.

6. **Capability detection** — detect whether the Proxy client component is present.
   Options: try `GET /show/interface/` and see if creating a `Proxy` interface is
   accepted, or look for a component marker in `GET /rc/...`. If absent, fall back to
   the current TPROXY mode and surface a clear hint ("install the Proxy client
   component to get a visible single connection"). Do NOT try to install firmware
   components from keen-manager.

7. **Mode selection** — add a setting (e.g. `Settings.XrayIntegration = "proxy" |
   "tproxy"`, default `"proxy"` when the component exists, else `"tproxy"`). Surface on
   the Settings/Connections UI. Keep TPROXY fully working as the fallback.

State: add `State.ManagedProxyIface map[connID]name` (or a single
`State.ManagedProxyIface string`, since it's one exit point) so the interface can be
reconciled/torn down after a restart. Persisted like `NativeIfaces`.

---

## 5. On-device validation (KeeneticOS 5.1.0, arm64, live beta — don't brick it)

1. Read-back the shape from the user's existing manual proxy connection (§3).
2. With keen-manager: activate an Xray server → confirm a single `ProxyN` appears in
   Other Connections → Proxy Connections, upstream `127.0.0.1:10808`, protocol socks5,
   status connected.
3. Set ProxyN primary in Connection Policies (or make a route to it) → confirm traffic
   egresses via the tunnel (public IP = server location).
4. Switch server / select-best → confirm the SAME ProxyN stays (no new interface), and
   the exit IP changes (Xray config swapped under the hood).
5. Create a Route targeting the Xray connection → confirm a `dns-proxy route → ProxyN`
   is created and only those domains use the tunnel.
6. DNS: verify name resolution through the proxy (enable DoT/DoH if needed, or
   socks5-udp + udpgw).
7. Delete the connection → ProxyN removed, router falls back to WAN.

---

## 6. Open questions / risks

- Exact RCI nesting for `proxy upstream`/`protocol`/`connect` (resolve via §3 read-back).
- Does `socks5-udp` work against Xray's SOCKS UDP-associate directly, or does Keenetic
  require a separate `udpgw-upstream` (badvpn-udpgw)? If the latter, DNS/UDP may need
  DoT/DoH instead. Start TCP-only.
- Does `dns-proxy route` accept a `Proxy` interface as target on 5.1.0? (Very likely —
  it's a normal interface — but confirm.)
- Loopback SOCKS: the router's proxy client connecting to `127.0.0.1:10808` (its own
  keen-manager) — confirm `proxy connect via any` works for loopback (expected).
- Component gate: cleanly detect "Proxy client" presence and fall back to TPROXY.

## 7. Code map (files to touch)
- NEW `internal/keenetic/proxyiface.go` (+ `_test.go`) — Proxy interface RCI.
- `internal/xray/config.go` — SOCKS-only profile (`ProxyConnMode`).
- `internal/engine/apply.go` — ConnXray bringUp/bringDown/verify in proxy-conn mode.
- `internal/engine/routes.go` — resolve Xray→ProxyN so dns-proxy routes apply.
- `internal/engine/connections.go` (`integrationOf`), `internal/engine/interfaces.go`,
  `internal/keenetic/interfaces.go` — recognise/annotate Proxy interfaces.
- `internal/model/model.go` — `ManagedProxyIface`, `Settings.XrayIntegration`.
- `internal/engine/settings.go` + web Settings/Routes — mode toggle + picker labels.
- Docs: fold the result back into ROADMAP + HANDOFF; keep this file as the design ref.
