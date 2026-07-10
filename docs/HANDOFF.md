# Handoff — keen-manager (для следующего агента)

Обновлено: 2026-07-10 (**17-я сессия**). Репозиторий: `github.com/miroslavrov/keen-manager`,
ветка `main`. Коммиты от лица **miroslavrov** (git-credentials уже в песочнице; токен
пользователь передаёт отдельно, в репозиторий НЕ коммитить). Пользователь на **KeeneticOS**
(прошивка отдаёт release-строку `"5.01.C.0.0-1"`, это 5.1.0), arch **arm64**, тестирует
бету на живом роутере — ломать нельзя.

Продукт: один Go-бинарь (демон + REST/JSON + SSE + CLI) со встроенной React/shadcn
мордой. Единый менеджер VPN (Xray/AmneziaWG) + обход DPI (nfqws2) для Keenetic.

---

## 0. Сессии 5–17 — что сделано и что осталось

### 17-я сессия (КОД + РЕЛИЗ) — доделан P1-UI (тумблер потока подписки = 3-й уровень выхода), P0.3 довёрстан на фронте; выпущен `v0.1.0-rc.4`. P0-гейт на устройстве ОТКРЫТ (нужны diag v4 + проверка rc.4)
**СТАРТОВАЯ СВЕРКА (юзер: «билда нет, перепроверь что осталось»):** предыдущий безымянный прогон УЖЕ закоммитил в `main` P0.1/P0.3/P1-бэкенд (HEAD дошёл до `50ccc39`), но НЕ доделал P1-UI, НЕ собрал релиз и НЕ обновил этот файл. Релиз оставался `rc.3` (собран 11:02Z, на ~9 ч РАНЬШЕ этих коммитов) → на устройстве не было ничего нового. Всё проверено: `go build/vet/test` (зелено) + кросс arm64/arm/mips/mipsle (зелено).

**ЧТО БЫЛО В main НА СТАРТЕ (проверено построчно — корректно):**
- `chore(diag)` `06a04b3` — **diag v4** (P0.1, линчпин). `scripts/diag-tunnel.sh` переписан: (A) freedom-контроль socks→freedom (xray стартует + egress жив → вернёт WAN-IP роутера); (B) reality через plain SOCKS (путь как на телефоне, БЕЗ tproxy/dokodemo → вернёт IP сервера); НИЧЕГО не прячет (echo конфига с редактом секретов, полный вывод + код `xray -test`, «процесс жив/умер», весь лог без `| tail`), watchdog вместо busybox-`timeout`, дерево решений P0.2. Read-only.
- `fix(xray,engine)` `d42389c` — **P0.3**: `normalizeXrayMSS` дефолт ВЫКЛ (клампит только явный `>0`; `0`/`<0` = выкл), `DefaultXrayMSS=1380` теперь лишь «подсказка UI»; `tproxy-in` dokodemo получил `routeOnly:true`.
- `feat(model,engine)` `50ccc39` — **P1-бэкенд**: `model.Subscription.Enabled` + `SubView.Enabled` (json `enabled`), предикат `connEligible(state,conn)=conn.Enabled && subEnabled(sub)` протянут в probe/autoselect/failover/activate/rollback/resumeConnector/connView, миграция схемы **v2** (включает все существующие подписки), выкл потока рвёт активный туннель (LAN→direct), CLI `sub enable|disable`.

**ЧТО СДЕЛАНО В ЭТУ СЕССИЮ:**
- **`feat(web)` `da9cbce` — P1-UI (доделка того, чего не было):** карточка подписки получила тумблер **«Поток»** (иконка Power) — СРЕДНИЙ из трёх уровней выхода. Теперь в UI разведены ВСЕ ТРИ: **(1) master-connector** (Dashboard, `State.TunnelPaused`, session 15) → **(2) поток подписки** (новое) → **(3) per-server `Enabled`** (Connections). Выключенный поток гасит карточку + бейдж «На паузе» + глушит select-best/auto-best (демон их всё равно отбивает). Оптимистичный кэш + инвалидация connections/state (выкл может порвать туннель). Протянуто: `types.ts` (`Sub.enabled`), `api.ts` (`updateSubscription` принимает `enabled`), моки (одна подписка disabled для наглядности), i18n **EN+RU** key-for-key. **Заодно (довёрстка P0.3):** подпись MSS в Настройках больше НЕ врёт «0 = авто 1380» — `0`/`<0` = «выкл (по умолчанию)», положительное = ручной per-ISP оверрайд. Бандл `internal/webui/dist` пересобран в ТОМ ЖЕ коммите (Node 24, `npm ci`; детерминизм проверен — повторная сборка байт-в-байт → CI-гард «verify committed bundle» зелёный). web-смоук 17/17.
- **Релиз `v0.1.0-rc.4`** (тег с дефисом = prerelease; `release.yml` → `make dist` = web + build-all + gzip 4 арки + sha256). Содержит ВСЁ: MSS-off, routeOnly, тумблер потока, diag v4.

**⚠️ ГЛАВНОЕ — P0 НА УСТРОЙСТВЕ (единственный оставшийся гейт; юзер подтвердил: на роутере СТОЯЛ rc.3):**
1. **Поведение изменилось vs rc.3:** в rc.3 MSS-clamp был ВКЛ по умолчанию, в rc.4 — ВЫКЛ. Т.е. rc.4 проверяет гипотезу session 16 «кламп был красной селёдкой». Поставить rc.4, включить `blanc` — несёт ли туннель трафик теперь?
2. **diag v4 (линчпин, билд НЕ нужен — уже в main):** одной строкой на роутере, прислать ВЕСЬ вывод:
   `curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/50ccc393dbc1cec6073b2b565c4e2770fbb4c709/scripts/diag-tunnel.sh | sh`
   Развязка (P0.2): **A ок + B = IP сервера** → reality через plain SOCKS несёт как телефон ⇒ баг в TPROXY/dokodemo-capture ⇒ перевести инсталл на **proxy-client** (`Settings.XrayIntegration="proxy"`); DNS-нюанс denisitpro (явные 1.1.1.1 + `HardenProxyInterface` уже чистит name-servers — проверить). **A ок + B пусто** → reality с роутера не идёт даже через socks (глубже: сервер/транспорт/DPI на узле) ⇒ взять **ws-узел** blanc (не reality-tcp-vision). **A пусто** → xray не стартует / нет прямого инета (читать `xray -test` + лог прямо в выводе).

