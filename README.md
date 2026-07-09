# keen-manager

**English** · [Русский](#keen-manager-русский)

A single, unified control panel for VPN and DPI-bypass on **Keenetic** routers.

keen-manager brings **AmneziaWG**, **Xray** (with subscription support), and
**nfqws2** (DPI bypass / service splitting) under one clean web UI and one CLI —
with automatic best-location selection, health checks, and **fallback chains** so
your connection stays up even when a server is blocked or an operator rotates IPs.

> **Status: beta.** The full stack is implemented and ships as a single
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

> Requires a Keenetic router with USB/NAND storage, **Entware (opkg)**, and the
> firmware components *IPv6 protocol* and *Netfilter subsystem kernel modules*.

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
   TAG=v0.1.0-beta.10   # ← replace with the newest tag listed there
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
- [ ] Xray gRPC hot-reload (swap outbounds without a full restart)
- [ ] Policy-based routing per device group (Keenetic policy fwmark integration)
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

> **Статус: бета.** Весь стек реализован и поставляется одним самодостаточным
> бинарём: ядро на Go (разбор подписок, генерация конфигов Xray, работа с
> конфигами AWG, логика health-check и фолбека), демон (REST/JSON API + SSE +
> встроенная веб-морда), CLI и все семь страниц интерфейса. Готовые бинари под
> все архитектуры роутеров опубликованы на странице
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

> Нужен роутер Keenetic с USB/NAND-накопителем, **Entware (opkg)** и компоненты
> прошивки *Протокол IPv6* и *Модули ядра подсистемы Netfilter*.

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
   TAG=v0.1.0-beta.10   # ← замени на самый свежий тег оттуда
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
- [ ] Горячая перезагрузка Xray через gRPC (смена аутбаундов без полного рестарта)
- [ ] Policy-based маршрутизация по группам устройств (интеграция с fwmark-политиками Keenetic)
- [ ] Упаковка в IPK + хостинг opkg-фида
- [ ] Интеграционные тесты на устройстве

## Отказ от ответственности

Для личного использования. Ты сам отвечаешь за соблюдение применимых к тебе
законов и условий сервисов. Без гарантий — см. лицензию.
