# Handoff — keen-manager (для следующего агента)

Обновлено: 2026-07-09 (**11-я сессия**). Репозиторий: `github.com/miroslavrov/keen-manager`,
ветка `main`. Коммиты от лица **miroslavrov** (git-credentials уже в песочнице; токен
пользователь передаёт отдельно, в репозиторий НЕ коммитить). Пользователь на **KeeneticOS**
(прошивка отдаёт release-строку `"5.01.C.0.0-1"`, это 5.1.0), arch **arm64**, тестирует
бету на живом роутере — ломать нельзя.

Продукт: один Go-бинарь (демон + REST/JSON + SSE + CLI) со встроенной React/shadcn
мордой. Единый менеджер VPN (Xray/AmneziaWG) + обход DPI (nfqws2) для Keenetic.

---

## 0. Сессии 5–11 — что сделано и что осталось

### 11-я сессия (ЗАКРЫТА) — hardening под релиз (P2) + фикс установщика + CLI + rc.1
Всё в `main`, сборка зелёная (`go build/vet/test` + кросс mipsle/arm64; веб НЕ трогали,
CI-гард бандла проходит). Выпущен **`v0.1.0-rc.1`** — мост «бета → stable» (фичи считаем
готовыми, ждём on-device P0). Коммиты:
- **`fix(install)` 143209e** — БАГ ЮЗЕРА «качается beta.9 вместо beta.10». `install.sh`
  брал ПЕРВЫЙ `tag_name` из GitHub `/releases`, считая список «сначала новейшее» — а он
  НЕ так отсортирован (beta.10 приходил ПОЗАДИ beta.9), так что ставилась beta.9. Теперь
  тянет весь список и сам выбирает максимум по SemVer на чистом awk (busybox-safe). README
  (EN+RU) + ARCHITECTURE поправлены (ярлык `latest/download` 404-ит на репо без
  stable-релизов). `install.sh` отдаётся с `main` → фикс действует БЕЗ нового релиза.
- **`fix(engine)` 7354bc8** — `rollback_timeout_s=0` теперь ЯВНО = дефолт 90с; крошечные
  значения зажаты снизу до 10с (rollback не сработает раньше одного probe-цикла). Чистый
  `normalizeRollbackTimeout`, юнит-тест.
- **`feat(daemon)` 9dcf293** — single-instance guard: advisory flock
  `$RUN_DIR/keen-manager.lock`; второй демон падает с понятным сообщением (pid держателя).
  Ядро снимает flock при смерти процесса → не залипает (в отличие от голого pid-файла).
  Read-only каталог = warning, не фатально. `platform.Lock` + `ErrLocked`, юнит-тест + e2e.
- **`feat(engine)` aaeba95** — failover backoff+jitter, когда ВСЯ цепочка лежит: раунд
  «никто не доступен» ставит экспоненту (30с база, ×2, потолок 5м) с джиттером вместо
  долбёжки каждый тик; сброс при switch/recovery/смене конфига. Чистый `backoffDelay`.
- **`feat(engine)` efb9d4c** — per-attempt timeout на активациях из петли failover:
  `Activate` протянут контекстом, петля зовёт `activateWithin` (бюджет rollback + 45с),
  чтобы зависший bring-up (застрявшая докачка xray / незавершаемый verify) НЕ застопорил
  health-горутину. Интерактивные активации — без лимита. Юнит-тесты (`sleepCtx`, verify-cancel).
- **`feat(cli)` 5d2ee2e** — CLI-паритет по failover: `keen-manager failover
  show|on|off|chain|interval|threshold|autoreturn|probe`; `chain` валидирует id против
  подключений (+`direct`). Чистый `NormalizeFailoverChain`, юнит-тест.

⚠️ **Ничего on-device в этой сессии не трогалось** — только код/сборка/релиз. P0 read-back
(шейп Proxy-интерфейса + флаг «выход в интернет») и P1-валидация tpws ПО-ПРЕЖНЕМУ висят и
остаются ГЛАВНЫМ гейтом для настоящего stable `v0.1.0` (команды — в блоках 9-й/10-й сессий ниже).

