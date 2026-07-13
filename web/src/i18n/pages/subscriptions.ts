// Subscriptions page strings. { en, ru } must stay key-for-key in sync.
export const subscriptions = {
  en: {
    title: 'Subscriptions',
    desc: 'Import servers from a subscription URL, like a native client.',

    addSubscription: 'Add subscription',

    emptyTitle: 'No subscriptions',
    emptyDesc:
      'Add a subscription URL (e.g. https://host/s/<token>) to import a fleet of Xray servers.',

    refreshedTitle: 'Subscription refreshed',
    refreshErrorTitle: 'Could not refresh subscription',
    selectedBestTitle: 'Selected best server',
    selectBestErrorTitle: 'Could not select best server',
    removedTitle: 'Subscription removed',
    removedDesc: '“{name}” was deleted.',
    removeErrorTitle: 'Could not delete subscription',
    autoBestErrorTitle: 'Could not update auto-select best',
    autoBestEnabledTitle: 'Auto-select best enabled',
    autoBestDisabledTitle: 'Auto-select best disabled',
    streamEnabledTitle: 'Subscription stream on',
    streamDisabledTitle: 'Subscription stream off',
    streamErrorTitle: 'Could not switch subscription stream',
    stream: 'Stream',
    streamAria: 'Toggle this subscription stream on or off',
    streamTitle:
      'On/off for this whole subscription — the middle of three egress switches (connector → subscription → server).',
    pausedBadge: 'Paused',
    selectBestDisabledHint: 'Turn the subscription stream on to select a server.',
    createdTitle: 'Subscription added',
    createdDesc: '“{name}” will import its servers shortly.',
    createErrorTitle: 'Could not add subscription',

    deleteTitle: 'Delete subscription?',
    deleteDesc: '“{name}” and its imported servers will be removed.',

    autoSelectAria: 'Auto-select best server',
    autoBest: 'Auto-best',
    autoBestTitle: 'Auto-select best',
    autoBestHint:
      'Keeps you on the fastest ENABLED server in this subscription. Switch off servers you don’t want below — auto-best never picks them.',
    autoBestPool: '{enabled} of {total} in pool',
    autoBestPoolNone: 'no servers enabled',
    autoBestActiveNow: 'Applies now — you’re on a server from this subscription.',
    autoBestIdleNow: 'Applies once a server from this subscription is active.',
    selectBest: 'Select best now',
    selectBestOnceHint: 'One-off jump to the fastest enabled server (auto-best keeps it there).',
    poolCaption:
      'Auto-best and “Select best” only choose among enabled servers. A disabled server stays out of the pool and keeps its state across subscription refreshes.',
    serverExcluded: 'Excluded',
    serverEnableAria: 'Include or exclude this server from the auto-best pool',
    serverToggleErrorTitle: 'Could not update server',
    serverIncludedTitle: 'Server added to the pool',
    serverExcludedTitle: 'Server excluded from the pool',
    deleteAria: 'Delete subscription',
    copyAddress: 'Copy address',

    metaServers: 'Servers',
    metaProtocol: 'Protocol',
    metaUpdateInterval: 'Update interval',
    metaLastUpdate: 'Last update',

    dataUsage: 'Data usage',
    usedPct: '{pct}% used',
    expires: 'Expires {date}',

    showServers: 'Show servers ({count})',
    hideServers: 'Hide servers ({count})',

    noServersTitle: 'No servers',
    noServersDesc:
      "This subscription hasn't imported any servers yet. Try refreshing.",

    addDialogTitle: 'Add subscription',
    addDialogDesc:
      'Import an Xray subscription feed. The server list refreshes on the configured interval.',
    nameLabel: 'Name',
    namePlaceholder: 'e.g. OceanLink Premium',
    urlLabel: 'Subscription URL',
    urlPlaceholder: 'https://host/s/<token>',
  },
  ru: {
    title: 'Подписки',
    desc: 'Импорт серверов по ссылке подписки, как в нативном клиенте.',

    addSubscription: 'Добавить подписку',

    emptyTitle: 'Нет подписок',
    emptyDesc:
      'Добавь ссылку подписки (например, https://host/s/<token>), чтобы импортировать серверы Xray.',

    refreshedTitle: 'Подписка обновлена',
    refreshErrorTitle: 'Не удалось обновить подписку',
    selectedBestTitle: 'Выбран лучший сервер',
    selectBestErrorTitle: 'Не удалось выбрать лучший сервер',
    removedTitle: 'Подписка удалена',
    removedDesc: '«{name}» удалена.',
    removeErrorTitle: 'Не удалось удалить подписку',
    autoBestErrorTitle: 'Не удалось изменить автовыбор лучшего',
    autoBestEnabledTitle: 'Автовыбор лучшего включён',
    autoBestDisabledTitle: 'Автовыбор лучшего выключен',
    streamEnabledTitle: 'Поток подписки включён',
    streamDisabledTitle: 'Поток подписки выключен',
    streamErrorTitle: 'Не удалось переключить поток подписки',
    stream: 'Поток',
    streamAria: 'Включить или выключить поток этой подписки',
    streamTitle:
      'Вкл/выкл всей подписки — средний из трёх переключателей выхода (коннектор → подписка → сервер).',
    pausedBadge: 'На паузе',
    selectBestDisabledHint: 'Включи поток подписки, чтобы выбрать сервер.',
    createdTitle: 'Подписка добавлена',
    createdDesc: '«{name}» скоро импортирует свои серверы.',
    createErrorTitle: 'Не удалось добавить подписку',

    deleteTitle: 'Удалить подписку?',
    deleteDesc: '«{name}» и её импортированные серверы будут удалены.',

    autoSelectAria: 'Автовыбор лучшего сервера',
    autoBest: 'Автовыбор',
    autoBestTitle: 'Автовыбор лучшего',
    autoBestHint:
      'Держит на самом быстром из ВКЛючённых серверов подписки. Выключи ненужные ниже — автовыбор их не тронет.',
    autoBestPool: '{enabled} из {total} в пуле',
    autoBestPoolNone: 'нет включённых серверов',
    autoBestActiveNow: 'Действует сейчас — активен сервер из этой подписки.',
    autoBestIdleNow: 'Начнёт действовать, когда активным станет сервер этой подписки.',
    selectBest: 'Выбрать лучший',
    selectBestOnceHint: 'Разовый переход на самый быстрый из включённых (автовыбор потом держит его).',
    poolCaption:
      'Автовыбор и «Выбрать лучший» берут только включённые серверы. Выключенный сервер остаётся вне пула и сохраняет состояние при обновлении подписки.',
    serverExcluded: 'Исключён',
    serverEnableAria: 'Включить или исключить сервер из пула автовыбора',
    serverToggleErrorTitle: 'Не удалось изменить сервер',
    serverIncludedTitle: 'Сервер добавлен в пул',
    serverExcludedTitle: 'Сервер исключён из пула',
    deleteAria: 'Удалить подписку',
    copyAddress: 'Скопировать адрес',

    metaServers: 'Серверов',
    metaProtocol: 'Протокол',
    metaUpdateInterval: 'Интервал обновления',
    metaLastUpdate: 'Последнее обновление',

    dataUsage: 'Использование трафика',
    usedPct: 'Использовано: {pct}%',
    expires: 'Истекает {date}',

    showServers: 'Показать серверы ({count})',
    hideServers: 'Скрыть серверы ({count})',

    noServersTitle: 'Нет серверов',
    noServersDesc:
      'Эта подписка ещё не импортировала серверы. Попробуй обновить.',

    addDialogTitle: 'Добавить подписку',
    addDialogDesc:
      'Импорт ленты подписки Xray. Список серверов обновляется по заданному интервалу.',
    nameLabel: 'Название',
    namePlaceholder: 'например, OceanLink Premium',
    urlLabel: 'Ссылка подписки',
    urlPlaceholder: 'https://host/s/<token>',
  },
}