**ОСТАЛОСЬ СЛЕДУЮЩЕМУ АГЕНТУ:**
- **P0.2-развязка по diag v4** (см. выше) — главный приоритет.
- **Код-гигиена, кандидат №3 из session 16 (НЕ сделан):** TPROXY-правила `internal/route/route.go` без XKeen-овских `-m socket --transparent -j MARK` (established), `-m conntrack --ctstate DNAT/INVALID -j RETURN` и `--on-ip 127.0.0.1`. Capture работает, но это дыры надёжности. Делать, если после diag остаёмся на tproxy; если уходим на proxy-client — неактуально.
- **P1 — закрыт** (UI + бэкенд + релиз). Сверить на устройстве, что три тумблера (коннектор/подписка/сервер) ведут себя как ожидается.

### 16-я сессия (СВЕРКА С КАНОНОМ XKeen; кода НЕ меняли, HEAD остался `993477a`) — P0-гипотеза ПЕРЕСТАВЛЕНА: конфиг ПРАВИЛЬНЫЙ, расходимся в СПОСОБЕ завода трафика в Xray (TPROXY/dokodemo-door у нас vs socks/tun-клиент на рабочих ПК/телефоне)
**Триггер:** юзер попросил свериться со статьёй Habr `ru/articles/990322` («Xray on Keenetic / Xkeen») и другими источниками — «правильно ли мы вообще делаем». Сверили: `Skrill0/XKeen` + активный форк **`jameszeroX/XKeen`** (README + FAQ `jameszero.net/faq-xkeen.htm` + `_xkeen/.../04_register_init.sh`), `xtls.github.io` (tproxy-доки), XTLS/Xray-core (vision/reality, issues). **Кода не трогали — только research + working-doc.**

**ВЕРДИКТ СВЕРКИ — наш конфиг КОРРЕКТЕН, баг НЕ в форме конфига:**
- Наш outbound `vless+reality+flow xtls-rprx-vision` (`internal/xray/outbound.go`) совпадает с каноном XKeen/XTLS **дословно**: `encryption:"none"`, `flow` внутри `users[]`, `network:"tcp"` (vision требует raw TCP), `security:"reality"`, все reality-поля (serverName/fingerprint/publicKey/shortId/spiderX), `sockopt.mark:255` = ровно антилуп-марка XKeen.
- Capture РАБОТАЕТ: device-лог 14-й с тегом `[tproxy-in -> srv-conn]` = соединения принимаются и роутятся в серверный outbound. Значит форма конфига — не причина.

**КОРНЕВАЯ АСИММЕТРИЯ (главный след P0):** на роутере активен режим **TPROXY + dokodemo-door** (inbound-тег `tproxy-in`). ПК/телефон юзера, где ТЕ ЖЕ конфиги РАБОТАЮТ, используют обычный клиент (socks/tun-inbound). Автор XKeen прямо: **«в 99% случаев нет смысла возиться с tproxy/dokodemo-door или маркировкой трафика»**, рекомендует встроенный **Proxy-client Keenetic** (роутер → локальный socks-inbound Xray). У нас режим `proxy` уже реализован (`engine.xrayMode`/`buildProxyConnConfig`), но авто-детект выбрал `tproxy` (или proxy упал в fallback — `isProxyClientDown`). => единственный участок, отличающий роутер от рабочего телефона, — прозрачный capture, а не сам туннель. Возможный механизм (НЕ подтверждён, и неважен для A/B): vision + dokodemo-door задействуют splice-путь Xray, PR #5737 (fix splice-handoff vision) влит ~за 5 дней до сборки 26.3.27 + open-issue #5966 про хрупкость vision-паддинга.

**РАСХОЖДЕНИЯ ОТ КАНОНА (нашёл при сверке):**
1. **MSS-clamp дефолт-ON (`model.DefaultXrayMSS=1380`)** — XKeen **НИГДЕ** не клампит MSS (проверено по всему `04_register_init.sh`). Самодельная догадка 15-й; баг был ДО клампа (14-я — тот же провал без клампа). Кандидат №1 убрать из дефолта.
2. **dokodemo-door inbound без `routeOnly:true`** (`internal/xray/config.go:221`) — XKeen ставит `routeOnly:true` на прозрачных inbound'ах; у нашего `socks-in` (config.go:209) он есть, у `tproxy-in` — нет.
3. TPROXY-правила (`internal/route/route.go`) без XKeen-овских `-m socket --transparent -j MARK` (established) и `-m conntrack --ctstate DNAT/INVALID -j RETURN`, и без `--on-ip 127.0.0.1`. Capture работает, но это дыры в надёжности.

**⚠️ diag v3 БЫЛА НЕИНФОРМАТИВНОЙ (НЕ считать её доказательством MSS/DPI):** в выводе юзера НЕТ строки версии xray, после `xray -test:` пусто, `listening` пусто, все три `(log empty)` → standalone-xray в том прогоне **вообще не стартовал/не логировался** (вероятно `run_test` не поднял xray, а `| tail -2` это спрятало). v3 просто ничего не измерила.

