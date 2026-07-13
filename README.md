# keen-manager

**English** · [Русский](#keen-manager-русский)

A single, unified control panel for VPN and DPI-bypass on **Keenetic** routers.

keen-manager brings **AmneziaWG**, **Xray** (with subscription support), and
**nfqws2** (DPI bypass / service splitting) under one clean web UI and one CLI —
with automatic best-location selection, health checks, and **fallback chains** so
your connection stays up even when a server is blocked or an operator rotates IPs.

> **Status: beta (rc.10).** The full stack is implemented and ships as a single
> self-contained binary: the Go core (subscription parsing, Xray config
> generation, AWG config handling, health & failover logic), the daemon
> (REST/JSON API + SSE + embedded web UI), the CLI, and all seven web UI pages.
> Prebuilt binaries for every router architecture are published on the
> [Releases](https://github.com/miroslavrov/keen-manager/releases) page. Device
> actions are built to be safe (validate-before-apply, backups, rollback) but
> transparent-proxy / kill-switch are **off by default** — installing does not
> touch your firewall until you turn them on.

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
  `mips` (big-endian, softfloat), `arm64`, and `arm` (armv7).
- **Front-end:** React + TypeScript + Vite + Tailwind + shadcn/ui, built to
  static assets and embedded via `go:embed`.
- **Cooperates with the firmware:** reinstalls its firewall rules from an
  `/opt/etc/ndm/netfilter.d` hook (Keenetic flushes iptables on every topology
  change), uses fwmarks/tables outside Keenetic's reserved ranges, and never
  flushes the firmware's own chains.

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the details (routing,
tproxy, safety model, arch detection).

## Install (Keenetic + Entware)

### Prerequisites

1. **Keenetic router** with USB/NAND storage and **Entware (opkg)** installed.
   [Entware install guide →](https://github.com/Entware/Entware/wiki)

2. **Firmware components** (install via Keenetic web UI → *General settings* →
   *Component options*):
   - **IPv6 protocol** — needed for Xray to connect to servers that resolve to
     IPv6 (common with European VPN providers; without it, IPv6-only servers
     are unreachable).
   - **Netfilter subsystem kernel modules** — needed for TPROXY capture and
     the kill-switch (provides `xt_TPROXY`, `xt_socket`, `xt_conntrack`,
     `xt_mark`).

3. **Entware packages** (on the router via SSH):
   ```sh
   opkg update
   opkg install curl           # download (auto-installed by the one-liner)
   opkg install ip-full        # full-featured iproute2 (preferred for policy routing)
   ```
   The installer auto-installs `curl` if missing. `ip-full` is optional but
   recommended — without it, keen-manager falls back to the firmware's limited
   `ip` binary (some policy-routing features may not work).

4. **Managed components** (keen-manager drives these but does not bundle them):

   | Component | What it does | How to install |
   |-----------|-------------|----------------|
   | **xray-core** | VLESS/Reality/VMess/Trojan/SS VPN tunnel | Auto-downloaded by keen-manager on first activation (from GitHub). If your ISP blocks GitHub CDN, see [Offline xray install](#xray-wont-start-exec-format-error). |
   | **nfqws2** | DPI bypass (strategy-based packet manipulation) | Install separately: see [`nfqws2-keenetic`](https://github.com/nfqws/nfqws2-keenetic). keen-manager detects and manages it if present. |
   | **tpws** | Socket desync proxy for routable DPI bypass | `opkg install tpws` — only needed if you want the *Bypass* page's routable ProxyN exit point. Without it, nfqws2 still works; tpws is for per-domain routing through a managed proxy interface. |
   | **AmneziaWG** | WireGuard-based VPN with obfuscation | On firmware 5.1.0+, the native AWG2 kernel modules are used (auto-detected). On older firmware, `awg-quick` from Entware. |

There are two ways to install. **Method A** is the one-liner. Use **Method B**
when the router can't reach GitHub's release CDN — some ISPs reset the TLS
connection to `objects.githubusercontent.com` (DPI/RST) even when `raw` and the
API work, which shows up as `curl: (35) … Connection reset by peer` during the
download. Method B copies the files over SSH instead, so the router makes no
outbound connection at all.

### Method A — one-line install (online)

```sh
opkg update && opkg install curl
curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/install.sh | sh
```

The installer detects your architecture, installs the right binary to
`/opt/bin/keen-manager`, sets up the init script and ndm hook, and prints the web
UI address (default `http://<router-lan-ip>:47115`).

### Method B — offline install over SSH (download on a PC, copy to the router)

Use this when the router itself can't download the release. Fetch the files on a
machine that *can* reach GitHub (e.g. behind a VPN), then copy them across.

1. **On your computer**, grab the installer and the binary for your router's
   architecture. Pick the arch from this table (on the router run
   `opkg print-architecture`, or `uname -m`):

   | Keenetic CPU          | asset                    |
   | --------------------- | ------------------------ |
   | ARM64 (aarch64)       | `keen-manager-arm64.gz`  |
   | ARMv7 (arm)           | `keen-manager-arm.gz`    |
   | MIPS little-endian    | `keen-manager-mipsle.gz` |
   | MIPS big-endian       | `keen-manager-mips.gz`   |

   ```sh
   # installer script
   curl -fsSLO https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/install.sh
   # matching binary (arm64 shown — swap for your arch). Betas are pre-releases,
   # so the 'latest/download' shortcut does NOT resolve yet — grab the newest tag
   # from the Releases page: https://github.com/miroslavrov/keen-manager/releases
   TAG=v0.1.0-rc.10   # ← replace with the newest tag from the Releases page
   curl -fsSLO "https://github.com/miroslavrov/keen-manager/releases/download/${TAG}/keen-manager-arm64.gz"
   ```

2. **Copy both to the router.** Put them in **`/opt/tmp`** (Entware's storage),
   over SSH (the Entware SSH login is usually `root` on **port 222**):

   ```sh
   scp -P 222 install.sh keen-manager-arm64.gz root@192.168.1.1:/opt/tmp/
   ```

   > ⚠️ **`/tmp` ≠ `/opt/tmp` on Keenetic.** `/tmp` is the firmware's volatile
   > tmpfs; `/opt/tmp` is Entware on the USB/flash storage. They are **different
   > filesystems**, and file managers (e.g. copying via the KeenOS web UI, which
   > you may need if `scp` won't connect) commonly drop files into `/opt/tmp`.
   > Whatever directory you copy into, use that **same** path in step 3 — putting
   > the files in `/opt/tmp` but running `sh /tmp/install.sh` fails with
   > `can't open '/tmp/install.sh'`.

3. **On the router**, run the installer pointed at the local file — no network is
   used for the download:

   ```sh
   KEEN_URL="file:///opt/tmp/keen-manager-arm64.gz" KEEN_ARCH=arm64 sh /opt/tmp/install.sh
   ```

   `KEEN_URL` accepts a `file://` URL or a plain local path; `KEEN_ARCH` skips
   auto-detection. The installer still writes the init script and ndm hook and
   starts the service, exactly like Method A.

<details>
<summary>Fully manual (no installer script)</summary>

Copy the binary and the init script (`scripts/init.d/S99keen-manager` from this
repo) to the router, then:

```sh
gzip -dc /tmp/keen-manager-arm64.gz > /opt/bin/keen-manager
chmod +x /opt/bin/keen-manager
/opt/bin/keen-manager version                     # sanity check it runs

mkdir -p /opt/etc/init.d
cp /tmp/S99keen-manager /opt/etc/init.d/S99keen-manager
chmod +x /opt/etc/init.d/S99keen-manager

/opt/bin/keen-manager install-hook                # optional: route-reapply hook
/opt/etc/init.d/S99keen-manager start
```
</details>

### Upgrade

- **Method A:** re-run the one-liner — the binary is replaced atomically and the
  service restarts.
- **Method B:** copy the new `keen-manager-<arch>.gz` over and re-run the
  `KEEN_URL=…` command (or, fully manual, overwrite `/opt/bin/keen-manager` and
  run `/opt/etc/init.d/S99keen-manager restart`).

### Manage the service

```sh
/opt/etc/init.d/S99keen-manager start
/opt/etc/init.d/S99keen-manager stop
/opt/etc/init.d/S99keen-manager restart
/opt/etc/init.d/S99keen-manager status
tail -f /opt/var/log/keen-manager.log     # daemon log
```

### Reset all settings

Wipes **every** keen-manager setting — connections, subscriptions, routes,
failover, DPI bypass and the web password — and tears down the managed tunnel and
router interfaces, returning the router to a clean first-run state. The previous
`state.json` is snapshotted into `/opt/etc/keen-manager/backups` first, so a reset
is recoverable; the `nfqws2` package and its lists are left untouched.

- **Web UI:** Settings → *Danger zone* → **Reset everything** (asks to confirm).
- **CLI:** `keen-manager reset --yes`

It is the in-process equivalent of the manual recovery `stop → rm -rf
/opt/etc/keen-manager → start`.

### xray won't start ("exec format error")

`xray config invalid: fork/exec /opt/sbin/xray: exec format error` means the xray
binary at `/opt/sbin/xray` is the wrong CPU architecture (or corrupt) — it cannot
run on this router. keen-manager now verifies the binary and **automatically
reinstalls the correct build** for the device on the next activation.

If your ISP's DPI resets GitHub's release CDN so the auto-download can't complete
(a common Keenetic situation), install xray **offline**: put a matching build on
the router and point keen-manager at it via `KEEN_XRAY_URL`, then restart the
service so the daemon re-provisions:

```sh
# in the daemon's environment (e.g. before starting the service)
export KEEN_XRAY_URL="file:///opt/tmp/Xray-linux-arm64-v8a.zip"
/opt/etc/init.d/S99keen-manager restart
```

`KEEN_XRAY_URL` accepts an `http(s)://` or `file://` URL, pointing at either an
Xray-core release `.zip` or a raw `xray` binary. (`KEEN_XRAY_VERSION` still pins
the version for the default GitHub download.)

### routing fails ("exec format error" on `ip`)

`routing failed …: ip rule add: … fork/exec /opt/sbin/ip: exec format error`
means the Entware iproute2 binary at `/opt/sbin/ip` is the wrong CPU architecture
(or corrupt), so the transparent-proxy policy route can't be installed.
keen-manager now **detects this without running the binary** (it inspects the ELF
header) and falls back to the firmware's own `ip` (`/sbin/ip`) automatically,
logging a one-time hint. To restore the preferred full-featured binary, reinstall
Entware's iproute2:

```sh
opkg update && opkg install --force-reinstall ip-full   # provides /opt/sbin/ip
/opt/etc/init.d/S99keen-manager restart
```

This only affects the Xray-via-TPROXY fallback (used when the KeeneticOS Proxy
client component isn't installed); transparent-proxy and kill-switch are opt-in
and off by default.

### all traffic stopped after enabling VPN

If the entire router loses internet (not just VPN) after activating an Xray
connection, check:

1. **Kill-switch left on after a crash?** Run on the router:
   ```sh
   iptables -L FORWARD -n 2>&1 | grep KEENMGR
   ```
   If you see `KEENMGR_KILL`, the kill-switch chain is still active. Remove it:
   ```sh
   iptables -D FORWARD -j KEENMGR_KILL 2>/dev/null
   iptables -F KEENMGR_KILL 2>/dev/null
   iptables -X KEENMGR_KILL 2>/dev/null
   ```
   Then disable the kill-switch in keen-manager (Settings → kill-switch off)
   before re-activating.

2. **TPROXY rules stuck after a failed activation?**
   ```sh
   iptables -t mangle -D PREROUTING -j KEENMGR_TPROXY 2>/dev/null
   iptables -t mangle -F KEENMGR_TPROXY 2>/dev/null
   iptables -t mangle -X KEENMGR_TPROXY 2>/dev/null
   ip rule del fwmark 0x2333/0x2333 lookup 993 2>/dev/null
   ip route flush table 993 2>/dev/null
   ```

3. **Connector paused?** In the web UI, make sure the connector toggle is ON.
   Or via CLI: `keen-manager connector on`.

4. **ndm race during activation?** If you see
   `No chain/target/match by that name` in the log, ndm flushed iptables
   mid-apply. rc.10+ retries automatically; on older builds, just re-activate
   the connection.

### Uninstall

Stops the service and removes the binary, init script, and ndm hook. Config and
state under `/opt/etc/keen-manager` are **kept** unless you pass `--purge`.

```sh
# online
curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/uninstall.sh | sh

# offline: copy scripts/uninstall.sh to the router, then
sh /tmp/uninstall.sh                # add --purge to also delete config/state
```

<details>
<summary>Fully manual uninstall</summary>

```sh
/opt/etc/init.d/S99keen-manager stop 2>/dev/null
rm -f /opt/etc/init.d/S99keen-manager
rm -f /opt/etc/ndm/netfilter.d/*keen-manager*
rm -f /opt/bin/keen-manager
rm -f /opt/var/run/keen-manager.pid
# optional — wipe config, state, secrets, backups:
rm -rf /opt/etc/keen-manager
```
</details>

## Build from source

```sh
# Front-end -> internal/webui/dist (requires Node 18+)
make web

# Cross-compile all router targets into ./build
make build-all

# Or a single target
make build GOARCH=mipsle GOMIPS=softfloat

# Release artifacts (gzipped per-arch binaries)
make dist
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
- Transparent-proxy (TPROXY) and the kill-switch are **opt-in and off by
  default**; reserved/LAN ranges are always bypassed so the router stays
  reachable even with a dead tunnel.
- It only writes under `/opt`. It never touches firmware partitions.

Still — this is beta software that manipulates routing and firewall rules. Test
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
- [x] Prebuilt release binaries + one-line installer
- [x] TPROXY capture with XKeen-canonical ruleset (conntrack, socket, mark, bypass)
- [x] ndm netfilter hook (route reapply after topology change)
- [x] ndm race retry (chain-vanished during apply → automatic retry)
- [x] Health loop: SOCKS-first probe for active Xray (handles IPv4 TCP broken on router)
- [x] TPROXY verify in activation + health loop (SOCKS ok → verify capture chain)
- [x] On-device selftest (`keen-manager selftest` — xray, SOCKS, TPROXY, nfqws, …)
- [x] Self-update (`keen-manager update` — check GitHub, download, verify, atomic replace)
- [x] Fast reload (`Controller.Reload` — fast process restart, no init-script sleep)
- [ ] Xray gRPC hot-reload (swap outbounds without any process restart)
- [ ] IPK packaging + hosted opkg feed
- [ ] On-device integration tests

## Disclaimer

For personal use. You are responsible for complying with the laws and terms of
service that apply to you. No warranty — see the license.

<br/>

---

# keen-manager (Русский)

[English](#keen-manager) · **Русский**

Единая панель управления VPN и обходом DPI на роутерах **Keenetic**.

keen-manager объединяет **AmneziaWG**, **Xray** (с поддержкой подписок) и
**nfqws2** (обход DPI / разделение сервисов) под одной аккуратной веб-мордой и
одним CLI — с автоматическим выбором лучшей локации, проверкой живости каналов и
**цепочками фолбека**, чтобы соединение оставалось живым, даже когда сервер
блокируют или оператор меняет IP-адреса.

> **Статус: бета (rc.10).** Весь стек реализован и поставляется одним
> самодостаточным бинарём: ядро на Go (разбор подписок, генерация конфигов Xray,
> работа с конфигами AWG, логика health-check и фолбека), демон (REST/JSON API
> + SSE + встроенная веб-морда), CLI и все семь страниц интерфейса. Готовые
> бинари под все архитектуры роутеров опубликованы на странице
> [Releases](https://github.com/miroslavrov/keen-manager/releases). Действия на
> устройстве сделаны безопасно (проверка перед применением, бэкапы, откат), а
> прозрачный прокси / kill-switch **выключены по умолчанию** — установка не
> трогает твой firewall, пока ты сам их не включишь.

---

## Зачем

На Keenetic обычно приходится жонглировать несколькими отдельными инструментами:

- [`hoaxisr/awg-manager`](https://github.com/hoaxisr/awg-manager) для AmneziaWG,
- [`XKeen`](https://github.com/Skrill0/XKeen) / ручной Xray для VLESS/Reality,
- [`nfqws2-keenetic`](https://github.com/nfqws/nfqws2-keenetic) +
  [`nfqws-keenetic-web`](https://github.com/nfqws/nfqws-keenetic-web) для обхода DPI.

У каждого свой интерфейс, свой конфиг, свой сервис. keen-manager — это один
бинарь, который **оркестрирует** их и добавляет недостающую связку: вставляешь
ссылку на подписку, он сам выбирает самую быструю рабочую локацию и
автоматически переключается на запасной канал, когда что-то умирает.

## Возможности

- **Единые подключения.** Управляй туннелями AmneziaWG и аутбаундами Xray рядом
  друг с другом. Один список, один вид статуса, одно место для переключения
  активного канала.
- **Подписки Xray.** Добавь ссылку на подписку (например,
  `https://host/s/<token>`). keen-manager скачает её, разберёт каждый сервер
  (`vless` / `vmess` / `trojan` / `ss`, base64 / Clash YAML / SIP008) и будет
  обновлять по расписанию. Reality + `xtls-rprx-vision` поддерживаются полностью.
- **Лучшая локация — автоматически.** Использует `burstObservatory` Xray +
  балансировщик `leastPing`, чтобы постоянно гнать трафик через рабочий сервер с
  наименьшим пингом — без ручного выбора.
- **Проверка живости.** Каждое подключение проверяется (SOCKS-проба / TCP-пинг /
  возраст handshake для AWG). Мёртвые серверы определяются и пропускаются.
- **Цепочки фолбека.** Задай упорядоченную цепочку, например
  `Xray (лучший) → AmneziaWG → direct/kill-switch`. Если активное подключение —
  или его стратегия nfqws2 — умирает, трафик уходит на следующий узел и
  автоматически возвращается на приоритетный, когда тот восстановится.
- **Интеграция nfqws2.** Управляй существующим сервисом `nfqws2-keenetic`:
  запуск/остановка/reload, смена режима (`AUTO`/`LIST`/`ALL`), правка стратегий и
  хостлистов (`user`/`auto`/`exclude`/`ipset`), плюс проверка доступности
  доменов — тот же функционал, что и у оригинальной веб-морды, в одном месте.
- **Аккуратная веб-морда + CLI.** Фронтенд на shadcn/ui (тёмный, без эмодзи,
  иконки lucide), встроенный в бинарь, плюс скриптуемый CLI для тех же операций.
- **Сделано так, чтобы не окирпичить роутер.** Проверка конфигов перед
  применением, бэкап перед каждым изменением и таймер отката (deadman), который
  восстановит связь, если применение пошло не так. Пишет только в `/opt`
  (Entware).

## Архитектура

Один статически слинкованный бинарь на Go (без runtime-зависимостей) — то, что
нужно для роутеров MIPS/ARM:

```
keen-manager (один бинарь)
├── daemon         REST API + встроенная веб-морда + движок health/failover
├── cli            те же операции, скриптуемо
└── управляет
    ├── Xray       генерирует конфиг из подписок; observatory + балансировщик
    ├── AmneziaWG  разбор/генерация конфига (вкл. обфускацию Jc/S1/H1..); awg-quick
    └── nfqws2     управляет init-скриптом nfqws2-keenetic + конфигом + хостлистами
```

- **Бэкенд:** Go, `CGO_ENABLED=0`, кросс-компиляция под `mipsle` (softfloat),
  `mips` (big-endian, softfloat), `arm64` и `arm` (armv7).
- **Фронтенд:** React + TypeScript + Vite + Tailwind + shadcn/ui, собирается в
  статику и встраивается через `go:embed`.
- **Дружит с прошивкой:** переустанавливает свои правила firewall из хука
  `/opt/etc/ndm/netfilter.d` (Keenetic сбрасывает iptables при каждом изменении
  топологии), использует fwmark/таблицы вне зарезервированных Keenetic
  диапазонов и никогда не флашит цепочки самой прошивки.

Подробности (маршрутизация, tproxy, модель безопасности, детект архитектуры) —
в [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

## Установка (Keenetic + Entware)

### Предварительные требования

1. **Роутер Keenetic** с USB/NAND-накопителем и установленным **Entware (opkg)**.
   [Инструкция по установке Entware →](https://github.com/Entware/Entware/wiki)

2. **Компоненты прошивки** (ставятся через веб-интерфейс Keenetic →
   *Общие настройки* → *Параметры компонентов*):
   - **Протокол IPv6** — нужен, чтобы Xray мог подключаться к серверам,
     резолвящимся в IPv6 (часто встречается у европейских VPN-провайдеров;
     без него IPv6-only серверы недоступны).
   - **Модули ядра подсистемы Netfilter** — нужны для TPROXY-захвата и
     kill-switch (дают `xt_TPROXY`, `xt_socket`, `xt_conntrack`, `xt_mark`).

3. **Пакеты Entware** (на роутере через SSH):
   ```sh
   opkg update
   opkg install curl           # скачивание (авто-ставится установщиком)
   opkg install ip-full        # полноценный iproute2 (предпочтительно для policy-routing)
   ```
   Установщик сам поставит `curl`, если его нет. `ip-full` опционален, но
   рекомендуется — без него keen-manager переключается на ограниченный
   системный `ip` (часть функций policy-маршрутизации может не работать).

4. **Управляемые компоненты** (keen-manager управляет ими, но не включает в бинарь):

   | Компонент | Что делает | Как установить |
   |-----------|-----------|----------------|
   | **xray-core** | VLESS/Reality/VMess/Trojan/SS VPN-туннель | Авто-скачивается keen-manager при первой активации (с GitHub). Если DPI провайдера режет CDN GitHub, см. [Оффлайн-установка xray](#xray-не-запускается-exec-format-error). |
   | **nfqws2** | Обход DPI (стратегийная манипуляция пакетами) | Ставится отдельно: см. [`nfqws2-keenetic`](https://github.com/nfqws/nfqws2-keenetic). keen-manager находит и управляет им, если установлен. |
   | **tpws** | Прокси десинхронизации сокетов для маршрутизируемого обхода DPI | `opkg install tpws` — нужен только если хочешь routable ProxyN exit point на странице *Bypass*. Без него nfqws2 работает; tpws нужен для per-domain маршрутизации через управляемый прокси-интерфейс. |
   | **AmneziaWG** | WireGuard-based VPN с обфускацией | На прошивке 5.1.0+ используются нативные модули ядра AWG2 (авто-детект). На старой прошивке — `awg-quick` из Entware. |

Есть два способа. **Способ A** — одной строкой. **Способ B** нужен, когда роутер
не может достучаться до CDN релизов GitHub — некоторые провайдеры сбрасывают
TLS-соединение до `objects.githubusercontent.com` (DPI/RST), даже когда `raw` и
API работают; в логе это выглядит как `curl: (35) … Connection reset by peer` при
скачивании. Способ B переносит файлы по SSH, так что роутер вообще не делает
исходящих подключений.

### Способ A — одной строкой (онлайн)

```sh
opkg update && opkg install curl
curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/install.sh | sh
```

Установщик определит архитектуру, поставит нужный бинарь в
`/opt/bin/keen-manager`, настроит init-скрипт и ndm-хук и выведет адрес веб-морды
(по умолчанию `http://<LAN-IP-роутера>:47115`).

### Способ B — оффлайн-установка по SSH (скачать на ПК, скопировать на роутер)

Используй, когда сам роутер не может скачать релиз. Файлы качаешь на машине,
которой GitHub доступен (например, под VPN), и переносишь на роутер.

1. **На своём компьютере** скачай установщик и бинарь под архитектуру роутера.
   Архитектуру возьми из таблицы (на роутере: `opkg print-architecture` или
   `uname -m`):

   | CPU Keenetic         | ассет                    |
   | -------------------- | ------------------------ |
   | ARM64 (aarch64)      | `keen-manager-arm64.gz`  |
   | ARMv7 (arm)          | `keen-manager-arm.gz`    |
   | MIPS little-endian   | `keen-manager-mipsle.gz` |
   | MIPS big-endian      | `keen-manager-mips.gz`   |

   ```sh
   # скрипт установщика
   curl -fsSLO https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/install.sh
   # бинарь под свою архитектуру (показан arm64). Бета — это pre-release, поэтому
   # ярлык 'latest/download' пока НЕ срабатывает — возьми самый свежий тег со
   # страницы Releases: https://github.com/miroslavrov/keen-manager/releases
   TAG=v0.1.0-rc.10   # ← замени на самый свежий тег со страницы Releases
   curl -fsSLO "https://github.com/miroslavrov/keen-manager/releases/download/${TAG}/keen-manager-arm64.gz"
   ```

2. **Скопируй оба файла на роутер** в **`/opt/tmp`** (хранилище Entware), по SSH
   (вход Entware-SSH обычно `root` на **порту 222**):

   ```sh
   scp -P 222 install.sh keen-manager-arm64.gz root@192.168.1.1:/opt/tmp/
   ```

   > ⚠️ **`/tmp` ≠ `/opt/tmp` на Keenetic.** `/tmp` — это волатильный tmpfs
   > прошивки, а `/opt/tmp` — Entware на USB/флеше. Это **разные файловые
   > системы**, и файловые менеджеры (например копирование через веб-морду
   > KeenOS — оно может понадобиться, если `scp` не подключается) обычно кладут
   > файлы в `/opt/tmp`. В какой каталог скопировал — тот же путь и указывай в
   > шаге 3: если файлы в `/opt/tmp`, а запускаешь `sh /tmp/install.sh`, получишь
   > `can't open '/tmp/install.sh'`.

3. **На роутере** запусти установщик с локальным файлом — сеть для скачивания не
   используется:

   ```sh
   KEEN_URL="file:///opt/tmp/keen-manager-arm64.gz" KEEN_ARCH=arm64 sh /opt/tmp/install.sh
   ```

   `KEEN_URL` принимает `file://`-URL или обычный локальный путь; `KEEN_ARCH`
   пропускает автодетект. Установщик так же пишет init-скрипт и ndm-хук и
   запускает сервис — как в способе A.

<details>
<summary>Полностью вручную (без установщика)</summary>

Скопируй на роутер бинарь и init-скрипт (`scripts/init.d/S99keen-manager` из
этого репозитория), затем:

```sh
gzip -dc /tmp/keen-manager-arm64.gz > /opt/bin/keen-manager
chmod +x /opt/bin/keen-manager
/opt/bin/keen-manager version                     # проверка, что бинарь запускается

mkdir -p /opt/etc/init.d
cp /tmp/S99keen-manager /opt/etc/init.d/S99keen-manager
chmod +x /opt/etc/init.d/S99keen-manager

/opt/bin/keen-manager install-hook                # опц.: хук переустановки маршрутов
/opt/etc/init.d/S99keen-manager start
```
</details>

### Обновление

- **Способ A:** повтори однострочник — бинарь заменится атомарно, сервис
  перезапустится.
- **Способ B:** перекинь новый `keen-manager-<arch>.gz` и повтори команду с
  `KEEN_URL=…` (или, вручную, перезапиши `/opt/bin/keen-manager` и выполни
  `/opt/etc/init.d/S99keen-manager restart`).

### Управление сервисом

```sh
/opt/etc/init.d/S99keen-manager start
/opt/etc/init.d/S99keen-manager stop
/opt/etc/init.d/S99keen-manager restart
/opt/etc/init.d/S99keen-manager status
tail -f /opt/var/log/keen-manager.log     # лог демона
```

### Сброс всех настроек

Стирает **все** настройки keen-manager — подключения, подписки, маршруты,
фейловер, обход DPI и пароль веб-интерфейса — и снимает управляемый туннель и
интерфейсы роутера, возвращая его к чистому состоянию, как после установки.
Предыдущий `state.json` сначала сохраняется в `/opt/etc/keen-manager/backups`,
поэтому сброс обратим; пакет `nfqws2` и его списки не трогаются.

- **Веб-интерфейс:** Настройки → *Опасная зона* → **Сбросить всё** (с подтверждением).
- **CLI:** `keen-manager reset --yes`

Это внутрипроцессный эквивалент ручного восстановления `stop → rm -rf
/opt/etc/keen-manager → start`.

### xray не запускается («exec format error»)

`xray config invalid: fork/exec /opt/sbin/xray: exec format error` означает, что
бинарь xray в `/opt/sbin/xray` не той архитектуры процессора (или битый) — он не
может выполниться на этом роутере. Теперь keen-manager проверяет бинарь и
**автоматически переустанавливает правильную сборку** для устройства при
следующей активации.

Если DPI провайдера рвёт CDN релизов GitHub и авто-докачка не проходит (частая
ситуация на Keenetic), поставь xray **офлайн**: положи подходящую сборку на роутер
и укажи её keen-manager через `KEEN_XRAY_URL`, затем перезапусти сервис, чтобы
демон переустановил бинарь:

```sh
# в окружении демона (например, перед запуском сервиса)
export KEEN_XRAY_URL="file:///opt/tmp/Xray-linux-arm64-v8a.zip"
/opt/etc/init.d/S99keen-manager restart
```

`KEEN_XRAY_URL` принимает `http(s)://` или `file://`-URL, указывающий либо на
релизный `.zip` Xray-core, либо на «сырой» бинарь `xray`. (`KEEN_XRAY_VERSION`
по-прежнему пинит версию для стандартной докачки с GitHub.)

### маршрутизация падает («exec format error» на `ip`)

`routing failed …: ip rule add: … fork/exec /opt/sbin/ip: exec format error`
означает, что бинарь iproute2 в `/opt/sbin/ip` не той архитектуры процессора
(или битый), поэтому policy-маршрут прозрачного прокси не устанавливается. Теперь
keen-manager **определяет это, не запуская бинарь** (читает ELF-заголовок), и
автоматически переключается на системный `ip` (`/sbin/ip`), один раз записав
подсказку в лог. Чтобы вернуть предпочтительный полнофункциональный бинарь,
переустанови iproute2 из Entware:

```sh
opkg update && opkg install --force-reinstall ip-full   # ставит /opt/sbin/ip
/opt/etc/init.d/S99keen-manager restart
```

Это затрагивает только резервный путь Xray-через-TPROXY (когда не установлен
компонент Proxy-клиента KeeneticOS); прозрачный прокси и kill-switch по умолчанию
выключены.

### весь трафик пропал после включения VPN

Если весь роутер потерял интернет (не только VPN) после активации Xray-подключения,
проверь:

1. **Kill-switch остался включённым после сбоя?** Выполни на роутере:
   ```sh
   iptables -L FORWARD -n 2>&1 | grep KEENMGR
   ```
   Если видишь `KEENMGR_KILL` — цепочка kill-switch ещё активна. Удали:
   ```sh
   iptables -D FORWARD -j KEENMGR_KILL 2>/dev/null
   iptables -F KEENMGR_KILL 2>/dev/null
   iptables -X KEENMGR_KILL 2>/dev/null
   ```
   Затем выключи kill-switch в keen-manager (Настройки → kill-switch выкл)
   перед повторной активацией.

2. **TPROXY-правила зависли после неудачной активации?**
   ```sh
   iptables -t mangle -D PREROUTING -j KEENMGR_TPROXY 2>/dev/null
   iptables -t mangle -F KEENMGR_TPROXY 2>/dev/null
   iptables -t mangle -X KEENMGR_TPROXY 2>/dev/null
   ip rule del fwmark 0x2333/0x2333 lookup 993 2>/dev/null
   ip route flush table 993 2>/dev/null
   ```

3. **Коннектор на паузе?** В веб-интерфейсе проверь, что тумблер коннектора
   включён. Или через CLI: `keen-manager connector on`.

4. **Гонка ndm при активации?** Если в логе видишь
   `No chain/target/match by that name` — ndm сбросил iptables прямо во время
   установки. В rc.10+ это лечится автоматически (retry); на старых сборках —
   просто повтори активацию.

### Удаление

Останавливает сервис и удаляет бинарь, init-скрипт и ndm-хук. Конфиг и состояние
в `/opt/etc/keen-manager` **сохраняются**, если не передать `--purge`.

```sh
# онлайн
curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/uninstall.sh | sh

# оффлайн: скопируй scripts/uninstall.sh на роутер, затем
sh /tmp/uninstall.sh                # добавь --purge, чтобы удалить и конфиг/состояние
```

<details>
<summary>Удаление полностью вручную</summary>

```sh
/opt/etc/init.d/S99keen-manager stop 2>/dev/null
rm -f /opt/etc/init.d/S99keen-manager
rm -f /opt/etc/ndm/netfilter.d/*keen-manager*
rm -f /opt/bin/keen-manager
rm -f /opt/var/run/keen-manager.pid
# опционально — стереть конфиг, состояние, секреты, бэкапы:
rm -rf /opt/etc/keen-manager
```
</details>

## Сборка из исходников

```sh
# Фронтенд -> internal/webui/dist (нужен Node 18+)
make web

# Кросс-компиляция всех целей роутеров в ./build
make build-all

# Или одна цель
make build GOARCH=mipsle GOMIPS=softfloat

# Релизные артефакты (пожатые gzip бинари по архитектурам)
make dist
```

## Безопасность

keen-manager спроектирован так, что худший случай — это **самооткатывающееся
неудачное применение**, а не заблокированный роутер:

- Конфиг **проверяется перед применением** (`xray -test`, проверка полей AWG).
- Каждое изменение **бэкапит** предыдущий конфиг и снапшот firewall/маршрутов.
- **Таймер отката (deadman)** восстанавливает последнее рабочее состояние, если
  изменение не подтверждено в течение таймаута.
- Веб-морда слушает на LAN, **вне** любого туннеля/прокси, которым управляет,
  поэтому сломанный VPN никогда не отрежет тебя от самого менеджера.
- Прозрачный прокси (TPROXY) и kill-switch **включаются вручную и выключены по
  умолчанию**; зарезервированные/LAN-диапазоны всегда идут в обход, так что
  роутер остаётся доступным даже с мёртвым туннелем.
- Пишет только в `/opt`. Никогда не трогает разделы прошивки.

И всё же — это бета-софт, который меняет правила маршрутизации и firewall.
Сначала протестируй на железе, до которого можешь физически дотянуться.

## Лицензия и благодарности

keen-manager под лицензией **MIT** (см. [`LICENSE`](LICENSE)).

Это **оркестратор**: он управляет проверенными компонентами, а не пере-упаковывает
их, что держит лицензирование чистым. На дизайн и интеграцию повлияли эти
проекты под MIT (keen-manager независим и ими не одобрен):

- [`hoaxisr/awg-manager`](https://github.com/hoaxisr/awg-manager) — MIT
- [`nfqws/nfqws2-keenetic`](https://github.com/nfqws/nfqws2-keenetic) — MIT
- [`nfqws/nfqws-keenetic-web`](https://github.com/nfqws/nfqws-keenetic-web) — MIT

Компоненты, которыми keen-manager управляет, но **не** распространяет (ставятся
из своих upstream-репозиториев, на своих условиях): демон `nfqws2`
(из `bol-van/zapret2`), модули/утилиты ядра AmneziaWG и `xray-core`
(XTLS/Xray-core, MPL-2.0). См. [`NOTICE`](NOTICE).

## Дорожная карта

- [x] Доделать страницы веб-морды (дашборд, подключения, подписки, обход, фолбек, логи, настройки)
- [x] Готовые релизные бинари + установщик одной командой
- [x] TPROXY-захват по каноническому XKeen-набору (conntrack, socket, mark, bypass)
- [x] ndm netfilter-хук (переустановка маршрутов после смены топологии)
- [x] Retry при гонке ndm (цепочка пропала во время apply → автоматический retry)
- [x] Health loop: SOCKS-first проба для активного Xray (работает при сломанном IPv4 TCP)
- [x] TPROXY verify при активации + в health loop (SOCKS ок → проверка цепочки захвата)
- [x] Самотест на устройстве (`keen-manager selftest` — xray, SOCKS, TPROXY, nfqws, …)
- [x] Самообновление (`keen-manager update` — проверка GitHub, скачивание, проверка, атомарная замена)
- [x] Быстрый reload (`Controller.Reload` — быстрый рестарт процесса, без sleep init-script)
- [ ] gRPC hot-reload Xray (смена аутбаундов без рестарта процесса)
- [ ] Упаковка в IPK + хостинг opkg-фида
- [ ] Интеграционные тесты на устройстве

## Отказ от ответственности

Для личного использования. Ты сам отвечаешь за соблюдение применимых к тебе
законов и условий сервисов. Без гарантий — см. лицензию.
