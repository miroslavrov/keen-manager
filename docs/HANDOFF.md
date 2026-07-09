# Handoff — keen-manager (для следующего агента)

Обновлено: 2026-07-09 (**6-я сессия**). Репозиторий: `github.com/miroslavrov/keen-manager`,
ветка `main`. Коммиты от лица **miroslavrov** (git-credentials уже в песочнице; токен
пользователь передаёт отдельно, в репозиторий НЕ коммитить). Пользователь на **KeeneticOS**
(прошивка отдаёт release-строку `"5.01.C.0.0-1"`, это 5.1.0), arch **arm64**, тестирует
бету на живом роутере — ломать нельзя.

Продукт: один Go-бинарь (демон + REST/JSON + SSE + CLI) со встроенной React/shadcn
мордой. Единый менеджер VPN (Xray/AmneziaWG) + обход DPI (nfqws2) для Keenetic.

---

## 0. Сессии 5–6 — что сделано и что осталось

**Все 4 бага из 4-й сессии ([P0] канал C, [P1] auth, [P1] листинг интерфейсов,
[P1] диагностика select-best) ЗАКРЫТЫ.** Бэкенд сделала 5-я сессия, фронт+бандл+доки+релиз —
6-я. Текущий HEAD main = `<этой сессии>`, следующий тег → **`v0.1.0-beta.5`**. Сборка зелёная:
`go build/vet/test`, `tsc`, 17 smoke-тестов, кросс mipsle — всё exit 0; бандл детерминирован
(Node 24, working==index, CI-гард проходит).

### 5-я сессия — бэкенд (в main)
- **`fix(capabilities)` 9b34603** — `parseKeeneticVersion` теперь понимает канал **C**
  (release/stable) и терпит хвост `-1`/лишние сегменты/ведущий `v`. Каналы: A=0(alpha),
  B=1(beta), C+=2(stable). `isAtLeast501A3`/`isAtLeast5` сравнивают по (major,minor,channel,
  build). Регрессии в `keenetic_test.go`: `5.01.C.0.0-1`→AWG2 true, `5.00.C.0.0-1`→false,
  `5.1.0`→true и т.д. **Это разблокировало нативный AWG2 + DNS-маршруты на прошивке юзера.**
- **`fix(auth)` 03238d7** — хеш пароля морды теперь в 0600-vault (`servers.json`, блок
  `auth`), НЕ в `state.json` (`PasswordHash` остаётся `json:"-"`). `loadAuthFromVault()` на
  старте реинстейтит хеш и self-heal: `auth_enabled && hash==""` → auth off (лечит «фантомный
  локаут»). CLI: `keen-manager passwd <new>`, `keen-manager auth disable|status` (действуют
  после рестарта сервиса). Выключение auth чистит хеш.
- **`feat(interfaces)` d042851** — `keenetic.ListInterfaces` поверх RCI `GET /show/interface/`
  (шейп подтверждён: объект по id интерфейса; поля `id/type/description/state/link/connected/
  security-level/address`). `engine.Interfaces()` → `GET /api/interfaces` (off-device/RCI-down
  → пустой список + `note`, не ошибка). `model.ServiceRoute.TargetIface` + `resolveRouteIface`:
  маршрут может целиться прямо в интерфейс роутера (`"Wireguard0"`), не только в подключение
  keen-manager. `POST /api/routes` принимает `target_iface` ИЛИ `target_conn_id`.
- **`fix(activation)` 16a1012** — `verifyActive` возвращает причину; `Activate` вкладывает её
  и probe-target в ошибку: «the tunnel did not carry traffic to `<target>` (`<reason>`);
  rolled back — … set a different probe target on the Failover page».

