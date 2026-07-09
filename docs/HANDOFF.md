# Handoff — keen-manager (для следующего агента)

Дата: 2026-07-09. Репозиторий: `github.com/miroslavrov/keen-manager`, ветка `main`.
Коммиты от лица **miroslavrov** (токен и git-credentials уже настроены в песочнице,
см. «Окружение»). Пользователь на **KeeneticOS 5.1.0**, тестирует бету, роутер
ломать нельзя.

Продукт: один Go-бинарь (демон + REST/JSON + SSE + CLI) со встроенной React/shadcn
мордой. Единый менеджер VPN (Xray/AmneziaWG) + обход DPI (nfqws2) для Keenetic.

---

## 1. Что сделано в эту сессию (уже в main)

**b894a8c — feat(routes): нативная DNS-маршрутизация Keenetic + каталог из 81 сервиса**
- `internal/presets/` — встроенный каталог 81 сервиса (YouTube, Instagram, Telegram,
  Netflix, OpenAI, Steam, …), 7 категорий, 3942 домена + 331 подсеть. Портирован из
  awg-manager (`tools/genpresets/transform.mjs` — воспроизводимо; композиты развёрнуты,
  singbox-only отброшены).
- `internal/keenetic/dnsroute.go` — `object-group fqdn` (set/delete), `dns-proxy route`
  (add/delete/list), чанкинг доменов по 300, namespace `km-`. Это и есть нативный
  раздел «Маршруты/DNS» KeenOS 5.x.
- `internal/keenetic/policy.go` — политики доступа (permit interface), статические
  маршруты для подсетей (через `parse` CLI `ip route`).
- `internal/keenetic/capabilities.go` — добавлен флаг `SupportsDNSRoute` (>=5.0).
- `internal/engine/routes.go` — модель `ServiceRoute`, CRUD, apply/unapply/reconcile
  (dry-run aware, rollback при частичном применении, чистка при удалении соединения,
  переприменение когда целевой AWG-тоннель поднялся).
- `internal/engine/connections.go` — блок **Integration** в детали соединения: честно
  объясняет, как соединение видно роутеру (нативный WireguardN vs прозрачный Xray/TPROXY).
  Это прямой ответ на боль «добавил сабку — в морде роутера ничего не появилось».
- REST: `GET/POST /api/routes`, `GET /api/routes/presets`, `PUT /api/routes/{id}/toggle`,
  `DELETE /api/routes/{id}`.

**7763d88 — feat(nfqws,failover): структурный конфиг nfqws2 + per-connection fallback**
- `GET/PUT /api/nfqws/config/structured` — типизированная форма поверх round-trip
  парсера (`internal/nfqws/schema.go`), PUT мержит частичные поля и переписывает только
  изменённое, затем reload.
- `internal/engine/failover.go` — поле `Connection.FallbackTo` подключено к движку: когда
  глобальная цепочка исчерпана/пуста, применяется персональный фолбек соединения
  (VPN → другой VPN → AWG → direct).

Состояние сборки на момент хендоффа: `go build/vet/test` — зелёное; кросс-компиляция
mipsle — ок; веб-бандл байт-в-байт совпадает с закоммиченным (CI-guard пройдёт).

---

## 2. Что осталось — приоритеты (P1 → P3)

### P1 — ФРОНТ под новый бэкенд (самое важное, пользователь ждёт «сочно/красиво»)
Бэкенд готов, UI — НЕТ. Нужно (всё в `web/src/`, потом **обязательно** пересобрать бандл):
1. **Страница «Маршруты» (`/routes`)** — грид каталога пресетов по категориям (иконки
   lucide/бренд, без эмодзи), мультивыбор + пикер целевого соединения + переключатель,
   добавление кастомных доменов. Данные: `GET /api/routes/presets`, `GET /api/routes`,
   `POST /api/routes {name,preset_id,domains[],subnets[],target_conn_id}`,
   `PUT /api/routes/{id}/toggle {enabled}`, `DELETE /api/routes/{id}`. Referencial UX —
   awg-manager `ServiceCatalogModal.svelte`. Добавить пункт в `web/src/lib/nav.ts` (иконку,
   напр. `Route`/`Waypoints`) + i18n RU/EN в `web/src/i18n/pages/`.
