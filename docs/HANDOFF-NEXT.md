# Handoff — следующему агенту (после сессии 19 / rc.6)

Обновлено: **2026-07-13 (сессия 19)**. Репозиторий: `github.com/miroslavrov/keen-manager`,
ветка `main`, коммиты от лица **miroslavrov**. Устройство пользователя: **KeeneticOS
`5.01.C.0.0-1`**, arch **arm64**, живой роутер с реальным трафиком — **ломать нельзя**.
Полная история — в `docs/HANDOFF.md` (сессии 5–18). Этот файл — актуальный срез + две
открытые задачи. Архитектура: `docs/ARCHITECTURE.md`. План по Xray-проксику:
`docs/XRAY-PROXY-PLAN.md`.

> ⚠️ Задачи ниже НЕ РЕШЕНЫ. Предыдущий агент по прямой просьбе пользователя их не трогал —
> только зафиксировал состояние. Не считать сделанным ничего, кроме пунктов в «Что уже сделано».

---

## Что уже сделано (сессия 19, в `main`, релиз `v0.1.0-rc.6`)

1. **`fix(route,awg,engine)` — «exec format error» на `/opt/sbin/ip`** (коммит `64b3b27`).
   Причина: `route.New`, `awg.Controller.ipBin`, `engine.ipBin` брали `/opt/sbin/ip`,
   если файл просто существует, без проверки исполнимости. Кривой (не той арх / битый)
   ip-full → `ip rule add` падал с `fork/exec … exec format error` → каждая активация
   Xray-через-TPROXY откатывалась.
   Фикс: `internal/platform/runnable.go` — `Runnable()` (проверка ELF-заголовка БЕЗ
   запуска, переиспользует `ELFArch`) + `ResolveIP()` (предпочитает `/opt/sbin/ip`, но
   пропускает нерабочий → `/sbin/ip` → PATH) + `IPBinBroken()` (разовый диагностический
   лог с советом `opkg install --force-reinstall ip-full`). Тесты в
   `internal/platform/runnable_test.go`. README EN/RU дополнен.

2. **`feat(web)` — загрузка AWG `.conf` файлом** (коммит `2659e3e`). В диалоге
   «Добавить подключение» вкладка AmneziaWG: кнопка «Загрузить из файла» + drag-and-drop.
   Файл читается в браузере и идёт тем же путём `awg_conf`, что и вставленный текст —
   бэкенд не менялся. Файлы: `web/src/pages/ConnectionsPage.tsx`,
   `web/src/i18n/pages/connections.ts`. Бандл `internal/webui/dist` пересобран и
   закоммичен (CI-гард «bundle up to date» зелёный).

3. **`chore(tools)` — `tools/publish_release.py`** (коммит `30b68ba`) — ручной выпуск
   релиза через REST API, если GitHub Actions недоступен.

4. **Релиз `v0.1.0-rc.6`** (тег на `2659e3e`) с бинарями под 4 арх + `sha256sums.txt`.
   Прим.: на этом ПУБЛИЧНОМ репо `release.yml` **сам отработал** и пересобрал ассеты
   (значит Actions по факту доступен — «лимиты кончились» больше не подтверждается).

**Состояние `main`:** HEAD `30b68ba`; тег/релиз `v0.1.0-rc.6`. `go vet`/`go test ./...`
зелёные; CI на обоих push в `main` — success.

**Подтверждено на устройстве пользователем:** после `opkg install --force-reinstall ip-full`
(его `/opt/sbin/ip` был не той арх) + установки rc.6 — `exec format error` ИСЧЕЗ,
TPROXY-цепочка ставится целиком (`ip rule add`, `ip route replace`, весь `KEENMGR_TPROXY`).
`keen-manager version` → `v0.1.0-rc.6`. **P0 «exec format» закрыт.**

---

## ЗАДАЧА A (P0, теперь ИЗМЕРИМА) — активация «заводится, потом сама выключается»