### 10-я сессия (ЗАКРЫТА) — nfqws как routable-интерфейс через tpws (P1)
Реализована фича **P1** (юзер просил nfqws «как Xray» — routable IP:port,
**НЕ** глобальный инлайн-NFQUEUE). Всё в `main`, сборка зелёная (`go
build/vet/test` + кросс mipsle/arm64 + веб-бандл `tsc`/`vitest 17`; CI-гард
бандла проходит). Коммиты:
- **`feat(tpws)` ddc0a8e** — новый пакет `internal/tpws`: `Controller`
  супервизит локальный **`tpws`** (сокетный desync-прокси из zapret, сестра
  nfqws) в **SOCKS-режиме** на `127.0.0.1:10809` через генерируемый init-скрипт
  `S52tpws` (порт + стратегия зашиты в argv; переконфиг = перезапись скрипта +
  рестарт — аналог `xray.Controller.Apply`). `Options.Args` строит argv;
  стратегия — свободная строка, тюнится на устройстве, дефолт
  `--split-tls=sni --disorder`. Всё через `platform.Runner` (инертно в dry-run);
  отсутствие бинаря `tpws` детектится (`Installed`) и отдаётся хинтом, не ломает
  роутер. + `platform.Paths.TpwsBin` (`/opt/usr/bin/tpws`). Юнит-тесты.
- **`feat(engine)` 32ac213** — `engine/bypassconn.go` зеркалит `proxyconn.go`
  1:1: `ensureManagedBypassIface`/`teardownManagedBypassIface`/`bringUpBypass`/
  `bringDownBypass`/латч `bypassClientDown`/`reconcileBypass` (на бут). ОДИН
  управляемый `ProxyN` → tpws, `State.ManagedBypassIface`, `model.Bypass{Enabled,
  Port,Strategy,Seeded}`. **ТОТ ЖЕ анти-петля-урок:** bypass-`ProxyN` — только
  цель для per-domain маршрутов, `ip global` НЕ ставим. **Домены — ОБЩИЙ
  источник с Routes:** маршрут целится в зарезервированную цель `bypass`
  (`bypassTargetID`); `resolveRouteIface` отдаёт управляемый iface;
  Create/UpdateRoute пропускают проверку `findConn` для сентинела; view метит
  «DPI Bypass». **Дефолт-сид** при первом включении — Discord + YouTube из
  `internal/presets` (guard `Bypass.Seeded`, юзер может править/удалять). API
  `GET/PUT /api/bypass`. Юнит-тесты (сентинел, seed-once, валидация порта,
  teardown, view).
- **`feat(web)` 6c32b96** — Bypass-страница: карточка «Routable bypass interface»
  (тумблер вкл/выкл = запуск tpws + регистрация ProxyN, редактор стратегии tpws
  + порт, статус installed/running + имя интерфейса; домены тут НЕ выбираются —
  указывает на Routes). Routes-пикер: группа «DPI bypass» с целью `conn:bypass`
  (показывается, когда фича включена). `api.bypass()/saveBypass()`, тип
  `Bypass`, `mockBypass` (DEV), EN+RU. Бандл пересобран в ТОМ ЖЕ коммите.

⚠️ **ПЕСОЧНИЦА НЕ ВИДИТ РОУТЕР.** SSH `root@192.168.1.1` — приватная LAN,
из облачной песочницы недостижима (и SSH за файрволом закрыт: только HTTP/S).
Поэтому ВСЕ on-device задачи этой сессии (**P0 read-back**, чек-лист XRAY §5,
проверка наличия `tpws` в фидах) **НЕ выполнены** — только подготовлены команды
(P0-блок 9-й сессии ниже + P1-чек-лист здесь). Фича tpws построена **защитно**
(рантайм-детект tpws + Proxy client, graceful-хинт, dry-run-aware): если `tpws`
есть в opkg-фидах — routable-обход заработает; если нет — отдаётся хинт, роутер
не ломается, а инлайн-NFQUEUE (nfqws2) остаётся как был.