### 6-я сессия — фронт + бандл + доки + релиз (в main)
- **`feat(web)` `<этой сессии>`** — довёл до фронта то, что 5-я оставила бэкенд-only:
  - **Пикер цели маршрута (Routes) тянет интерфейсы с роутера вживую** (`api.interfaces()` →
    `GET /api/interfaces`): один дропдаун с двумя группами — «Подключения keen-manager» (AWG) и
    «Интерфейсы роутера» (routable WireGuard, включая созданные в веб-морде Keenetic). Значение
    закодировано `conn:<id>` / `iface:<name>` (`decodeTarget`); интерфейс, который уже
    представлен подключением, дедуплицируется; когда `dns_routing_available=false` — понятный
    хинт вместо пустого пикера. Раньше пикер фильтровал только `type==='awg'` client-side → у
    Xray-only юзера пусто.
  - **Реальные ошибки бэка в тостах.** `api.ts::request()` парсит тело `{"error":…}` и бросает
    настоящий текст → активация (`use-actions.tsx`) и select-best (`SubscriptionsPage.tsx`)
    показывают причину дословно (probe-target + reason / «no reachable server») вместо
    generic-тоста. Это и есть невидимая раньше причина «нет активного подключения» и «ошибка
    на выбрать оптимальный».
  - **Правка устаревшего хинта:** списки делятся на группы **≤300** (было ошибочно ≤100),
    `routes.chunkHint` EN+RU. Бэкенд-константы (`MaxDomainsPerGroup`/`DefaultListSplit`=300)
    были верны — расходился только текст.
  - Пересобран `internal/webui/dist` в том же коммите (CI-гард зелёный).

### Установка (боль юзера — РЕШЕНО, инструкция в README)
Роутер за DPI: **нет рабочего исходящего TLS к CDN релизов GitHub**
(`objects/release-assets.githubusercontent.com` → `curl: (35) reset by peer`), хотя
`raw.githubusercontent.com`/`api.github.com` работают. Ставили оффлайн. У Keenetic **`/tmp`
(прошивочный tmpfs) и `/opt/tmp` (Entware) — РАЗНЫЕ ФС**; класть файлы и запускать из `/opt/tmp`.
SSH: `root@192.168.1.1 -p 222` (у юзера `scp` не завёлся — переносил через KeenOS). Рабочая
команда: `KEEN_URL="file:///opt/tmp/keen-manager-arm64.gz" KEEN_ARCH=arm64 sh /opt/tmp/install.sh`.
`scripts/install.sh` умеет `KEEN_URL` = локальный файл (session 4, 8758659). README (67412d8)
описывает онлайн `curl|sh` и оффлайн-по-SSH.

### Осталось — валидировать на устройстве (разблокировано фиксом [P0])
Теперь, когда `native-awg2=true` детектится, проверить на реальном 5.1.0:
- **AWG-подключение** реально создаёт `WireguardN` (виден в морде Keenetic, есть handshake,
  роутит; удаление сносит интерфейс). Именно этого юзер и ждал от «добавления сервера».
- **Маршруты**: пикер теперь показывает интерфейсы роутера; проверить, что маршрут
  (object-group fqdn + dns-proxy route) применяется и трафик идёт в выбранный интерфейс.
- **Xray-активация** (`blanc`): снять причину провала верификации — теперь тост её показывает.
  Скорее всего DPI рвёт reality/TLS к vless (TCP-пинг проходит, туннель нет) ИЛИ проба к
  `gstatic /generate_204` заблокирована → сменить probe-target на странице Failover
  (поле есть, `FailoverPage.tsx`). При провале bringUp снять `xray -test` + логи `xray run`.
- Из 3-й сессии: подхват `user2.list` демоном; бейдж NFQUEUE/`healthy`; nfqws-guard.

### Полезная инфа от юзера / устройство
- firmware `"5.01.C.0.0-1"` (KeeneticOS 5.1.0), `wireguard=true`, arch arm64 (Entware
  `aarch64-k3.10`). После фикса [P0] `native-awg2` должен детектиться как **true**.