### Симптом (лог устройства, 2026-07-13, rc.6)
```
15:23:35 activating 🇳🇱 Амстердам … 
15:23:37 proxy-conn: Proxy client unavailable (… create proxy interface Proxy2 …) — falling back to TPROXY for Xray
15:23:38 exec: /opt/sbin/ip rule add fwmark 0x2333 lookup 993        ← теперь РАБОТАЕТ
15:23:38 exec: … весь KEENMGR_TPROXY ставится, PREROUTING -j KEENMGR_TPROXY
   (в access-логе xray: десятки  «… accepted tcp:… [tproxy-in -> srv-conn-djxi61hbibcm]»
    т.е. реальный трафик клиентов 192.168.1.57/.107/.81 РЕАЛЬНО захватывается и уходит в туннель;
    в т.ч. 15:25:06  «from tcp:127.0.0.1:56520 accepted tcp:www.gstatic.com:443 [socks-in -> srv-conn]»
    — это САМА проба через SOCKS, её xray принял и отправил на сервер)
15:25:14 post-activate probe failed … target=https://www.gstatic.com/generate_204:
         Get "…/generate_204": context deadline exceeded; xray log: … Xray 25.1.30 started — rolling back to ""
15:25:14 … снос KEENMGR_TPROXY / KILL / table 993, S99xray stop
```
Итог: Xray стартовал, конфиг валиден (это уже чинил rc.5, см. session 18), TPROXY ловит
трафик, но **пост-активационная проба `verifyActive` за 90 с ни разу не получила ответ →
deadman откатил всё назад**. Пользователь описал как «оно завелось, потом выключилось само».

### Где это в коде
- `internal/engine/apply.go`:
  - `activate()` шаг 4 «verify + rollback deadman» (~строки 85–107): если `verifyActive`
    не прошёл — `rollback(prev, c)`.
  - `verifyActive(ctx, c)` (~305–360): цикл до `rollbackTimeout()`; для `ConnXray`
    зовёт `health.SOCKSHTTP(127.0.0.1:10808, probeTarget, 6s)`, ретрай каждые ~8 с
    (6 с проба + 2 с backoff).
  - `probeTarget()` (~537): `Failover.ProbeTarget` или дефолт
    `https://www.gstatic.com/generate_204`.
  - `rollbackTimeout()` / `normalizeRollbackTimeout` (~557–575): дефолт **90 с**, мин 10 с
    (`Settings.RollbackTimeoutS`).
- `internal/health/health.go`: `SOCKSHTTP` (SOCKS5 dial + HTTP GET, ждёт 2xx/3xx;
  `generate_204` → 204), `DefaultTimeout = 6s`, `socks5Dial`.
- Xray SOCKS-инбаунд: `127.0.0.1:10808` (`engine.go` const `xraySocksHost/Port`).
- TPROXY-правила и защита от петли: `internal/route/route.go` (table `993`, fwmark
  `0x2333`, `SelfMark = 255` — егресс самого Xray помечается и НЕ перезахватывается;
  проверить, что xray реально ставит этот SO_MARK).

### ГЛАВНЫЙ вопрос (сначала дискриминировать, потом чинить)
**Туннель реально не несёт payload — ИЛИ проба даёт ложноотрицательный результат?**
Пользователь сказал «оно завелось» — во время окна реальные клиенты гнали трафик в туннель.
access-лог показывает только `accepted` (вход), НЕ подтверждает успех round-trip. Нужно развести:

1. **Если реальный сёрфинг РАБОТАЛ эти ~90 с, а проба падала → ложноотрицательная verify.**
   Тогда чинить ПРОБУ, а не туннель:
   - `generate_204` мог быть недоступен с этого exit (гео/DPI на выходе). Сменить
     `Failover.ProbeTarget` (UI: Failover → Probe target) на, напр.,
     `https://www.google.com/generate_204`, `http://cp.cloudflare.com/generate_204`,
     `https://1.1.1.1`.
   - Проба ходит через SOCKS c удалённым DNS; если DNS-резолв через туннель тормозит/рвётся,
     проба падает, а уже установленные соединения живут. Проверить проксирование DNS.
   - Рассмотреть более устойчивый сигнал здоровья: счётчики байт на `KEENMGR_TPROXY`
     (`iptables -t mangle -L KEENMGR_TPROXY -nv`), либо статистику Xray API, либо
     несколько проб-таргетов с ИЛИ-логикой, либо больше ретраев на первом окне.