#### P1-ВАЛИДАЦИЯ (10-я, on-device) — tpws-обход, СНЯТЬ С УСТРОЙСТВА
```sh
# 1) tpws в фидах? (если НЕТ — обсудить с юзером: без сокет-прокси routable-
#    интерфейса у DPI-обхода не будет; инлайн-NFQUEUE остаётся, но это не то):
opkg update; opkg list | grep -i tpws; which tpws; ls -l /opt/usr/bin/tpws
# 2) включить «Обход DPI как интерфейс» в вебе (или PUT /api/bypass {enabled:true}),
#    затем: один ProxyN в «Прочие подключения → Прокси», upstream 127.0.0.1:10809,
#    socks5, connected:
curl -s http://localhost:79/rci/show/interface/ | grep -o 'Proxy[0-9]*' | sort -u
# 3) КЛЮЧЕВОЙ — tpws реально десинхронит (домен, закрытый напрямую, открывается
#    через SOCKS tpws):
curl -s -x socks5h://127.0.0.1:10809 -m 12 https://<заблокированный-домен>/ -o /dev/null -w '%{http_code}\n'
# 4) Route с целью «DPI Bypass» → dns-proxy route на этот ProxyN, туннелит ТОЛЬКО
#    выбранные домены (по умолчанию засеяны Discord + YouTube).
# 5) подобрать стратегию tpws под провайдера на странице Обход (поле «стратегия»):
#    напр. --split-tls=sni --disorder | --split-pos=1 | --oob | --hostcase (тюнить).
```

### 9-я сессия (ЗАКРЫТА) — баг-репорты с боевого роутера + релиз beta.9
Юзер накатил beta.8 на живой роутер и прислал баги. Отгружено в main, выпущен
**`v0.1.0-beta.9`** (release.yml зелёный, 5 ассетов, prerelease). Коммиты:
- **`fix(engine)` 6d064ed — петля маршрутизации (ГЛАВНЫЙ баг).** `ProxyN`
  помечался `ip global` («использовать для выхода в интернет») → кандидат в
  дефолтное подключение. У SOCKS-интерфейса (в отличие от WG) нет пиннинга
  endpoint'а сервера, поэтому собственный трафик роутера (UDP DNS + исходящее
  Xray к vless-серверу) заворачивался `ProxyN → SOCKS → Xray → снова ProxyN` =
  петля; TCP-only SOCKS роняет UDP DNS. Это и есть «штормит / отваливается / ни
  один сайт не грузится / без политики заворачивает весь трафик в себя». Фикс:
  `ProxyN` теперь ТОЛЬКО цель для маршрутов по доменам (dns-proxy route, как
  AWG); global НЕ ставим → WAN дефолт, петли нет. AWG не тронут.
- **`feat(routes)` 0a48795 + `feat(web)` 11d9294 — редактирование маршрута.**
  `GET /api/routes/{id}` (полный список доменов) + `PUT /api/routes/{id}`
  (`Engine.UpdateRoute`, чистый тир-даун старой формы). Веб: нажатие на правило
  (или карандаш) открывает боковой редактор со ВСЕМИ доменами. EN+RU. Юнит-тест.
- **`feat(web)` b1bf98c — тумблер «Интеграция Xray»** (auto/proxy/tproxy) на
  Настройках. Приземлён из `wip/web-xray-toggle` (взяты ТОЛЬКО 4 веб-файла, не
  устаревший backend ветки), бандл пересобран (Node 24), ветка удалена.

⚠️ ВАЖНО про фикс петли: юзер сообщил, что галочка «использовать для выхода в
интернет» НЕ прожимается → значит шейп `{"ip":{"global":true}}` на его прошивке
не срабатывает. Поэтому beta.9 (не ставить global) безопасен (стейл-флага нет),
но это может быть НЕ вся причина петли — точный механизм не подтверждён на
устройстве. Нужен read-back (ниже).