- xray: `/opt/sbin/xray`. nfqws2 из opkg-фидов (`nfqws-keenetic`, `nfqws2-keenetic`, `hoaxisr`).
- Подписка `blanc`, 63 vless (reality + ws), локации Extra Whitelist / Extra Whitelist2. 63/63.
- Морда по умолчанию `:47115`; юзер запускал вручную `KEEN_LISTEN=:8090`.
- Данные/секреты: `/opt/etc/keen-manager` (`state.json`, `servers.json` vault 0600 — теперь и
  хеш пароля, `backups/`, `xray/`). uninstall сохраняет их без `--purge`. Полный сброс морды:
  `stop → rm -rf /opt/etc/keen-manager → start` (или `keen-manager auth disable` — без потери данных).
- **[инфо] Два инстанса демона:** `bind: address already in use` на :8088 = сервис + ручной
  `daemon` одновременно. См. P2 single-instance guard.

---

## 1. Прошлые сессии (история, уже в main)

**3-я сессия — 4 слайса P1 (сборка зелёная, бандл пересобран):**
- `feat(lists)` авто-сплит импортируемых списков на `user.list`/`user2.list`/… по ≤300
  доменов (`nfqws.DefaultListSplit`, совпадает с `keenetic.MaxDomainsPerGroup`).
  `POST /api/nfqws/lists/import` (resolve→split→write→reload).
- `feat(nfqws)` kernel-module readiness (`nfnetlink_queue`/`xt_NFQUEUE`);
  `NfqwsStatusView` += `kernel_ready/missing_modules/healthy`.
- `feat(failover)` nfqws-guard: `failoverTick` гоняет `nfqwsGuardTick`; на прямом пути
  мёртвый обход → фейловер на туннель. `Failover.NfqwsGuard/NfqwsFallbackTo/NfqwsProbeDomains`.
- `feat(web)` фронт под всё выше + пересобранный `internal/webui/dist`.

**ВАЖНО (не потерять!):** сплит доменов = **300, НЕ 100**. `keenetic.MaxDomainsPerGroup`
остаётся 300 (прошивка принимает 300-элементные object-group), `nfqws.DefaultListSplit`=300.
Старое «~100 строк на список» — НЕВЕРНО, не снижать.

**2-я сессия (для истории):** fix(auth) жёсткий gate логина (`RequireAuth`, `no-store`);
страница «Маршруты» (`/routes`, каталог 81 пресета); Integration-панель на detail-sheet
подключения; структурная форма nfqws2 (Форма/Advanced); резолвер удалённых доменных списков
(`internal/listsrc`, `POST /api/lists/resolve`).

Всё device-mutating — dry-run aware, помечено «validate on-device».

---

## 2. Приоритеты (обновлено 6-й сессией)

**Все P0/P1 из 4-й сессии закрыты (см. §0).** Дальше:

1. **On-device валидация (главное)** — теперь `native-awg2=true` детектится, разблокировано:
   AWG-подключение создаёт `WireguardN` (виден/роутит/сносится); маршрут применяется в
   выбранный интерфейс роутера; Xray-активация `blanc` — снять причину из тоста (тост её теперь
   показывает), при DPI-провале сменить probe-target на Failover; user2.list/NFQUEUE-бейдж/
   nfqws-guard. Ломать бету на живом роутере нельзя.
2. **P2 дефолт-пресеты для 6 листов nfqws** (редактор есть, дефолтных доменов нет) — НУЖЕН
   точный список от юзера: какие 6 листов и какие домены в каждом. НЕ выдумывать.
3. **P2:** per-attempt timeout вокруг `Activate()` в failover-цикле; backoff/jitter когда вся
   цепочка лежит; single-instance guard (pidfile/flock); through-tunnel reachability узлов;
   nfqws2 parity (lua/log/самообновление, ISP_INTERFACE autodetect, проверка ndm-хука);
   `RollbackTimeoutS==0`.
4. **P3:** hysteria2/tuic; дашборд live-трафик; CLI-паритет structured nfqws + failover.

