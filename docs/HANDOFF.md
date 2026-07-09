# Handoff — keen-manager (для следующего агента)

Обновлено: 2026-07-09 (**4-я сессия**). Репозиторий: `github.com/miroslavrov/keen-manager`,
ветка `main`. Коммиты от лица **miroslavrov** (git-credentials уже в песочнице; токен
пользователь передаёт отдельно, в репозиторий НЕ коммитить). Пользователь на **KeeneticOS**
(прошивка отдаёт release-строку `"5.01.C.0.0-1"`, это 5.1.0), arch **arm64**, тестирует
бету на живом роутере — ломать нельзя.

Продукт: один Go-бинарь (демон + REST/JSON + SSE + CLI) со встроенной React/shadcn
мордой. Единый менеджер VPN (Xray/AmneziaWG) + обход DPI (nfqws2) для Keenetic.

---

## 0. ЭТА (4-я) сессия — что сделано и что НАЙДЕНО

### Сделано (в main)
- **`feat(install)` 8758659** — `scripts/install.sh` теперь принимает `KEEN_URL` как
  локальный файл (`file:///abs/path` или обычный `/abs/path`): `download()` копирует его
  напрямую, без сети. Плюс `--connect-timeout/--retry` на удалённых загрузках. Разблокирует
  установку на роутере, у которого нет рабочего внешнего TLS.
- **`docs(readme)` 67412d8** — README: два способа установки (A: онлайн `curl|sh`;
  B: оффлайн по SSH — скачать на ПК, перенести, поставить из локального файла), таблица
  архитектур, полностью-ручной путь, управление сервисом, удаление (онлайн/оффлайн/ручное),
  EN+RU.
- **HEAD main = 67412d8.** Последний тег = `v0.1.0-beta.4`. Бинарь на роутере = **beta.4**
  (commit 4228d84). Правок Go/веба в эту сессию НЕ было → бандл пересобирать не требовалось.

### Контекст установки (боль пользователя, важно)
Роутер за DPI: **нет рабочего исходящего TLS к CDN релизов GitHub**
(`objects/release-assets.githubusercontent.com`) — `curl: (35) reset by peer`, хотя
`raw.githubusercontent.com` и `api.github.com` в том же прогоне работают. Ассеты релиза
целы (проверено sha256). Ставили оффлайн: бинарь на ПК → перенос на роутер → локальная
установка. У Keenetic **`/tmp` (прошивочный tmpfs) и `/opt/tmp` (Entware) — РАЗНЫЕ ФС**;
файлы клали в `/opt/tmp`. SSH: `root@192.168.1.1 -p 222`. `scp` у юзера не завёлся —
переносил через KeenOS. Рабочая команда:
`KEEN_URL="file:///opt/tmp/keen-manager-arm64.gz" KEEN_ARCH=arm64 sh /opt/tmp/install.sh`.

### НАЙДЕННЫЕ БАГИ (по коду + логам устройства) — ещё НЕ пофикшены

**[P0] Детект возможностей не понимает канал «C».**
`internal/keenetic/capabilities.go` → `parseKeeneticVersion("5.01.C.0.0-1")` знает только
каналы `A`/`B`, а на `C` (и на формат с лишними сегментами + хвостом `-1`) уходит в
`default` → `ok=false` → `SupportsAWG2=false` и `SupportsDNSRoute=false`. В логе демона:
`native-awg2=false`. Следствие: нативный AWG2 заглушён (`useNativeAWG()==false` в
`engine/awgnative.go`), DNS-маршруты заглушены (`dnsRoutingAvailable()==false` в
`engine/routes.go`) → интерфейсы не создаются, пикер маршрутов пуст.
*Фикс:* сделать `parseKeeneticVersion` устойчивым (обрезать хвост после `-`, игнорировать
лишние сегменты, любой буквенный канал не-`A`, т.е. `B`/`C`/… трактовать как ≥ бета).
Правило для `isAtLeast501A3`: true при `major>5` или (`major==5 && minor>=1`, кроме ранних
`5.01.A.0..A.2`). Обновить `capabilities_test.go`: `"5.01.C.0.0-1"`→true, `"5.1.0"`→true,
`"5.01.A.2"`→false, `"5.01.A.3"`→true, `"5.0.x"`→false. **Чинить ПЕРВЫМ** — разблокирует
AWG + маршруты + пикер интерфейсов.