**ПРИОРИТЕТЫ СЛЕДУЮЩЕМУ АГЕНТУ:**
1. **P0.1 — diag v4 (линчпин, собрать ПЕРВЫМ):** переписать `scripts/diag-tunnel.sh` так, чтобы он ОДНОЗНАЧНО отвечал: (а) контрольный **freedom-outbound** socks-xray — доказывает, что xray на устройстве вообще запускается и egress жив; (б) **socks-inbound + reality** проба — путь как на телефоне, БЕЗ tproxy/dokodemo; (в) **гарантированный вывод даже при падении xray** — НЕ прятать stdout/stderr через `| tail`, печатать код возврата `xray -test`, echo сгенерённого конфига, печатать почему процесс вышел. Запуск одной строкой `curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/<commit>/scripts/diag-tunnel.sh | sh`.
2. **P0.2 — развязка по итогам v4:** socks-xray НЕСЁТ трафик → баг в TPROXY/dokodemo-capture → перевести инсталл на **proxy-client** (`Settings.XrayIntegration="proxy"`, как на телефоне). DNS-нюанс (кейс denisitpro): явные DNS (1.1.1.1) + интерфейс НЕ раздаёт DNS роутера (`HardenProxyInterface` уже чистит `name-servers` — проверить на устройстве). socks-xray ПУСТ, а freedom НЕСЁТ → reality с роутера не идёт даже через socks (глубже — сверить транспорт узла blanc; но противоречит «работает на телефоне»).
3. **P0.3 — код-гигиена по канону:** убрать MSS-clamp из дефолта (`normalizeXrayMSS`/`DefaultXrayMSS` — дефолт «выкл», тумблер оставить); добавить `routeOnly:true` в `tproxy-in`. Сборка `go build/vet/test` + arm64-кросс.
4. **P1 — UI-путаница тумблеров (прямой запрос юзера):** у `model.Subscription` НЕТ поля `Enabled` — юзер тумблит отдельные сервера (`Connection.Enabled` на Connections) и `auto_select_best`, но НЕ саму подписку как поток. Добавить вкл/выкл ПОТОКА подписки и развести в UI три уровня: (1) master-connector = общий выход VPN (session 15, `State.TunnelPaused`, тумблер на Dashboard), (2) подписка вкл/выкл (новое), (3) per-server `Enabled`. Тумблер nfqws/bypass уже есть во вкладке.

**Источники (для перепроверки):** Habr `ru/articles/990322`; `github.com/Skrill0/XKeen`; `github.com/jameszeroX/XKeen` (README, FAQ, `04_register_init.sh`, шаблоны inbounds/outbounds); `xtls.github.io/.../tproxy_ipv4_and_ipv6`; XTLS/Xray-core PR #5737, issue #5966; кейс «туннель встал, сайты стоят из-за DNS» — denisitpro.wordpress.com.

### 15-я сессия (КОД + РЕЛИЗ) — P0-фикс мёртвого туннеля ОТГРУЖЕН (outbound MSS-clamp, дефолт ON) + P1-инструментовка (свой лог Xray → причина в ошибке) + master-connector (бэк+веб) ; выпущен `v0.1.0-rc.3`
**ПОВОРОТНЫЙ ФАКТ ОТ ЮЗЕРА (снимает всю 14-ю DPI-гипотезу):** те же самые
vless-reality-vision конфиги **подключаются с ПК и телефона юзера в ТОЙ ЖЕ сети** —
не работает ТОЛЬКО роутер. Это исключает общий DPI и сервер (они бы убили и ПК/телефон)
и указывает на **router-LOCAL причину**. Это ровно ветка «Да» из P0.1 дерева 14-й сессии →
копать keen-manager/MTU-специфику, НЕ сервер/DPI.

**ГЛАВНЫЙ ПОДОЗРЕВАЕМЫЙ — MSS/MTU (и отгруженный фикс).** LAN-трафик, ФОРВАРДЯЩИЙСЯ
через роутер, KeeneticOS клампит MSS-to-PMTU; а СОБСТВЕННЫЙ egress Xray к серверу — это
router-локальный сокет (цепочка OUTPUT), который НЕ клампится. На WAN с уменьшенным MTU /
под ТСПУ маленький reality-хендшейк проходит, а полноразмерные data-сегменты чёрно-дырятся →
«туннель встаёт, payload не идёт» (ровно улика 14-й: соединения приняты, ошибок reality нет,
но наружу пусто). **Фикс: клампить MSS исходящего через Xray-sockopt `tcpMaxSeg`.**

**ЧТО В `main` (session-15, все зелёно: go build/vet/test + arm64-cross + веб-бандл + CI-гард):**
- **`feat(engine,xray)` fb73881 (Go-часть, была уже в main на старте сессии):**
  - `Settings.XrayMSSClamp` (`tcpMaxSeg` на server-outbound) — **0 = дефолт 1380 (клампит!),
    <0 = выкл, >0 = явный MSS**. `applyMSSClamp` в config.go бьёт ОБА режима (TPROXY и
    proxy-conn — egress router-локален в любом). `normalizeXrayMSS`/`DefaultXrayMSS=1380`.
    **Дефолт ВКЛючён** — кламп безвреден на здоровом пути и самый вероятный фикс тут.
  - **P1-инструментовка (сэкономила бы всю 14-ю):** Xray пишет СВОЙ error-лог в известный
    файл (`Controller.ErrorLogPath` = `xray/xray-error.log`), тумблер `Settings.XrayLogLevel`
    (`warning` деф / `debug` / …), а провал активации **тейлит лог и дописывает настоящую
    причину** (`; xray log: …` — reset / i/o timeout / REALITY mismatch) в текст ошибки вместо
    голого «did not carry traffic». Лог трункейтится перед каждым bring-up (`TruncateErrorLog`).
    Файлы: `internal/xray/config.go`, `internal/xray/control.go`, `internal/engine/apply.go`
    (`verifyActive`→`xrayFailureReason`→`distillXrayFailure`, чистые, юнит-тесты).
  - **Master connector switch (прямой запрос юзера — «один тумблер общего выхода»):**
    `State.TunnelPaused/PausedConnID`, `SetConnectorEnabled` рвёт активный туннель (LAN→прямой
    путь) и стопит петли (failover/auto-select/nfqws-guard/boot-reconcile все уважают паузу),
    либо восстанавливает запомненное подключение. Любая интерактивная активация снимает паузу.
    `POST /api/connector`, `StateView.connector_enabled`, CLI `connector on|off|show`.