2. **Если реальный трафик ТОЖЕ не ходил → настоящий P0 «reality/vision несёт хэндшейк,
   но не payload»** (слой из session 15–17). Теперь он РАЗБЛОКИРОВАН (конфиг валиден rc.5,
   ip починен rc.6) и наконец измерим. Возможные причины: серверный конфиг/flow (`xtls-rprx-vision`),
   мёртвый узел, MTU, DPI на выходе. Пробовать другой сервер / несколько (см. Задачу B / select-best).

### Диагностика на устройстве (только чтение, безопасно)
Во время активации (или подняв Xray руками) выполнить на роутере:
```sh
# 1. Работает ли SOCKS-путь вообще и что именно отвечает generate_204:
curl -x socks5h://127.0.0.1:10808 -sv -o /dev/null -w 'code=%{http_code} time=%{time_total}\n' https://www.gstatic.com/generate_204
# 2. Тот же путь на другие таргеты (развести «gstatic заблокирован» vs «туннель мёртв»):
curl -x socks5h://127.0.0.1:10808 -s  -o /dev/null -w '%{http_code}\n' http://cp.cloudflare.com/generate_204
curl -x socks5h://127.0.0.1:10808 -s  -o /dev/null -w '%{http_code}\n' https://www.google.com/generate_204
curl -x socks5h://127.0.0.1:10808 -s  https://api.ipify.org ; echo   # виден ли выходной IP сервера
# 3. Счётчики TPROXY (растут ли reply-байты) и таблица маршрутов:
/opt/sbin/iptables -t mangle -L KEENMGR_TPROXY -nv
/opt/sbin/ip rule ; /opt/sbin/ip route show table 993
# 4. Лог xray в debug (движок дистиллирует причину — см. xrayFailureReason):
#    поднять loglevel до debug/warning и смотреть /opt/etc/keen-manager/xray + S99xray лог
# 5. Готовый диагност:
sh /opt/tmp/diag-tunnel.sh   # scripts/diag-tunnel.sh в репо
```
Также в репо: `scripts/diag-tunnel.sh` и `scripts/selftest.sh`.

### Возможные направления фикса (на выбор, по итогам диагностики)
- Сделать `verifyActive` устойчивее к ложноотрицанию: несколько таргетов (ИЛИ), больший
  первый бюджет, либо принять «идёт reply-трафик по счётчикам TPROXY» как успех.
- Дать быстрый UI-путь сменить `ProbeTarget` (Failover-страница уже умеет — упомянуть юзеру).
- Если это реальный P0-payload — отдельная ветка про reality/flow; НЕ смешивать с пробой.
- НЕ занижать `RollbackTimeoutS` — 90 с достаточно; проблема не в таймауте.

---

## ЗАДАЧА B (UX) — «использовать лучший» должен быть ТУМБЛЕРОМ (режим), а не разовой кнопкой

### Что просил пользователь (дословно)
«когда я нажимал использовать лучший — там должна не кнопка быть а тумблер. то бишь эта
настройка должна поверх подключения быть наверное».

То есть: авто-выбор лучшего сервера — это **постоянный режим** (вкл/выкл), и его место —
**на уровне подключения** (страница Подключения / карточка активного подключения), а не
разовое действие, спрятанное в подписках.

### Ключевой факт: БЭКЕНД УЖЕ ЭТО УМЕЕТ
- `internal/model` — `Subscription.AutoSelectBest bool` (это и есть постоянный тумблер).
- `internal/engine/loops.go` — `autoSelectLoop()` + `autoSelectTick()`: если у активного
  подключения подписка с `AutoSelectBest && Enabled`, периодически мигрирует на
  «ощутимо более быстрый» сервер. Интервал — `Settings.AutoSelectIntervalMin`
  (дефолт 30, `config/defaults.go`).
- `internal/engine/subscriptions.go` — `SelectBest()` (разовый перебор best-first с
  verify) и запись `AutoSelectBest` в `UpdateSubscription` (~строка 369). Новая подписка
  создаётся с `AutoSelectBest: true` (~строка 96).