#### P0 СЛЕДУЮЩЕЙ СЕССИИ — снять данные с устройства (юзер не успел прислать)
SSH `root@192.168.1.1 -p 222`, RCI на loopback без auth. Команды (заменить
`Proxy0/Proxy1` на реальные имена из шага 2), вывод → в тред:
```sh
# 1) КЛЮЧЕВОЙ: несёт ли SOCKS-туннель Xray трафик вообще (разводит петля/флаг vs
#    туннель-не-работает-DPI). Должен вернуть IP СЕРВЕРА, не WAN:
curl -s -x socks5h://127.0.0.1:10808 -m 12 https://api.ipify.org; echo
curl -s -m 12 https://api.ipify.org; echo                 # для сравнения (WAN IP)
# 2) имена Proxy-интерфейсов:
curl -s http://localhost:79/rci/show/interface/ | grep -o 'Proxy[0-9]*' | sort -u
# 3) КОНФИГ-ШЕЙП руками созданного прокси (с галочкой) — авторитет для write+флага:
curl -s http://localhost:79/rci/show/rc/interface/Proxy0
# 4) прокси от keen-manager (описание "keen-manager (Xray)") — для сравнения:
curl -s http://localhost:79/rci/show/rc/interface/Proxy1
# 5) как выглядит global/приоритет интернета в rc:
curl -s http://localhost:79/rci/show/rc/ip | head -c 4000; echo
# 6) кто дефолтный маршрут (гипотеза «всё в прокси»):
curl -s http://localhost:79/rci/show/ip/route | head -c 4000; echo
# 7) живой статус прокси (флапает?):
curl -s http://localhost:79/rci/show/interface/Proxy1
```
Когда придёт: (a) привести `proxyInterfaceBody`+тест к реальному шейпу (шаг 3/4);
(b) флаг «для интернета» завести на САМЫЙ НИЗКИЙ приоритет (WAN дефолт) — шаг 5;
(c) добавить пиннинг host-route на IP активного сервера через WAN (belt-and-
suspenders против петли для full-tunnel). Если шаг 1 не вернул IP сервера —
туннель к blanc не несёт трафик (DPI reality/TLS), снять `xray -test` + логи
`xray run`; это отдельный корень, не роутинг.

#### P1 СЛЕДУЮЩЕЙ СЕССИИ — nfqws как routable «интерфейс» + выбор доменов (СПЕЦА ниже)
Юзер уточнил: хочет nfqws «как Xray» — **чтобы наружу отдавался IP:port и этот
интерфейс можно было назначить на роутинг** (в KeenOS или у нас), НЕ глобальный
инлайн-NFQUEUE. Ответы юзера: домены **из общего источника с Routes**; дефолт при
инициализации — **как уже есть, Discord и YouTube** (взять домены из пресетов
`internal/presets`, НЕ выдумывать).
Дизайн (реализовать в след. сессии, зеркалить Xray-Proxy-плумбинг):
- nfqws (NFQUEUE) сокет-эндпоинта НЕ даёт. Чтобы получить routable IP:port,
  нужен **сокетный desync-прокси — `tpws`** (сестра nfqws из zapret: transparent/
  SOCKS-прокси прикладного уровня, десинхронит на своём порту). Модель 1:1 с Xray:
  `tpws` слушает `127.0.0.1:<port>` → регистрируем ОДИН управляемый Keenetic
  `ProxyN` на него (переиспользовать `keenetic/proxyiface.go` + паттерн
  `engine/proxyconn.go`) → Routes вешают dns-proxy route на этот `ProxyN`.
  ПРОВЕРИТЬ на устройстве наличие `tpws` в opkg-фидах (рядом с
  `nfqws-keenetic`/`nfqws2-keenetic`/`hoaxisr`). Если tpws нет — обсудить с юзером
  (без сокет-прокси routable-интерфейса у DPI-обхода не будет; инлайн-NFQUEUE
  остаётся, но это не то, что просил).
- **Тот же анти-петля-урок:** этот bypass-`ProxyN` тоже НЕ метить global —
  только цель для per-domain маршрутов.
- **Общий источник доменов с Routes:** добавить bypass-интерфейс как ещё одну
  ЦЕЛЬ в пикере Routes (рядом с AWG/Xray/интерфейсами) — один источник правды,
  Route можно нацелить на «обход DPI». Дефолт-сид: домены из пресетов Discord +
  YouTube (`internal/presets`), создавать при инициализации/первом включении.
- Стратегии nfqws/tpws — внутри (страница Обход), из UI выбираются только домены.

### 🔴 ПРИОРИТЕТЫ СЛЕДУЮЩЕЙ СЕССИИ (кратко — детали в блоках 9-й/10-й/11-й сессий выше)
P1 (**nfqws как routable-интерфейс через tpws**) приземлён в 10-й; P2-hardening
(single-instance, failover backoff, per-attempt timeout, rollback=0) + фикс установщика
+ CLI-паритет по failover — в 11-й. Выпущен `v0.1.0-rc.1`. **Единственный гейт для
настоящего stable `v0.1.0` — on-device валидация** (из облачной песочницы недостижимо;
выполнить на роутере и прислать вывод):
1. **P0 — снять Xray-Proxy read-back с устройства** (команды в блоке 9-й сессии). По
   нему: привести `internal/keenetic/proxyiface.go::proxyInterfaceBody` (+тест) к
   реальному шейпу (сейчас ДОГАДКА из плана §3, изолирована в одной функции; шейп
   ОБЩИЙ для Xray- и bypass-ProxyN; отказ RCI латчит down → хинт/TPROXY, не ломает)
   и правильно завести флаг «использовать для выхода в интернет» (низкий приоритет).