- **`chore(diag)` 1d1db0a (была в main):** `scripts/diag-tunnel.sh` **v3** — read-only,
  гоняет туннель через локальный SOCKS тремя способами (**xray-plain** = как шлёт keen-manager,
  воспроизводит провал; **mss1380** = дефолтный кламп; **mss1280** = агрессивный фолбэк) +
  WAN-MTU/PMTU-контекст + независимая TLS/DPI-проба. Доказывает MSS/MTU на устройстве.
- **`feat(web)` 620a419 (ЭТА сессия — ДОДЕЛАЛ WIP, что «не билдился»):** прошлый агент оставил
  ветку `wip/web-connector-mss` (протянул пропсы + импортнул иконку `Power`, но САМ тумблер не
  отрендерил → tsc падал на unused; + в DEV-моках не было новых required-полей). Доделано и
  влито в main ОДНИМ чистым слайсом (ветка `wip/web-connector-mss` удалена как устаревшая):
  - Дашборд: **строка-тумблер master-connector** (иконка Power) над kill-switch в hero-карточке,
    `POST /api/connector`, оптимистичный кэш + тост; при OFF заголовок «Connector paused».
  - Настройки: **селект loglevel Xray** (warning/debug/info/error/none) + **числовое поле
    MSS-clamp** (0=авто 1380 / <0=выкл / >0=явный) с живой подписью «Сейчас: …».
  - Моки (`connector_enabled`/`xray_log_level`/`xray_mss_clamp`), EN+RU key-for-key, **бандл
    пересобран в ТОМ ЖЕ коммите** (Node 24, детерминирован — CI-гард зелёный).
- **Релиз `v0.1.0-rc.3`** (тег с дефисом = prerelease; `release.yml` собрал ассеты).

**⚠️ ГЛАВНОЕ НА СЛЕДУЮЩЕМ АГЕНТЕ / ЮЗЕРЕ — ON-DEVICE ПОДТВЕРЖДЕНИЕ P0 (единственный оставшийся гейт):**
1. Юзер ставит **rc.3** (кламп уже дефолтно ON) и включает подписку `blanc`. Вопрос: **несёт
   ли туннель трафик теперь** (сайты грузятся, `curl -x socks5h://127.0.0.1:10808 https://api.ipify.org`
   отдаёт IP сервера)? Да → **P0 закрыт**, MSS/MTU подтверждён, можно на stable `v0.1.0`.
2. Если ВСЁ ЕЩЁ пусто — запустить **диаг v3** одной строкой (raw, НЕ релиз-CDN):
   `curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/main/scripts/diag-tunnel.sh | sh`
   — смотреть, какой вариант несёт трафик: если `mss1280` несёт, а `1380` нет → выставить **1280**
   (или ниже) в Настройки → MSS-clamp; если несёт даже `plain` → баг в TPROXY-capture LAN, не в
   outbound; если все пустые + лог `reset`/`timeout` → всё-таки путь к серверу/DPI (вернуть
   14-ю ветку). Теперь **loglevel=debug в Настройках** сам покажет причину в ошибке активации.
3. Веб master-connector + per-route тумблеры вместе закрывают запрос юзера («уметь вкл/выкл и
   подсети, и общий выход») — сверить на устройстве, что OFF реально уводит LAN на прямой путь.

### 14-я сессия (ДИАГНОСТИКА; код НЕ меняли, кроме diag-скрипта) — P0 «туннель не несёт трафик» ЛОКАЛИЗОВАН: reality-хендшейк ВСТАЁТ, но полезные данные не идут (НЕ роутинг / НЕ mark / НЕ параметры)
Юзер прислал ещё on-device read-back'и (включая СОБСТВЕННЫЙ лог Xray). Задача: понять
ПОЧЕМУ туннель не несёт трафик, код пока не трогать. Причина сужена до **канала данных
reality**, НЕ роутинг/хайджек/параметры. Кода не меняли (кроме нового diag-скрипта).

**Активный сервер (config.json, из blanc):** `109.163.239.98:443`, vless, **reality +
flow `xtls-rprx-vision`**, network tcp, SNI `cdn3-87.yahoo.com`, fp firefox,
pbk `CMkW1axrhEXoiJ6anMz9XEjlfqlAtEZya7L0b5ZPMyw`, sid `7e77e7e2cf2b7a79`
(в старой тест-фикстуре `parse_test.go` был `07ddc43269d197c0` — сервер РОТИРОВАЛ sid,
keen-manager подтянул новый корректно), uuid `839d4028-…`. Адрес — **IP** (DNS вне цепочки отказа).

**Что ИСКЛЮЧЕНО (доказательно, on-device):**
- **SO_MARK 255 (0xff)** — НЕ виновен. `ip rule show`: Keenetic-марки `0xffffa00`/`0xffffaaa`,
  правила под `0xff` НЕТ → egress с mark 255 уходит в main table → WAN штатно. Коллизии нет.
- **DPI по SNI** — НЕ режется. Прямой TLS1.3 к `cdn3-87.yahoo.com`@`109.163.239.98:443`
  с устройства → полный хендшейк + **HTTP/2 200** (reality-cover цела, сервер доступен, TCP 0.07s).