2. **Панель Integration** на странице Connections — показывать `integration.summary`,
   бейдж «виден в роутере: WireguardN» / «прозрачный прокси», и если Xray — подсказку
   «нативного интерфейса не будет, это норма; используйте Маршруты/политики». Поле уже
   приходит в `GET /api/connections/{id}` → `integration`.
3. **Структурная форма nfqws2** на `BypassPage.tsx` — типизированные поля из
   `GET /api/nfqws/config/structured`, режим AUTO/LIST/ALL как Select, сырой редактор
   оставить вкладкой «Advanced». Паритет с nfqws-keenetic-web (форма + raw).
4. **Per-connection fallback picker** на ConnectionsPage — селект `fallback_to`
   (другое соединение / «direct»), пишется через `PUT /api/connections/{id} {fallback_to}`.

### P1 — на устройстве (проверить, я не мог — off-device)
- Реальное применение маршрута: `object-group fqdn` + `dns-proxy route` на живой 5.1.0.
  Шейпы взяты из awg-manager (верифицированы там), но подтвердить на железе.
- Нативный AWG2 apply (import .conf) уже был; убедиться что WireguardN появляется в морде.

### P2
- Шесть листов nfqws2 с дефолт-пресетами (`user/exclude/auto/ipset/ipset_exclude`) —
  сейчас редактор есть, дефолтных доменов нет. Данные листов см. отчёт nfqws ниже.
- Сигнал «стратегия nfqws сдохла → фолбек на AWG» (роадмап P1, пока не сделан) —
  детект мёртвого демона/пробы should-bypass доменов → драйвит failover.
- Kernel-module readiness для nfqws (`nfnetlink_queue`/`xt_NFQUEUE`).
- ndm netfilter-хук для nfqws (у нас есть свой `50-keen-manager`; добавить парити для
  nfqws-правил, чтобы переживали firewall reload — см. отчёт).

### P3
- Дашборд: live-трафик, быстрый переключатель, бейдж native-AWG2.
- CLI-паритет для structured nfqws + failover.
- hysteria2/tuic в подписках.

---

## 3. Окружение песочницы (важно для сборки/верификации)

- **Go НЕ предустановлен.** Ставить: `curl -fsSL https://go.dev/dl/go1.26.5.linux-amd64.tar.gz`
  → распаковать в `/agent/workspace/.goroot`, `export PATH=/agent/workspace/.goroot/bin:$PATH
  GOPATH=/agent/workspace/.gopath GOCACHE=/agent/workspace/.gocache`. go.mod требует go 1.26.
- **`make` нет** — цели выполнять вручную: сборка бандла = `cd web && npm run build`
  (vite пишет в `internal/webui/dist`); `go vet ./...`; `go test ./...`.
- **npm-реестр (`registry.npmjs.org`) за firewall** — доступ пользователь уже подтвердил
  через RequestNetworkAccess; `npm install` в `web/` работает (188+ пакетов).
- **git-credentials уже настроены** (`~/.git-credentials`, helper=store), push от miroslavrov
  работает. НЕ коммить токен в репозиторий.
- Go-модуль-прокси и GitHub доступны из песочницы; таргет-девайс (mips/arm) тестировать
  нельзя — только off-device dry-run + честные пометки «validate on-device».

### CI-гард (не сломать!)
`.github/workflows/ci.yml` шаг «Verify committed bundle is up to date» делает
`npm run build` и падает, если `internal/webui/dist` отличается от закоммиченного.
**Любое изменение в `web/src/` требует пересобрать бандл и закоммитить `internal/webui/dist`
в том же коммите.** Иначе CI красный. Node в CI пиннится на 24.

---

## 4. Ключевые RCI-шейпы Keenetic (верифицированы по awg-manager; для справки)

- База: `http://localhost:79/rci`, без auth на loopback, POST=действия, GET=`/show/...`.
- Сохранить конфиг (переживает ребут): `{"system":{"configuration":{"save":{}}}}`
  (НЕ `{"save":true}` — молча no-op на OS5). Уже реализовано в `client.go: Save()`.
- Нативный AWG2: наш путь — import .conf (`internal/keenetic/import.go`), NDMS парсит все
  jc/jmin/jmax/s1-s4/h1-h4/i1-i5 и создаёт видимый `WireguardN`. Альтернатива awg-manager —
  batch (interface create/description/security-level public/ip address/mtu/adjust-mss/
  ip global/wireguard private-key/peer) + asc через `{"interface":{"WireguardN":
  {"wireguard":{"asc":{...}}}}}`.
