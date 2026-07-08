// Bypass (nfqws2) page strings. { en, ru } must stay key-for-key in sync.
export const bypass = {
  en: {
    title: 'Bypass',
    desc: 'nfqws2 DPI-bypass service, strategies and lists.',

    // Service control card
    versionUnknown: 'version unknown',
    notPresent: 'not present',
    notInstalled: 'Not installed',
    running: 'Running',
    stopped: 'Stopped',
    install: 'Install',
    start: 'Start',
    stop: 'Stop',
    restart: 'Restart',
    reload: 'Reload',
    starting: 'Starting nfqws2…',
    stopping: 'Stopping nfqws2…',
    restarting: 'Restarting nfqws2…',
    reloading: 'Reloading strategy…',
    installing: 'Installing nfqws2…',
    actionError: 'Service action failed',

    // Tabs
    tabConfig: 'Config',
    tabHostlists: 'Hostlists',
    tabCheck: 'Domain check',

    // Config editor
    configTitle: 'Strategy configuration',
    configHint: 'Raw nfqws2 arguments and desync mode.',
    modeLabel: 'Bypass mode',
    modeAuto: 'Auto (learned + user lists)',
    modeList: 'List (hostlists only)',
    modeAll: 'All traffic',
    rawConfigLabel: 'Raw configuration',
    configSaved: 'Configuration saved',
    configSavedDesc: 'Reload the service to apply changes.',
    configSaveError: 'Could not save config',

    // Hostlists manager
    emptyListsTitle: 'No hostlists',
    emptyListsDesc: 'nfqws2 has no managed hostlists on this device.',
    activeEntries: 'Active entries: {count}',
    hostlistSaved: 'Hostlist saved',
    hostlistSavedDesc: '{name} updated.',
    hostlistSaveError: 'Could not save hostlist',
    hostlistPlaceholder: '# one domain or CIDR per line',

    // Domain checker
    checkTitle: 'Domain reachability',
    checkHint:
      'Probe a hostname on both the direct path and through the nfqws2 desync engine.',
    domainPlaceholder: 'youtube.com',
    check: 'Check',
    checkError: 'Domain check failed',
    directPath: 'Direct path',
    throughBypass: 'Through nfqws2',
    reachableDirect: 'Reachable directly',
    blockedUnreachable: 'Blocked / unreachable',
    reachableViaBypass: 'Reachable via bypass',
    stillUnreachable: 'Still unreachable',
    note: 'Note.',
    checkEmptyHint: 'Enter a domain to run a reachability probe.',
  },
  ru: {
    title: 'Обход',
    desc: 'Сервис обхода DPI nfqws2, стратегии и списки.',

    // Service control card
    versionUnknown: 'версия неизвестна',
    notPresent: 'не установлен',
    notInstalled: 'Не установлен',
    running: 'Работает',
    stopped: 'Остановлен',
    install: 'Установить',
    start: 'Запустить',
    stop: 'Остановить',
    restart: 'Перезапустить',
    reload: 'Перечитать',
    starting: 'Запускаю nfqws2…',
    stopping: 'Останавливаю nfqws2…',
    restarting: 'Перезапускаю nfqws2…',
    reloading: 'Перечитываю стратегию…',
    installing: 'Устанавливаю nfqws2…',
    actionError: 'Действие с сервисом не удалось',

    // Tabs
    tabConfig: 'Конфигурация',
    tabHostlists: 'Списки хостов',
    tabCheck: 'Проверка домена',

    // Config editor
    configTitle: 'Конфигурация стратегии',
    configHint: 'Аргументы nfqws2 и режим десинхронизации.',
    modeLabel: 'Режим обхода',
    modeAuto: 'Авто (обученный + пользовательские списки)',
    modeList: 'Список (только списки хостов)',
    modeAll: 'Весь трафик',
    rawConfigLabel: 'Конфигурация (raw)',
    configSaved: 'Конфигурация сохранена',
    configSavedDesc: 'Перечитай сервис, чтобы применить изменения.',
    configSaveError: 'Не удалось сохранить конфигурацию',

    // Hostlists manager
    emptyListsTitle: 'Нет списков хостов',
    emptyListsDesc: 'На этом устройстве у nfqws2 нет управляемых списков хостов.',
    activeEntries: 'Активных записей: {count}',
    hostlistSaved: 'Список хостов сохранён',
    hostlistSavedDesc: '{name} обновлён.',
    hostlistSaveError: 'Не удалось сохранить список хостов',
    hostlistPlaceholder: '# один домен или CIDR на строку',

    // Domain checker
    checkTitle: 'Доступность домена',
    checkHint:
      'Проверить хост и по прямому пути, и через движок десинхронизации nfqws2.',
    domainPlaceholder: 'youtube.com',
    check: 'Проверить',
    checkError: 'Проверка домена не удалась',
    directPath: 'Прямой путь',
    throughBypass: 'Через nfqws2',
    reachableDirect: 'Доступен напрямую',
    blockedUnreachable: 'Заблокирован / недоступен',
    reachableViaBypass: 'Доступен через обход',
    stillUnreachable: 'По-прежнему недоступен',
    note: 'Примечание.',
    checkEmptyHint: 'Введи домен, чтобы проверить доступность.',
  },
}