- **Порча параметров keen-manager'ом** — НЕТ. `7e77…` нигде не захардкожен; генерации sid нет;
  pbk/sid идут `parse → vault → outbound.go` дословно (проверено grep'ом по internal/).
- **DNS / петля / global-хайджек** — Proxy0 `connected:no` (рулит TPROXY), `global:false` держится,
  адрес сервера IP; как причина мёртвого туннеля не воспроизводится.

**КЛЮЧЕВАЯ УЛИКА — лог `/opt/var/log/xray.log` (loglevel warning) в момент сбоя:** десятки строк
`from 192.168.1.x accepted tcp:<google/telegram/...>:443 [tproxy-in -> srv-conn-…]` и **НИ ОДНОЙ**
ошибки reality/TLS/reset. Т.е. на устройстве reality **НЕ** падает в `received real certificate`
/ `processed invalid connection`. Соединения принимаются и роутятся в серверный outbound без
ошибок — но наружу трафик не идёт (сайты стоят, `curl -x socks5h://127.0.0.1:10808` пуст,
проба к `gstatic/generate_204` — timeout).

**ДИАГНОЗ (ведущая гипотеза):** reality-туннель **устанавливается**, но **полезная нагрузка не
проходит**. Подпись **DPI (ТСПУ) душит vless-reality-VISION (TLS-in-TLS)** на этом ISP: хендшейк
(выглядит как обычный TLS к yahoo) пропускается, а данные vision-потока throttl-ятся в ноль →
xray НЕ видит ошибки, LAN висит. Альтернатива — серверная (сервер принимает хендшейк, но не
проксирует). Точно НЕ роутинг/mark/DNS/params.

**⚠️ УРОКИ ПО ИНСТРУМЕНТАМ (не наступать снова):**
- **Песочница делает MITM TLS** (сертификаты `CN=whoami-sandbox-ca`) → **reality в песочнице
  проверить НЕЛЬЗЯ**: локальная репродукция `received real certificate` — артефакт MITM,
  невалидна (on-device лог её опроверг). Reality тестировать ТОЛЬКО на устройстве.
- Роутер busybox: **нет `timeout`**; **jq БЕЗ ONIGURUMA** (нет `test()/match()` — только
  `select(.protocol=="vless" or …)`); многострочная вставка в ash **рассыпается**. →
  Диагностику давать ТОЛЬКО скриптом в репо, запуск одной строкой
  `curl -fsSL https://raw.githubusercontent.com/miroslavrov/keen-manager/<commit>/scripts/<x>.sh | sh`
  (raw.githubusercontent работает; GitHub-РЕЛИЗЫ у юзера часто `curl (35) reset`/`(28) timeout`).
- Добавлен **`scripts/diag-tunnel.sh`** (standalone-xray nomark/mark + TLS/DPI-проба + endpoint TCP;
  read-only, сервис/iptables/маршруты не трогает).

**ПРИОРИТЕТЫ СЛЕДУЮЩЕМУ АГЕНТУ:**
1. **Развести DPI-vs-сервер (1 вопрос юзеру):** подключается ли blanc в приложении на ТЕЛЕФОНЕ
   юзера в ТОЙ ЖЕ сети прямо сейчас? Да → не общий DPI/сервер, копать keen-manager-специфику
   (MTU/фрагментация/sockopt/vision). Нет → сервер/DPI, keen-manager ни при чём (нужна свежая
   подписка / другой узел).
2. **Транспорт:** взять из blanc узел на **ws** (не reality-tcp-vision) либо reality без `flow`;
   сравнить, несёт ли трафик. Подписка «reality+ws» — ws-узлы могут проходить ТСПУ.
3. **КОД (наивысшая ценность):** сейчас keen-manager на провале пишет только «the tunnel did not
   carry traffic (context deadline exceeded)». Надо **захватывать собственный лог/stderr Xray и
   вставлять причину в ошибку активации** (+ debug-тумблер логлевела Xray). Это бы сэкономило всю
   14-ю сессию. Файлы: `internal/xray/config.go` (`Log.Error` → файл + опция loglevel),
   `internal/xray/control.go` (читать хвост лога), `internal/engine/apply.go` (`verifyActive` —
   добавить xray-reason в текст ошибки).
4. **Свежие device-логи Xray:** остановить сервис (`S99keen-manager stop`), выставить loglevel
   debug, `xray run -confdir …` в форграунде, `curl -x socks5h://127.0.0.1:10808`, читать.
   (`S99xray restart` при engine-запущенном xray может не переподхватиться — pidfile/pkill рассинхрон:
   в этой сессии restart НЕ дописал свежих строк, лог был от прошлого сбойного прогона.)
5. Если DPI-по-vision подтвердится — гнать vision-данные через обход (nfqws/tpws, P1) либо
   предпочитать non-vision/ws-узлы.

### 13-я сессия (В РАБОТЕ) — Proxy0-хайджек починен (код в main); НО 3-й прогон вскрыл: активен TPROXY и туннель НЕ несёт трафик (новый P0 — см. подраздел «3-й прогон» ниже)
Юзер прислал **on-device read-back** (то, чего ждали все сессии с 8-й — облачная
песочница LAN роутера не видит). Его жалоба: при включённом Xray-туннеле **весь**
трафик (и со сплит-роутами, и без) утягивается в туннель, ничего кроме локальных
IP не грузится; как только подключение падает — сразу всё ок.

**Данные с роутера (KeeneticOS 5.1.0, arm64):**
- Единственный Proxy-интерфейс — `Proxy0` (description «keen-manager (Xray)»),
  upstream `127.0.0.1:10808`, `up:true`, **`security-level {public:true}`**,
  **`ip.name-servers:true`**. `ip global` НЕТ (фикс 9-й на месте). `Proxy1` не
  существует. `curl -x socks5h://127.0.0.1:10808 https://api.ipify.org` — **висит**
  (туннель НЕ несёт трафик). `curl https://api.ipify.org` НАПРЯМУЮ с роутера —
  тоже **висит**. `tpws` в opkg-фидах **НЕТ** (для P1-обхода — отдельно).

**Корень (диагноз):** снятие `ip global` в 9-й было НЕОБХОДИМО, но НЕ достаточно.
На 5.1.0 **подключённый `security-level public` Proxy-интерфейс сам попадает в
приоритет интернет-доступа (кандидат в дефолт-роут)** независимо от флага `ip
global` (потому у юзера галочка «выход в интернет» и «не прожималась» — интерфейс
уже в группе), а публичному интернет-интерфейсу вешается **`ip name-servers`** →
DNS роутера идёт через дохлый TCP-only SOCKS. Итог: (a) весь LAN в чёрную дыру,
(b) весь DNS висит, (c) собственный аплинк Xray к vless-серверу заворачивается
обратно в ProxyN — петля, туннель не поднимается. Это ровно жалоба юзера.

**Уточнение корня по 2-му прогону на устройстве (важно):** зона НЕ виновата.
`ip global false` на устройстве вернул `"Proxy0": global priority cleared` —
значит firmware САМ навесил интерфейсу global-приоритет (интернет-доступ), и
именно его надо ЯВНО снять (9-я лишь перестала его ВЫСТАВЛЯТЬ, но не снимала
авто-назначенный). `ip name-servers false` → `ignore IPv4 name servers` (ок).
Запись зоны строкой (`"security-level":"private"`) **отклонена** RCI
(`no input`) — нужна ОБЪЕКТНАЯ форма `{"security-level":{"public":true}}`.
IPv6 `name-servers` остаётся отдельным полем (`ipv6.name-servers`) — гасить тоже.

**Фикс (в `main`, слайсы `fix(engine)` 460d6a8 + корректировка):**
- `keenetic/proxyiface.go::proxyInterfaceBody` пиннит **`ip global:false` +
  `ip name-servers:false` + `ipv6.name-servers:false`**, зона остаётся
  **`public`** (правильная для интернет-egress прокси; баг был в приоритете, НЕ
  в зоне), записывается в объектной форме. Интерфейс — цель для per-domain
  маршрутов, из дефолт-роута исключён снятием global-приоритета.
- `keenetic.HardenProxyInterface(name)` перезакрепляет это на УЖЕ созданном
  интерфейсе (чистит global+name-servers v4/v6, зону НЕ трогает; лечит инсталлы
  прошлых билдов БЕЗ пересоздания — маршруты выживают). Зовётся из
  `engine.reconcileProxyIface()` на бут и из `ensureManagedProxyIface` на активации.
- Юнит-тесты обновлены (объектная форма зоны + v6). Сборка зелёная:
  `go build/vet/test` + кросс mipsle/arm64. Веб не трогался.

⚠️ **2-й прогон НЕ воспроизвёл баг:** Xray в тот момент был ВЫКЛючен
(`ps|grep xray` пусто, на `:10808` никто не слушает, `curl -x socks5h` пусто —
refused, оба обычных curl отдали 301/200). Т.е. интернет работал просто потому,
что туннель лежал. Global-приоритет и name-servers мы сняли, но под НАГРУЗКОЙ
(туннель ВКЛ) фикс ещё не проверен, и неясно, почему Xray не поднят.

⚠️ **СЛЕДУЮЩЕЕ НА ЮЗЕРЕ (решающий тест, туннель ВКЛючён):** включить Xray-подписку
и снять: (1) поднялся ли Xray (`ps|grep xray`, слушает ли `:10808`) и «connected»
ли `Proxy0`; (2) не вернулся ли global-приоритет `Proxy0` при реконнекте (гипотеза
про авто-переназначение — если да, придётся уводить в `private`/низкий приоритет);
(3) не хайджекнут ли интернет (raw-ip + by-name); (4) несёт ли туннель трафик
(`curl -x socks5h://127.0.0.1:10808` → IP СЕРВЕРА). Если хайджека нет, а туннель
молчит — отдельный корень DPI reality/TLS (`xray -test` + логи `xray run`,
сменить probe-target на Failover).

#### P0-ЭКСПЕРИМЕНТ 13-й (on-device) — RCI-формы, ПОДТВЕРЖДЁННЫЕ на устройстве
```sh
B=http://localhost:79/rci
# ЗОНА — только объектная форма (строка => RCI "no input"):
curl -s $B/ -H 'Content-Type: application/json' \
  -d '{"interface":{"Proxy0":{"security-level":{"public":true}}}}'; echo
# СНЯТЬ хайджек-векторы (ЭТО и есть фикс; global-приоритет + DNS v4/v6):
curl -s $B/ -H 'Content-Type: application/json' \
  -d '{"interface":{"Proxy0":{"ip":{"global":false,"name-servers":false},"ipv6":{"name-servers":false}}}}'; echo
curl -s $B/ -H 'Content-Type: application/json' -d '{"system":{"configuration":{"save":{}}}}'; echo
# РЕШАЮЩИЙ ТЕСТ (сначала ВКЛючить подписку в keen-manager!):
ps | grep -i '[x]ray'; (netstat -ltn 2>/dev/null||ss -ltn 2>/dev/null)|grep 10808
curl -s $B/show/interface/Proxy0 | grep -iE 'state|connect'; echo
curl -s $B/show/ip/route | grep -iE 'default|0\.0\.0\.0|Proxy'; echo
curl -s -m 8 -o /dev/null -w 'raw-ip=%{http_code}\n' https://1.1.1.1/
curl -s -m 8 -o /dev/null -w 'by-name=%{http_code}\n' https://api.ipify.org/
curl -s -x socks5h://127.0.0.1:10808 -m 12 https://api.ipify.org; echo  # ждём IP СЕРВЕРА
```

#### 3-й прогон (РЕШАЮЩИЙ, туннель ВКЛючён) — АКЦЕНТ СМЕЩАЕТСЯ: активен TPROXY, а туннель не несёт трафик
Юзер включил подписку `blanc` и снял данные при живом Xray. Вывод переворачивает
приоритет — мой session-13 фикс Proxy0 верен, но лечит НЕ тот режим, что сейчас активен.

**1) Устройство СЕЙЧАС в режиме TPROXY, НЕ proxy-connection.** Лог keen-manager при
активации ставит transparent-proxy capture (НЕ Proxy-интерфейс):
```
exec: /opt/sbin/iptables -t mangle -N KEENMGR_TPROXY
exec: /opt/sbin/iptables -t mangle -A KEENMGR_TPROXY -m mark --mark 255 -j RETURN
   ...(RETURN на все приватные подсети)...
exec: /opt/sbin/iptables -t mangle -A KEENMGR_TPROXY -p tcp -j TPROXY --on-port 12345 --tproxy-mark 0x2333/0x2333
exec: /opt/sbin/iptables -t mangle -A KEENMGR_TPROXY -p udp -j TPROXY --on-port 12345 --tproxy-mark 0x2333/0x2333
exec: /opt/sbin/iptables -t mangle -I PREROUTING -j KEENMGR_TPROXY
exec: /opt/sbin/ip rule add fwmark 0x2333 lookup 993
exec: /opt/sbin/ip route replace local default dev lo table 993
```
`Proxy0` при этом — брошенный огрызок: `show/interface/Proxy0` → `link:down,
connected:no, state:up, global:false, security-level:public` (summary.layer:
conf running, link pending, ctrl pending) ДАЖЕ когда Xray поднят и слушает
`127.0.0.1:10808`. Proxy-интерфейс не задействован — рулит TPROXY.