2. **P0 — on-device чек-лист Xray** (XRAY-PROXY-PLAN §5): активация Xray → один
   `ProxyN` (socks5, `127.0.0.1:10808`, connected); смена сервера → тот же `ProxyN`,
   меняется только внешний IP; Route на Xray → dns-proxy → `ProxyN`; удаление
   последнего Xray → `ProxyN` снят. Ключевой тест — `curl -x socks5h://127.0.0.1:10808`.
3. **P1-валидация tpws-обхода** (чек-лист в блоке 10-й сессии): tpws в фидах?
   включить → один `ProxyN` (socks5, `127.0.0.1:10809`); `curl -x
   socks5h://127.0.0.1:10809` реально десинхронит заблокированный домен; Route на
   «DPI Bypass» туннелит только выбранное; подобрать стратегию tpws под провайдера.
   Если `tpws` НЕТ в фидах — обсудить с юзером (без сокет-прокси routable-обхода нет).

**Код-сайд (можно в песочнице, роутер НЕ нужен) — детали в ROADMAP P2/P3:** структурный
nfqws в CLI (парный к `failover`); through-tunnel reachability узлов цепочки перед выбором;
nfqws2 parity (lua/log/самообновление, `ISP_INTERFACE` autodetect, проверка ndm-хука);
дашборд live-трафик; hysteria2/tuic. После того как on-device P0 закрыт и шейп
`proxyInterfaceBody` подтверждён — тегнуть stable **`v0.1.0`** (без суффикса = не prerelease).

### 8-я сессия — Xray как одно «Прокси-подключение» KeenOS (бэкенд, в main)
Модель: keen-manager держит локальный Xray с SOCKS-инбаундом `127.0.0.1:10808` и
регистрирует ОДИН управляемый интерфейс `ProxyN` → этот SOCKS. Смена сервера/
«выбрать лучший» переписывает ТОЛЬКО конфиг Xray; `ProxyN` не трогаем — одно
стабильное подключение, видимое в UI. Маршруты вешаются на `ProxyN` штатным
`dns-proxy route`, как на AWG. TPROXY остаётся фолбэком. Слайсы (все в main):
- **`feat(xray)` 1818687** — `Options.ProxyConnMode`: минимальный SOCKS-only
  конфиг (socks-inbound + один pinned-сервер + direct/block; без tproxy-inbound,
  без api/observatory, без in-xray split). Юнит-тест.
- **`feat(keenetic)` e6e75d0** — `proxyiface.go`: RCI-хелперы Proxy-интерфейса
  (`FindFreeProxyIndex`/`CreateProxyInterface`/`ProxyConnect`/`SetProxyUpstream`),
  `Capabilities.HasProxyClient`, `InterfaceInfo.IsProxy`. Шейп в
  `proxyInterfaceBody` — догадка из плана §3, изолирована и юнит-тестируется;
  отказ RCI = ошибка → фолбэк в TPROXY. **СВЕРИТЬ НА УСТРОЙСТВЕ.**
- **`feat(engine)` 59f6a7f** — `proxyconn.go`: `xrayMode()` (auto|proxy|tproxy +
  латч `proxyClientDown`), `ensureManagedProxyIface()` (создать ОДИН `ProxyN`,
  переиспользовать между серверами), teardown при удалении последнего Xray,
  `bringUpXrayProxy()`. `apply.go`/`routes.go`/`connections.go`/`interfaces.go`:
  `buildActiveXray` ветвится по режиму; Xray-маршруты в proxy-режиме идут на
  `ProxyN` через dns-proxy (как AWG), in-xray split — только для TPROXY;
  `integrationOf` → `keenetic-proxy` (visible/routable); Proxy-интерфейсы
  routable. `model.Settings.XrayIntegration` + `State.ManagedProxyIface`.
Сборка зелёная: `go build/vet/test` + кросс mipsle/arm64. Релиз beta.8.

