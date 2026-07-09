# Handoff — keen-manager (для следующего агента)

Обновлено: 2026-07-09 (вторая сессия). Репозиторий: `github.com/miroslavrov/keen-manager`,
ветка `main`. Коммиты от лица **miroslavrov** (git-credentials уже в песочнице).
Пользователь на **KeeneticOS 5.1.0**, тестирует бету, роутер ломать нельзя.

Продукт: один Go-бинарь (демон + REST/JSON + SSE + CLI) со встроенной React/shadcn
мордой. Единый менеджер VPN (Xray/AmneziaWG) + обход DPI (nfqws2) для Keenetic.

---

## 1. Что сделано в ЭТУ сессию (уже в main)

Бэкенд под P1-фичи был готов прошлой сессией; закрыт фронт + баги.

**fix(auth): жёсткий gate логина**
- `internal/server/auth.go` + `respond.go`: `no-store` на `/api/auth`, `/api/login`,
  `/api/logout` — браузер/прокси не кэшируют состояние авторизации.
- `web/src/components/RequireAuth.tsx`: раньше был «мягкий» gate — защищённое дерево
  рендерилось, а редирект лишь планировался в эффекте, плюс `staleTime: 30s` на
  `['auth']`. Это и есть корень «иногда пускает без пароля». Теперь: пока
  `enabled && !authenticated` — рендерится нейтральный экран, дети НЕ монтируются;
  запрос авторизации всегда свежий (`staleTime: 0`, refetch on mount/focus).

**feat(web routes): страница «Маршруты» (`/routes`)**
- `web/src/pages/RoutesPage.tsx` + `i18n/pages/routes.ts` + пункт в `lib/nav.ts`
  (иконка `Waypoints`) + маршрут в `App.tsx` + `/routes` в smoke-тесте.
- Каталог 81 пресета по 7 категориям (поиск, мультивыбор), пикер целевого
  подключения (только нативные AWG-интерфейсы), список активных маршрутов
  (enable/apply-бейдж/delete), билдер своих доменов/подсетей и импорт списков.
- Иконки: категорийные lucide + монограмма на тинте (без эмодзи, без brand-deps).

**feat(web): панель Integration + структурная форма nfqws2**
- `ConnectionsPage.tsx`: в detail-sheet добавлена `IntegrationPanel` — читает
  `integration` из `GET /api/connections/{id}`, показывает «виден в роутере / не
  интерфейс», нативное имя `WireguardN`, режим, признак routable. Прямой ответ на
  боль «добавил сабку — в морде роутера пусто».
- `BypassPage.tsx`: вкладка Config → саб-табы **Форма / Advanced(raw)**.
  Структурная форма поверх `GET/PUT /api/nfqws/config/structured` (режим
  AUTO/LIST/ALL, TCP/UDP порты, блоки стратегий, policy, NFQUEUE, log level, IPv6).
  Raw-редактор сохранён как Advanced. Round-trip lossless (бэкенд переписывает
  только изменённые ключи).

**feat(lists): резолвер удалённых доменных списков (v2fly / plain / hosts)**
- Новый пакет `internal/listsrc/` (только stdlib, покрыт тестами): `Resolve(ctx,
  url, Options{AttrFilter,...}) (Result, error)`. Нормализует GitHub blob→raw,
  рекурсивно тянет `include:` (cycle-guard, MaxFiles/MaxDomains/MaxDepth),
  фильтрует по `@attr`, разбирает `domain:/full:/keyword:/regexp:` и inline-`#`,
  валидирует хостнеймы, дедуплицирует и сортирует.
- `internal/engine/lists.go` → `Engine.ResolveList(url, attr)`; API
  `POST /api/lists/resolve {url, attr?}` → `{domains[],skipped[],sources[],truncated,skipped_n}`.
- UI: диалог «Import from URL» в Bypass → Hostlists (append/replace, мержит в
  открытый список для ревью перед Save) и импортер в Routes → Custom & import.
- Живой тест подтвердил: `data/cloudflare` (с `include:cloudflare-cn/-ipfs`) →
  3 источника, 74 домена; blob-URL нормализуется идентично raw.

Состояние сборки на момент хендоффа: `go build/vet/test` — зелёное; кросс-компиляция
mipsle+arm64 — ок; веб-бандл пересобран и закоммичен (CI-guard пройдёт);
все 17 web smoke-тестов проходят (7→8 роутов ×2 языка + 404).

---

## 2. Что осталось — приоритеты

### P1 — на устройстве (я off-device, проверить нельзя)
- Реальное применение маршрута: `object-group fqdn` + `dns-proxy route` на живой
  5.1.0. **ВАЖНО:** пользователь говорит, что в один список Keenetic влезает ~100
  строк, а `keenetic.ChunkDomains` сейчас режет по 300 — проверить на железе и при
  необходимости снизить до 100.
- Нативный AWG2 apply: убедиться, что `WireguardN` появляется в морде и роутит.

### P1 — код (не сделано)
- **Авто-сплит импортируемых списков** на несколько `*.list` по ≤100 строк
  (`listsrc` отдаёт плоский список; диалог сейчас мержит в один файл).