**2) ПОЧЕМУ TPROXY (гипотеза — проверить):** развёрнутый на устройстве билд (ДО
session-13) создаёт `Proxy0` с `security-level:"public"` СТРОКОЙ, а RCI её отбивает
(`no input`, ident Command::Root — подтверждено дважды). → `CreateProxyInterface`
возвращает ошибку → `bringUpXrayProxy` латчит `proxyClientDown` → **фолбэк в TPROXY**,
частичный create оставляет огрызок `Proxy0`. **session-13 фикс (объектная форма
`{"security-level":{"public":true}}`) ровно это чинит** → после пере-сборки proxy-mode
create должен пройти. Но это ОТДЕЛЬНО от проблемы #3.

**3) ГЛАВНАЯ НЕРЕШЁННАЯ ПРОБЛЕМА (P0): туннель НЕ несёт трафик (в любом режиме).**
`select-best` валит ВСЕХ кандидатов `blanc` на пост-активационной пробе:
```
activating 🇫🇮 Финляндия, Extra Whitelist2 (previous active: "")
exec: /opt/sbin/xray -test -config .../config.json.tmp -format json     ← ПРОХОДИТ
exec: /opt/etc/init.d/S99xray restart
   ...(ставит KEENMGR_TPROXY)...
post-activate probe failed for 🇸🇪 Sweden, Extra Whitelist (target=https://www.gstatic.com/generate_204: context deadline exceeded) — rolling back to ""
select-best: ... the tunnel did not carry traffic to https://www.gstatic.com/generate_204 (context deadline exceeded); rolled back — check the server is reachable and not DPI-blocked, or set a different probe target on the Failover page
```
Xray РАБОТАЕТ (`23761 /opt/sbin/xray run -confdir /opt/etc/keen-manager/xray`,
`tcp 127.0.0.1:10808 LISTEN`), `xray -test` проходит, но
`curl -x socks5h://127.0.0.1:10808 https://api.ipify.org` → ПУСТО (наружу молчит).
И в TPROXY при мёртвом туннеле + активном capture ВЕСЬ роутер/LAN теряет интернет:
сразу после активации `raw-ip=000 (8s), by-name=000 (8s)` — вот оно «включаю подписку
→ всё умирает». Когда Xray НЕ активен, capture снят → интернет ок (отсюда «падает
соединение → всё ок»).