- API: `PUT /api/subscriptions/{id}` `{auto_select_best}` (постоянный тумблер);
  `POST /api/subscriptions/{id}/select-best` (разовое действие, кнопка).

### Текущий UI
- `web/src/pages/SubscriptionsPage.tsx` — есть И тумблер «Auto-best» И кнопка «Select best»
  (строки в `web/src/i18n/pages/subscriptions.ts`: `autoBest`, `selectBest`, `autoSelectAria`…).
- `web/src/pages/DashboardPage.tsx` — кнопка «Activate best» (`i18n/pages/dashboard.ts`).
- `web/src/lib/api.ts` — `selectBest(id)` (POST) и `updateSubscription(id,{auto_select_best})`.

### Что, вероятно, нужно сделать (обсудить с пользователем детали)
Задача В ОСНОВНОМ фронтовая (бэкенд-режим есть):
- Вынести переключатель **Auto-select best** на уровень подключения / активного туннеля —
  напр. в карточку подключения (`ConnectionsPage.tsx`) и/или на Dashboard рядом с активным,
  чтобы «поверх подключения» читалось как «держи меня на лучшем автоматически».
- Разовую «Select best» оставить второстепенной (или свернуть в меню), а первичным сделать
  тумблер режима. Убедиться, что состояние тумблера = `subscription.auto_select_best`
  активного подключения и мгновенно отражается (invalidate query).
- Продумать связку с failover-цепочкой (`internal/engine/failover.go`) — чтобы авто-бест и
  фейловер не воевали; и, возможно, показать интервал `AutoSelectIntervalMin`.
- Не забыть: любая правка `web/src/**` требует пересборки бандла (`make web` →
  `internal/webui/dist`), иначе CI-гард «bundle up to date» упадёт и на устройство уедет
  старый UI. Обновлять RU+EN i18n синхронно (файлы держатся key-for-key).

---

## Сборка / релиз / деплой — шпаргалка

- Тулчейн: **Go 1.26+** (go.mod = `go 1.26`), **Node 24** (CI пинит 24). CGO выключен.
- Локально:
  ```sh
  make web        # React → internal/webui/dist (go:embed); ОБЯЗАТЕЛЬНО после правок web/src
  make dist       # web + кросс всех 4 арх + gzip + sha256 → build/keen-manager-<arch>.gz
  # только arm64 без make:
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w \
    -X github.com/miroslavrov/keen-manager/internal/version.Version=$(git describe --tags --always --dirty)" \
    -o build/keen-manager-arm64 ./cmd/keen-manager
  ```
- Релиз, если Actions недоступен: `GH_TOKEN=… python3 tools/publish_release.py <tag> <commit_or_-> <файлы…>`
  (создаёт/переиспользует релиз, перезаливает ассеты по имени). Если `release.yml`
  отрабатывает сам на push тега `v*` — можно просто пушить тег.
- Деплой на роутер (Entware): установщик умеет ЛОКАЛЬНЫЙ файл —
  ```sh
  KEEN_URL="file:///opt/tmp/keen-manager-arm64.gz" KEEN_ARCH=arm64 sh /opt/tmp/install.sh
  # либо штатно из релиза:
  curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/install.sh | sh
  ```
  Бинарь → `/opt/bin/keen-manager`; init `/opt/etc/init.d/S99keen-manager`; веб `:47115`.
  Всегда сверять `sha256sum -c sha256sums.txt` и `keen-manager version`.

## Правила
- Коммиты — от **miroslavrov**. Токен — только в `~/.git-credentials` (helper=store),
  origin чистый (`https://github.com/miroslavrov/keen-manager.git`), секреты в репо НЕ коммитить
  (`.gitignore` уже покрывает `*.gz`, `/build/`, `.gh_token`, `*.key.local`).
- Живой роутер пользователя — не ломать; беты тестируются на нём.
- После правок `web/src/**` — `make web` и коммит пересобранного `internal/webui/dist`.
- `go vet ./...` + `go test ./...` + кросс 4 арх должны быть зелёными до релиза.