**7-я сессия закрыла жалобу юзера из скриншота/лога** (Xray-активация падала,
«нет единого интерфейса», «маршруты только awg»). Релиз `v0.1.0-beta.6` выпущен; фикс
зависимостей Xray (ObservatoryService) → **`v0.1.0-beta.7`**. Сборка зелёная: `go build/vet/test`, `tsc`,
кросс mipsle — всё exit 0; бандл детерминирован (Node 24, working==index, CI-гард проходит).

### 7-я сессия — Xray-активация, маршруты через Xray, единая точка выхода (в main)
- **`fix(xray)` 572827c** — активация Xray падала с *«Failed to get format of
  …/config.json.tmp»*. Xray определяет формат конфига по расширению, а pre-apply
  temp пишется как `config.json.tmp` → `xray -test -config …tmp` падал ДО разбора
  тела. `Controller.Validate` теперь передаёт `-format json` (keen-manager всегда
  пишет JSON). Суффикс `.tmp` оставлен намеренно: `xray run -confdir` мерджит
  только `*.json/*.yaml/*.toml`, поэтому недописанный temp не подхватится; стейл-temp
  чистится перед записью. Юнит-тест `internal/xray/control_test.go`. **Это и есть
  фикс «по xray подписке проблема».**
- **`fix(xray)` 4adcb72** — после того как конфиг стал реально загружаться, вылезла
  вторая ошибка: *«core: not all dependencies are resolved»*. Блок `api` всегда
  перечислял `ObservatoryService`, но observatory (`burstObservatory`) создаётся
  только в режиме балансировщика — одиночный (pinned) конфиг, который строят активация
  и select-best, анонсировал gRPC-сервис без стоящей за ним фичи → xray-core падал на
  старте. Теперь `api.services` собирается из реально присутствующих фич (Handler/Stats/
  Routing всегда; Observatory — только с балансировщиком). Регресс-тест в `config_test.go`.
- **`feat(routes)` a9fbffe** — (1) **маршрут можно нацелить на Xray-подключение,
  не только AWG.** Если на Xray-подключение указывает ≥1 включённый маршрут, его
  активный конфиг строится в режиме **split-tunnel**: только домены/подсети маршрутов
  идут в серверный outbound, остальное — в `direct` catch-all. Без маршрутов — полный
  туннель (как раньше). Домены → матчер Xray `domain:` (+сабдомены); уже-префиксные
  проходят как есть. Apply/remove на активном подключении = rebuild→revalidate→restart
  Xray; на неактивном — pending, вкомпилируется при активации. (2) **Единая точка
  выхода для нативного AWG:** после успешного переключения интерфейс `WireguardN`
  ПРЕДЫДУЩЕГО активного подключения (и привязанные к нему маршруты) сносится — перебор
  локаций больше не плодит интерфейсы. Только post-verify → провальное переключение не
  убьёт рабочий туннель (откат вернёт прежний). Юнит-тесты: `internal/xray/config_test.go`,
  `internal/engine/routes_xray_test.go`.
- **`feat(web)` 9588f0f** — пикер целей маршрута получил группу «Xray-туннели» рядом
  с AWG-подключениями и живыми интерфейсами роутера; предупреждение о нативной
  DNS-маршрутизации скрыто, когда доступны только Xray-цели (Xray-маршруты не
  используют dns-proxy). Бандл пересобран в том же коммите (CI-гард зелёный). EN/RU.

### Осталось (7-я сессия) — валидировать на устройстве
- Xray-подписка `blanc`: активация теперь проходит `xray -test`; проверить, что туннель
  реально несёт трафик (если нет — вероятно DPI рвёт reality/TLS: сменить probe-target
  на Failover, снять `xray run` логи).
- Маршрут на Xray-подключение: убедиться, что через туннель идут ТОЛЬКО выбранные сервисы,
  остальное — напрямую.
- AWG: переключение локаций оставляет РОВНО один `WireguardN` на роутере.

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

1. **On-device валидация (главное)** — 7-я сессия (см. §0): Xray-активация проходит
   `xray -test` — проверить, что туннель несёт трафик (иначе DPI/probe-target); маршрут
   на Xray-подключение гонит через туннель ТОЛЬКО выбранные сервисы (split), остальное
   напрямую; перебор AWG-локаций оставляет РОВНО один `WireguardN`. Плюс из прошлого:
   AWG-подключение создаёт `WireguardN` (виден/роутит/сносится); маршрут в выбранный
   интерфейс; user2.list/NFQUEUE-бейдж/nfqws-guard. Ломать бету на живом роутере нельзя.
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