**4) Наш RCI-hardening Proxy0 сработал и УДЕРЖАЛСЯ, но к текущему сбою не относится**
(сбой через TPROXY, не через Proxy0). Подтверждённые формы RCI (на устройстве):
- `ip global false` → `"Proxy0": global priority cleared` (потом в `show/interface` `global:false`).
- `ip name-servers false` → `ignore IPv4 name servers`; `ipv6 name-servers false` → `ignore IPv6 name servers`.
- `security-level` СТРОКОЙ → ERROR `no input`; нужна ОБЪЕКТНАЯ форма `{"security-level":{"public":true}}` (session-13 фикс уже так делает).
- `show/interface/Proxy0` даёт: `link/connected/state/global/security-level` + `summary.layer{conf,link,ipv4,ipv6,ctrl}` — полезный шейп статуса.
- keen-manager пишет лог в `/opt/var/log/keen-manager.log`. Xray-конфиг: `/opt/etc/keen-manager/xray/`, бинарь `/opt/sbin/xray`, init `/opt/etc/init.d/S99xray`.

**5) ПРИОРИТЕТЫ СЛЕДУЮЩЕМУ АГЕНТУ (юзер просил пока КОД НЕ править):**
- **P0 — почему туннель не несёт трафик** (реальный блокер, НЕ роутинг/хайджек). Снять
  СОБСТВЕННЫЕ логи Xray (не только keen-manager.log): куда пишет `S99xray`/xray-конфиг
  (`log` в `/opt/etc/keen-manager/xray/`), запустить `xray run` руками и смотреть stderr
  на живой хендшейк к vless. Проверить: (a) рвёт ли DPI reality/TLS к серверам blanc
  (наиболее вероятно на этом ISP — см. прошлые сессии, «Extra Whitelist/Whitelist2»);
  (b) доступность endpoint'ов (tcp-connect); (c) не блокирован ли `gstatic/generate_204`
  (сменить probe-target на Failover — но юзер: «вообще ничего», значит туннель реально
  мёртв, не только проба); (d) идёт ли собственный egress Xray в WAN при TPROXY
  (исключение по `mark 255`); (e) `0.0.0.0/0 gateway 0.0.0.0` в `show/ip/route` — свериться,
  что WAN-дефолт корректен.