**[P1] Пароль морды «фантомный» — баг персистентности.**
`model.Settings.PasswordHash` имеет `json:"-"` (`internal/model/model.go:226`) → значит
`config.Store.save()` (`internal/config/store.go`, `json.MarshalIndent(state)`) хеш в
`state.json` НЕ пишет. А `auth_enabled` пишется (`json:"auth_enabled"`). Итог: задал пароль
→ сессия работает («login successful» в логах); после рестарта демона `auth_enabled=true`,
хеша нет → `Login()` всегда «invalid password» → локаут на любой пароль. CLI сброса нет.
`/opt/etc/keen-manager` переживает переустановку (uninstall без `--purge`).
*Фикс:* (a) хранить хеш в 0600-файле как `servers.json` (vault); (b) self-heal: при загрузке
если `auth_enabled && PasswordHash==""` → auth off; (c) CLI `keen-manager passwd <new>` и
`keen-manager auth disable`. Дефолт: порт 47115, auth off (`internal/config/defaults.go`).
Пока обходится: `stop → rm -rf /opt/etc/keen-manager → start`.

**[P1] НЕТ листинга интерфейсов роутера (фича, которую ждёт юзер).**
Юзер хочет выпадающий список интерфейсов, тянущийся с KeenOS. Сейчас пикер на Routes
(`web/src/pages/RoutesPage.tsx` → `TargetPicker`) фильтрует `connections` по `type==='awg'`
(client-side), а НЕ интерфейсы роутера; эндпоинта `GET /api/interfaces` нет вовсе (см.
таблицу роутов `internal/server/api.go`). У юзера только Xray → пикер пуст
(`routes.noTargets`).
*Фикс:* добавить `keenetic.ListInterfaces(ctx,c)` поверх RCI `GET /show/interface/` (ответ —
JSON-объект: ключи = имена интерфейсов, значения = объекты; см. `FindFreeIndex` в `iface.go`
и `InterfaceStatus` в `status.go` как основу парсинга) → `engine.Interfaces()` DTO →
`GET /api/interfaces` → дропдаун во фронте (Routes/Connections/Settings). Отдавать
имя/описание/тип/up/link/адрес. **NB:** ресёрч по awg-manager в эту сессию НЕ доехал
(фоновый агент упал) — точный RCI-шейп `show interface` подтвердить по awg-manager
(github.com/hoaxisr/awg-manager) и/или живым `curl http://localhost:79/rci/show/interface/`.

**[P1] «Нет активного подключения» + ошибка на select-best.**
Подписка юзера `blanc` = Xray/VLESS. Xray НИКОГДА не становится интерфейсом роутера
(прозрачный TPROXY) — это by design (`engine/connections.go` → `integrationOf`). «Нет
активного» = активация не удержалась: `SelectBest` (`engine/subscriptions.go:394`) →
`fastest()` TCP-пингует эндпоинты; никто не ответил → «no reachable server»; иначе
`Activate()` → `bringUp`(xray Ensure+Apply) → `applyRouting`(TPROXY) → `verifyActive()`
пробит SOCKS→`gstatic /generate_204` сквозь туннель; провал → rollback → «activation
verification failed». В логе — циклические `activating … (previous active: "")`, туннель не
верифицируется. Вероятная причина on-device: DPI рвёт reality/TLS-хендшейк к vless-эндпоинтам
(TCP-пинг проходит, туннель не встаёт), ИЛИ проба к gstatic недоступна. Фронт
(`web/src/hooks/use-actions.tsx`, `SubscriptionsPage.tsx` → `selectBestMutation`) ГЛОТАЕТ
реальную ошибку бэка и показывает generic-тост — юзер не видит причину.
*Фикс:* (a) прокинуть реальный текст ошибки бэка в тост; (b) логировать stderr `xray` при
провале `bringUp`/`Apply`; (c) сделать probe-target конфигурируемым (не завязывать на
возможно-заблокированный gstatic); (d) on-device снять вывод `xray -test` и логи `xray run`.
Диагностика финализируется на устройстве.

**[инфо] Два инстанса демона.** В логах `bind: address already in use` на :8088 — были
запущены сервис + ручной `keen-manager daemon` одновременно. См. P2 single-instance guard.

### Что осталось невалидированным on-device (из 3-й сессии)
Юзер не дошёл — застрял на установке, затем на UI-багах: подхват `user2.list` демоном;
бейдж NFQUEUE/`healthy`; nfqws-guard; реальное применение маршрутов (object-group fqdn +
dns-proxy) и видимость `WireguardN`. Для AWG/маршрутов это заблокировано багом [P0] —
сначала чинить [P0], потом валидировать.