---

## 3. Окружение песочницы (важно для сборки/верификации)

- **Go НЕ предустановлен.** Ставить: `curl -fsSL https://go.dev/dl/go1.26.5.linux-amd64.tar.gz`
  → `/agent/workspace/.goroot`; затем `source /agent/workspace/goenv.sh`
  (GOROOT/GOPATH/GOCACHE + `GOFLAGS=-mod=mod`). go.mod требует go 1.26. (В эту сессию Go
  ставился, базовый `go build/vet ./...` — зелёный.)
- **`make` нет** — цели вручную: бандл = `cd web && npm run build` (vite пишет в
  `internal/webui/dist`); `go vet ./...`; `go test ./...`. Кросс:
  `GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0 go build ./cmd/keen-manager`.
- **npm-реестр за firewall!** `registry.npmjs.org` = 403 по умолчанию. До `npm ci` вызови
  `RequestNetworkAccess({domain:"registry.npmjs.org"})` и дождись одобрения.
  `web/package-lock.json` НЕ менять (детерминизм бандла).
- **git-credentials уже настроены** (`~/.git-credentials`, helper=store). Токен НЕ коммитить.
- **gh CLI нет** — статус релизов через GitHub API + `curl` с `Authorization: Bearer $TOKEN`.

### CI-гард (не сломать!)
`.github/workflows/ci.yml` шаг «Verify committed bundle is up to date» делает `make web` и
падает, если `internal/webui/dist` отличается от закоммиченного. **Любое изменение
`web/src/` требует пересобрать бандл и закоммитить `internal/webui/dist` в том же коммите.**
Node в CI = 24; локально Node 24 в песочнице.

---

## 4. Ключевые REST-эндпоинты (актуальный контракт)
Полная таблица роутов — `internal/server/api.go` (Go 1.22 mux, метод+путь).
- Подключения: `GET /api/connections`, `POST /api/connections {type,name,awg_conf?,share_link?}`,
  `GET/PUT/DELETE /api/connections/{id}`, `POST /api/connections/{id}/{action}`
  (action = up|down|activate|test).
- Подписки: `GET /api/subscriptions`, `POST /api/subscriptions`, `.../{id}` PUT/DELETE,
  `POST .../{id}/refresh`, `GET .../{id}/servers`, `POST .../{id}/select-best`.
- Интерфейсы роутера (live с KeenOS): `GET /api/interfaces` → `{interfaces[]{name,label,type,
  up,connected,address,is_wireguard,routable,managed_conn_id},dns_routing_available,note?}`.
- Маршруты: `GET /api/routes`, `POST /api/routes {name,preset_id,domains[],subnets[],
  target_conn_id? , target_iface?}` (нужен ОДИН из target_conn_id / target_iface),
  `GET /api/routes/presets`, `PUT /api/routes/{id}/toggle`, `DELETE /api/routes/{id}`.
- nfqws: `GET /api/nfqws`, `POST /api/nfqws/action`, `GET/PUT /api/nfqws/config[/structured]`,
  `GET/PUT /api/nfqws/lists[/{name}]`, `POST /api/nfqws/lists/import {base,url,attr?,mode?}`,
  `POST /api/nfqws/check-domain`.
- Списки: `POST /api/lists/resolve {url,attr?}`. Failover: `GET/PUT /api/failover`.
  Настройки: `GET/PUT /api/settings`. Kill-switch: `POST /api/killswitch`. Логи:
  `GET /api/logs`. Auth: `GET /api/auth`, `POST /api/login`, `POST /api/logout`. Состояние:
  `GET /api/state` (несёт `active_connection_id`, `connections`, nfqws с
  `kernel_ready/missing_modules/healthy`). SSE: `GET /api/events`.

## 5. RCI-шейпы Keenetic (для справки)
- База `http://localhost:79/rci` (loopback, без auth с самого роутера — подтверждено:
  `show/version` отвечает). Клиент — `internal/keenetic/client.go` (RCI отвечает HTTP 200
  даже на ошибку — тело несёт status-envelope; `findErrorEnvelope` разбирает).