- **P1 — режим:** `proxyClientDown` мог залатчиться в текущем процессе демона (из-за
  string-create) → рестарт сервиса (`/opt/etc/init.d/S99keen-manager restart`) сбросит
  латч. С object-form фиксом (после пере-сборки) proxy-create пройдёт → сравнить, несёт
  ли трафик proxy-mode (Proxy0 `connected:yes`?) vs TPROXY. Решить дефолт.
- **P2 — после пере-сборки на устройстве:** proxy-create без `no input`; `Proxy0
  connected:yes`; global остаётся clear при реконнекте (не пере-навешивает ли firmware);
  маршрут на Proxy0 форвардит выбранные домены. Тогда — тег stable/rc.

### 12-я сессия (ЗАКРЫТА) — CLI + качество failover + nfqws2-паритет + дашборд; rc.2
Только код-сайд полировка к стабильной (LAN роутера из облачной песочницы
недостижима — **весь on-device P0/P1 по-прежнему на юзере**, см. блоки 9-й/10-й
ниже). Всё в `main`, зелено: `go build/vet/test` + кросс mipsle/arm64 + веб-бандл
(tsc + vitest 17), CI-гард бандла проходит. Выпущен **`v0.1.0-rc.2`**. Коммиты:
- **`feat(cli)` 3d7cb2e — структурный nfqws в CLI** (закрыл последний пробел
  CLI-паритета из 11-й). `keen-manager nfqws config` печатает структурный
  nfqws2.conf; `nfqws set <field> <value>` правит одно типизированное поле
  (tcp/udp-порты, policy, nfqueue, log-level, ipv6, блоки стратегий args*) через
  ТОТ ЖЕ JSON-оверлей и lossless round-trip, что и веб-форма. Чистые
  `ParseConfField`/`ConfFieldHelp` в `internal/nfqws/fields.go`; юнит-тест +
  reflection-гард, что каждое поле маппится в реальный json-тег `Conf`.
- **`feat(engine)` 74b0fd6 — select-best проверяет кандидатов ЧЕРЕЗ туннель.**
  Раньше `fastest()` брал один самый быстрый по TCP и валил всё действие, если у
  него туннель не поднялся. Теперь `SelectBest` ранжирует достижимых по латентности
  и пробует по очереди, проверяя каждого сквозным probe через дедман
  verify-then-rollback `Activate`; первый, кто реально несёт трафик, побеждает →
  быстрый-но-мёртвый-по-DPI сервер больше не топит select-best. Ограничено
  (xray-core пред-ensure один раз, кап на per-candidate verify, ≤5 кандидатов);
  при полном провале откат на прежний active. Чистый `rankByLatency`, юнит-тест.
  (Цепочка failover это уже делала через `activateWithin`.)
- **`feat(engine)` 01877d7 — nfqws2-паритет: автодетект ISP + статус ndm-хука.**
  `DetectISPInterface` берёт WAN-аплинк из RCI-листинга интерфейсов (чистый
  `keenetic.PickWANInterface`: public, не-туннель, connected, priority) —
  авторитетно даже когда дефолт-роут занял VPN; `nfqws detect-isp` и
  `nfqws set isp-interface auto`. `HookStatus` (present/wired/binary-present) →
  CLI `route status` + `hook_installed` в `/api/state`. **ISP-интерфейс СВЕРИТЬ
  НА УСТРОЙСТВЕ.** Осталось: lua/log-файлы, self-update/version-check nfqws2.
- **`feat(api)` 9e76753 — capabilities + счётчики трафика.** `GET /api/health`
  несёт блок capabilities (native_awg2/wireguard/proxy_client/dns_route +
  firmware); новый `GET /api/traffic` — per-iface rx/tx из `/proc/net/dev`
  (чистый `parseProcNetDev`, юнит-тест).
- **`feat(web)` 7af8c28 — полировка дашборда.** Бар capabilities (бейджи Native
  AWG2 / WireGuard / Proxy client / DNS routing / Route hook) + живой
  Download/Upload throughput на WAN-карточке (диффит `/api/traffic`). EN+RU,
  бандл пересобран В ТОМ ЖЕ коммите (детерминирован, Node 24).

⚠️ **Ничего on-device не трогалось.** ISP-автодетект и ndm-хук построены защитно
(RCI-листинг подтверждён, фолбэки, dry-run-aware). Осталось on-device: сверить,
что выбранный ISP-интерфейс реально тот (и что хук ФИЗИЧЕСКИ срабатывает — сейчас
проверяем только наличие/проводку). Главный гейт стабильной `v0.1.0` — прежний
P0 read-back прокси-шейпа + флаг «выход в интернет» (блок 9-й сессии ниже).

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
+ CLI-паритет по failover — в 11-й; структурный nfqws-CLI, through-tunnel select-best,
nfqws2-паритет (ISP-автодетект + статус ndm-хука), capabilities/traffic API и полировка
дашборда — в 12-й. Выпущен `v0.1.0-rc.2`. **Единственный гейт для настоящего stable
`v0.1.0` — on-device валидация** (из облачной песочницы недостижимо; выполнить на роутере
и прислать вывод):
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

**Код-сайд (можно в песочнице, роутер НЕ нужен) — что ОСТАЛОСЬ (детали в ROADMAP P2/P3):**
nfqws2 parity — lua/log-файлы + self-update/version-check (ISP-автодетект и статус ndm-хука
сделаны в 12-й); дашборд — quick connection switcher (live-трафик + бейджи capabilities
сделаны в 12-й); hysteria2/tuic (модель + Xray-outbound). После того как on-device P0
закрыт и шейп `proxyInterfaceBody` подтверждён — тегнуть stable **`v0.1.0`** (без суффикса
= не prerelease). rc.2 забандлил весь код-сайд 12-й сессии как «latest».

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
