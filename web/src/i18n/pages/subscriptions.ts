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
    createdTitle: 'Subscription added',
    createdDesc: '“{name}” will import its servers shortly.',
    createErrorTitle: 'Could not add subscription',

    deleteTitle: 'Delete subscription?',
    deleteDesc: '“{name}” and its imported servers will be removed.',

    autoSelectAria: 'Auto-select best server',
    autoBest: 'Auto-best',
    selectBest: 'Select best',
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
    createdTitle: 'Подписка добавлена',
    createdDesc: '«{name}» скоро импортирует свои серверы.',
    createErrorTitle: 'Не удалось добавить подписку',

    deleteTitle: 'Удалить подписку?',
    deleteDesc: '«{name}» и её импортированные серверы будут удалены.',

    autoSelectAria: 'Автовыбор лучшего сервера',
    autoBest: 'Автовыбор',
    selectBest: 'Выбрать лучший',
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