- Save конфига: `{"system":{"configuration":{"save":{}}}}`.
- Список интерфейсов (для новой фичи): `GET /show/interface/` → JSON-объект, ключи = имена
  (`Wireguard0`, …). Статус одного: `GET /show/interface/{name}` (`up`, `wireguard.peer[]`).
- AWG import (нативный): `{"interface":{"wireguard":{"import":…}}}` (см. `keenetic/import.go`,
  `engine/awgnative.go`).
- DNS-маршруты: `{"object-group":{"fqdn":{"km-<slug>":{"include":[{"address":"x"}]}}}}`
  + `{"dns-proxy":{"route":[{"group":"km-<slug>","interface":"WireguardN","auto":true}]}}`.
- ASC (обфускация AWG2 s3/s4) гейтится `Capabilities.SupportsAWG2` — см. баг [P0].

## 6. Карта кода
- `internal/engine/` — ядро: `apply.go` (Activate/bringUp/verify/rollback), `awgnative.go`
  (нативный AWG через RCI), `connections.go`, `subscriptions.go` (SelectBest/fastest),
  `routes.go`, `failover.go`, `nfqws.go`, `settings.go` (auth/login), `vault.go`
  (секреты серверов, servers.json), `views.go` (DTO), `lists.go`.
- `internal/keenetic/` — RCI: `client.go`, `capabilities.go` (баг [P0]), `iface.go`
  (create/peer/asc + FindFreeIndex), `status.go` (InterfaceStatus/PeerStatus), `import.go`,
  `dnsroute.go`, `policy.go`.
- `internal/config/` — `store.go` (state.json, atomic+backup), `defaults.go` (порт 47115,
  auth off). `internal/model/model.go` — State/Settings/Connection/Server.
- `internal/subscription/`, `internal/nfqws/`, `internal/xray/`, `internal/route/`,
  `internal/listsrc/`, `internal/presets/`, `internal/platform/`, `internal/health/`.
- `web/src/pages/` — Dashboard, Connections, Subscriptions, Routes, Bypass, Failover, Logs,
  Settings. `web/src/lib/` — `api.ts` (клиент: `request()` парсит `{error}` и бросает реальный
  текст; mocks только в DEV), `types.ts` (+`RouterInterface`/`InterfacesView`), `mocks.ts`
  (+`mockInterfaces`). `web/src/hooks/use-actions.tsx` и `SubscriptionsPage.tsx` — тосты
  показывают `err.message`. `RoutesPage.tsx` — `TargetPicker` тянет `api.interfaces()`,
  `decodeTarget` кодирует `conn:`/`iface:`.

## 7. Порядок работы для следующего агента
1. Поставь Go (§3), `source goenv.sh`, `go build/vet/test ./...` — зелёно.
2. Все P0/P1 из §0 закрыты — фокус на on-device валидации (§2 п.1) и P2/P3.
3. При правках `web/src/`: `RequestNetworkAccess({domain:"registry.npmjs.org"})` → `npm ci`
   (lock НЕ менять) → `npm run build` → закоммить `internal/webui/dist` в ТОМ ЖЕ коммите
   (иначе CI-гард «Verify committed bundle» упадёт). Node 24. Проверка: после `npm run build`
   `git diff --exit-code internal/webui/dist` должно быть пусто.
4. `go vet/test` + кросс mipsle (`GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0`).
   Коммить логическими слайсами от miroslavrov, пуш в main.
5. Всё device-mutating держи dry-run aware и помечай «validate on-device». Обнови этот
   HANDOFF и ROADMAP в конце сессии. Релиз: тег `vX.Y.Z-<suffix>` (дефис = prerelease) → пуш
   тега → `release.yml` собирает и публикует ассеты. Статус — GitHub API + curl (gh CLI нет).