- DNS-маршруты: `{"object-group":{"fqdn":{"km-youtube":{"include":[{"address":"youtube.com"}]}}}}`
  + `{"dns-proxy":{"route":[{"group":"km-youtube","interface":"Wireguard0","auto":true}]}}`.
  Лимит ~300 доменов/группа. Всё в `internal/keenetic/dnsroute.go`.
- Политика: `{"ip":{"policy":{"Policy0":{"permit":{"global":true,"interface":"Wireguard0","order":0}}}}}`.

## 5. nfqws2 — сжатый паритет (из реверса nfqws2-keenetic + nfqws-keenetic-web)

- Конфиг `/opt/etc/nfqws2/nfqws2.conf` — shell `KEY="value"`. Ключи уже смоделированы в
  `internal/nfqws/schema.go`. Режимы: `MODE_LIST` (только user.list), `MODE_ALL` (все кроме
  exclude.list), `MODE_AUTO` (list + autohostlist «3 фейла за 60с → auto.list» + exclude).
- Листы `/opt/etc/nfqws2/lists/`: user/exclude/auto/ipset/ipset_exclude (`internal/nfqws/lists.go`).
- Управление: init `S51nfqws2 {start|stop|restart|reload|status}`. **reload = SIGHUP**
  (горячий перечит листов, без разрыва), restart = полный. У нас в `control.go`.
- ndm-хук выживания: `/opt/etc/ndm/netfilter.d/100-nfqws2.sh` реинжектит цепочки после
  перестройки firewall Keenetic'ом (keyed по `$table`/`$type`, no-op если демон мёртв).
- PBR по connmark: имя политики Keenetic → connmark из `ndmc -c show ip policy` → iptables.
- Веб-морда nfqws-keenetic-web: форма-над-conf + сырой редактор, редактор листов,
  «Check domains» (проверка доменов через живой путь роутера), кнопки lifecycle.

## 6. awg-manager — что ещё полезно скопировать (из реверса)
- 86 пресетов (у нас 81 после отбрасывания singbox-only) — DNS через `object-group fqdn`+
  `dns-proxy route` (сделано). Для apple/microsoft/rkn/ads (только .srs) — при желании
  декомпилировать sing-box .srs в плоские домены или тянуть `subscription_url` (у нас
  поле `subscription_url` в пресете уже есть, напр. `all-blocked` = itdoginfo).
- Dual-backend (kernel `ip link add type amneziawg` невидимый vs NativeWG видимый) —
  у нас native import + userspace awg-quick fallback, эквивалентно.
- Порт по умолчанию у нас **47115** (`internal/config/defaults.go`) — уже сменён, не трогать
  без причины.

## 7. Карта кода
- `internal/engine/` — ядро (apply.go активация+rollback, failover.go, routes.go, loops.go,
  connections.go, subscriptions.go, nfqws.go, awgnative.go, views.go DTO).
- `internal/keenetic/` — RCI (client, import, iface, dnsroute, policy, capabilities, status).
- `internal/subscription/` — парсинг подписок (base64/clash/sip008 + vless/vmess/trojan/ss;
  blancvpn 63/63 ок).
- `internal/nfqws/`, `internal/xray/`, `internal/route/` (TPROXY+killswitch+hook),
  `internal/presets/` (каталог).
- `web/src/pages/` — Dashboard, Connections, Subscriptions, Bypass(nfqws), Failover, Logs,
  Settings. Роутер и nav в `web/src/lib/nav.ts` + `App.tsx`.

## 8. Порядок работы для следующего агента
1. Поставь Go (см. §3), `go build/vet/test` — убедись что зелёно.
2. `cd web && npm install` (реестр открыт), затем правь `web/src/`.
3. Реализуй P1-фронт (§2): страница Routes, Integration-панель, форма nfqws, fallback-пикер.
4. **`cd web && npm run build`** → закоммить `internal/webui/dist` вместе с исходниками
   (иначе CI красный).
5. `go build/vet/test` + кросс-компиляция mipsle. Коммить логическими слайсами от miroslavrov,
   пуш в main. Обнови README/ROADMAP по факту.
6. Всё device-mutating держи dry-run aware и помечай «validate on-device».