- **nfqws health → failover signal**: детект мёртвой стратегии (демон/проба
  should-bypass доменов) → драйвит fallback на AWG. Модель `Connection.FallbackTo`
  и движок готовы; нужен сам сигнал.
- **Kernel-module readiness** для nfqws (`nfnetlink_queue`/`xt_NFQUEUE`) до статуса
  «healthy» (`platform.KernelModuleDirs()`).

### P2/P3
- Дефолт-пресеты для 6 листов nfqws (редактор есть, дефолтных доменов нет).
- Дашборд live-трафик, CLI-паритет structured nfqws + failover, hysteria2/tuic.

---

## 3. Окружение песочницы (важно для сборки/верификации)

- **Go НЕ предустановлен.** Ставить: `curl -fsSL https://go.dev/dl/go1.26.5.linux-amd64.tar.gz`
  → в `/agent/workspace/.goroot`; затем `source /agent/workspace/goenv.sh` (готовый
  скрипт: PATH/GOROOT/GOPATH/GOCACHE + `GOFLAGS=-mod=mod`). go.mod требует go 1.26.
- **`make` нет** — цели вручную: бандл = `cd web && npm run build` (vite пишет в
  `internal/webui/dist`); `go vet ./...`; `go test ./...`. Кросс:
  `GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0 go build ./cmd/keen-manager`.
- **npm-реестр за firewall!** `registry.npmjs.org` блокируется по умолчанию (403).
  В начале сессии вызови `RequestNetworkAccess({domain:"registry.npmjs.org"})` и
  дождись одобрения — иначе `npm install` не пройдёт и бандл не пересобрать.
  `web/package-lock.json` НЕ должен меняться (детерминизм бандла) — ставь как есть.
- **git-credentials уже настроены** (`~/.git-credentials`, helper=store). НЕ коммить
  токен в репозиторий.

### CI-гард (не сломать!)
`.github/workflows/ci.yml` шаг «Verify committed bundle is up to date» делает
`make web` и падает, если `internal/webui/dist` отличается от закоммиченного.
**Любое изменение в `web/src/` требует пересобрать бандл и закоммитить
`internal/webui/dist`.** Node в CI пиннится на 24; локально Node 24 в песочнице.

---

## 4. Ключевые REST-эндпоинты (актуальный контракт)

- Маршруты: `GET /api/routes`, `GET /api/routes/presets`,
  `POST /api/routes {name,preset_id,domains[],subnets[],target_conn_id}`,
  `PUT /api/routes/{id}/toggle {enabled}`, `DELETE /api/routes/{id}`.
- Интеграция: поле `integration` в `GET /api/connections/{id}`
  (`{mode,visible_in_router,interface,summary,routable_target}`).
- nfqws структурно: `GET/PUT /api/nfqws/config/structured` (типы = `internal/nfqws.Conf`).
- Per-conn fallback: `PUT /api/connections/{id} {fallback_to}` ("direct" / conn id / "").
- Списки: `POST /api/lists/resolve {url, attr?}` (read-only fetch, safe в dry-run).

## 5. RCI-шейпы Keenetic (для справки, верифицированы по awg-manager)
- База `http://localhost:79/rci`; save конфига: `{"system":{"configuration":{"save":{}}}}`.
- DNS-маршруты: `{"object-group":{"fqdn":{"km-<slug>":{"include":[{"address":"x"}]}}}}`
  + `{"dns-proxy":{"route":[{"group":"km-<slug>","interface":"WireguardN","auto":true}]}}`.
- Политика: `{"ip":{"policy":{"Policy0":{"permit":{"interface":"WireguardN"}}}}}`.

## 6. Карта кода
- `internal/engine/` — ядро (apply, failover, routes, loops, connections, subscriptions,
  nfqws, awgnative, lists (новый), views DTO).
- `internal/keenetic/` — RCI (client, import, iface, dnsroute, policy, capabilities).
- `internal/listsrc/` — резолвер удалённых доменных списков (новый, stdlib+tests).
- `internal/subscription/`, `internal/nfqws/` (schema round-trip), `internal/xray/`,
  `internal/route/`, `internal/presets/` (каталог 81 сервиса из data/presets.json).
- `web/src/pages/` — Dashboard, Connections, Subscriptions, **Routes (новый)**,
  Bypass(nfqws), Failover, Logs, Settings. Роутер/nav — `web/src/lib/nav.ts` + `App.tsx`.
- `web/src/lib/` — `api.ts` (клиент), `types.ts`, `mocks.ts` (DEV/тест fallback), `utils.ts`.

## 7. Порядок работы для следующего агента
1. Поставь Go (§3), `source goenv.sh`, `go build/vet/test` — зелёно.
2. `RequestNetworkAccess` для npm → `cd web && npm install`.
3. Правь `web/src/`, затем **`npm run build`** и закоммить `internal/webui/dist`.
4. `go vet/test` + кросс mipsle. Коммить логическими слайсами от miroslavrov, пуш в main.
5. Всё device-mutating держи dry-run aware и помечай «validate on-device».