### Полезная инфа от юзера / устройство
- firmware release-строкой = `"5.01.C.0.0-1"` (KeeneticOS 5.1.0), `wireguard=true`,
  `native-awg2=false` (баг [P0]), arch arm64 (Entware `aarch64-k3.10`).
- xray стоит: `/opt/sbin/xray`. nfqws2-экосистема из opkg-фидов (`nfqws-keenetic`,
  `nfqws2-keenetic`, `hoaxisr`).
- Подписка `blanc`, 63 vless (reality + ws), локации Extra Whitelist / Extra Whitelist2
  (NL/RU/SE…). Парсер сабки работает (63/63).
- Морда по умолчанию `:47115`; юзер запускал вручную `KEEN_LISTEN=:8090`.
- Данные/секреты: `/opt/etc/keen-manager` (`state.json`, `servers.json` (vault, 0600),
  `backups/`, `xray/`). uninstall сохраняет их без `--purge`.

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

## 2. Приоритеты (обновлено 4-й сессией)

1. **[P0] Починить `parseKeeneticVersion` (канал C)** — см. §0. Разблокирует нативный AWG2 и
   DNS-маршруты на реальной прошивке юзера.
2. **[P1] Листинг интерфейсов с роутера** (`GET /api/interfaces` + `keenetic.ListInterfaces`
   + дропдаун) — см. §0.
3. **[P1] Диагностика/UX select-best + Xray-активации** (прокинуть реальную ошибку в UI,
   логировать xray, конфигурируемый probe-target) — см. §0. Финал — on-device.
4. **[P1] Баг персистентности auth** (хеш в vault + self-heal + CLI passwd/auth disable) —
   см. §0.
5. **On-device валидация** (после [P0]): user2.list, NFQUEUE-бейдж, nfqws-guard, применение
   маршрутов + видимость WireguardN.
6. **P2/P3 (см. ROADMAP):** дефолт-пресеты для 6 листов nfqws (нужен точный список доменов от
   юзера — не выдумывать); per-attempt timeout вокруг `Activate()`; backoff/jitter;
   single-instance guard; through-tunnel reachability; nfqws2 parity; hysteria2/tuic;
   дашборд live-трафик; CLI-паритет.

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
- Маршруты: `GET /api/routes`, `POST /api/routes {name,preset_id,domains[],subnets[],target_conn_id}`,
  `GET /api/routes/presets`, `PUT /api/routes/{id}/toggle`, `DELETE /api/routes/{id}`.
- nfqws: `GET /api/nfqws`, `POST /api/nfqws/action`, `GET/PUT /api/nfqws/config[/structured]`,
  `GET/PUT /api/nfqws/lists[/{name}]`, `POST /api/nfqws/lists/import {base,url,attr?,mode?}`,
  `POST /api/nfqws/check-domain`.
- Списки: `POST /api/lists/resolve {url,attr?}`. Failover: `GET/PUT /api/failover`.
  Настройки: `GET/PUT /api/settings`. Kill-switch: `POST /api/killswitch`. Логи:
  `GET /api/logs`. Auth: `GET /api/auth`, `POST /api/login`, `POST /api/logout`. Состояние:
  `GET /api/state` (несёт `active_connection_id`, `connections`, nfqws с
  `kernel_ready/missing_modules/healthy`). SSE: `GET /api/events`.
- **ОТСУТСТВУЕТ (добавить):** `GET /api/interfaces` (листинг интерфейсов роутера).

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
  Settings. `web/src/lib/` — `api.ts` (клиент; mocks только в DEV, в prod реальные ошибки),
  `types.ts`, `mocks.ts`. `web/src/hooks/use-actions.tsx` (activate и т.п. — глотает ошибку).

## 7. Порядок работы для следующего агента
1. Поставь Go (§3), `source goenv.sh`, `go build/vet/test ./...` — зелёно.
2. Чини [P0] `parseKeeneticVersion` (+ тесты) — это Go-only, бандл не трогает.
3. Для листинга интерфейсов и UX select-best правь `web/src/` → `RequestNetworkAccess` для
   npm → `npm ci` → правки → **`npm run build`** → закоммить `internal/webui/dist` в том же
   коммите (иначе CI-гард упадёт).
4. `go vet/test` + кросс mipsle. Коммить логическими слайсами от miroslavrov, пуш в main.
5. Всё device-mutating держи dry-run aware и помечай «validate on-device». Обнови этот
   HANDOFF и ROADMAP в конце сессии.
