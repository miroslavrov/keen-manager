// i18n fragment for the Routes / "Маршруты" page. Exposes { en, ru } spread into
// the top-level dictionaries under the `routes` key.

export const routes = {
  en: {
    title: 'Routes',
    desc: 'Send chosen services through a connection using the router’s native domain routing (object-group + dns-proxy).',

    // Tabs
    tabCatalog: 'Service catalog',
    tabActive: 'Active routes',
    tabCustom: 'Custom & import',

    // Target picker
    targetLabel: 'Route through',
    targetPlaceholder: 'Select a target',
    targetHint:
      'Route through a keen-manager AmneziaWG connection, a WireGuard interface on the router (pulled live from KeeneticOS), or an Xray connection. For Xray the selected services go through the tunnel and everything else stays direct — keen-manager is the single exit point and does the split under the hood.',
    groupConnections: 'keen-manager connections',
    groupInterfaces: 'Router interfaces',
    groupXray: 'Xray tunnels',
    groupSubscriptions: 'Subscriptions (all servers)',
    xrayTargetHint: 'selected services via Xray, the rest direct',
    groupBypass: 'DPI bypass',
    bypassTarget: 'DPI Bypass',
    bypassTargetHint: 'selected domains via nfqws/tpws desync',
    ifaceDown: 'down',
    dnsUnavailable:
      'Native DNS routing isn’t available on this firmware, so routes can’t be applied — it needs KeeneticOS 5.x with AWG2 support.',
    noTargets: 'No routable targets yet',
    noTargetsHint:
      'Activate an AmneziaWG connection, or create a WireGuard interface on the router — it shows up here, pulled live from KeeneticOS.',

    // Catalog
    searchPlaceholder: 'Search services…',
    selected: '{count} selected',
    selectedNone: 'Select services to route',
    domains: '{count} domains',
    subnets: '{count} subnets',
    remoteList: 'remote list',
    createSelected: 'Create route',
    createSelectedN: 'Route {count} services',
    clearSelection: 'Clear',
    noResults: 'No services match “{query}”.',

    // Categories
    catSocial: 'Social',
    catMedia: 'Media & streaming',
    catAi: 'AI',
    catGaming: 'Gaming',
    catDeveloper: 'Developer',
    catCloud: 'Cloud & CDN',
    catBlock: 'Blocked lists',
    catCustom: 'Custom',

    // Active routes
    activeTitle: 'Active routes',
    emptyActiveTitle: 'No routes yet',
    emptyActiveDesc:
      'Pick services from the catalog and a target connection to start routing them through the tunnel.',
    routeVia: 'via',
    applied: 'Applied',
    pending: 'Pending',
    notApplied: 'Not applied',
    deleteRoute: 'Delete route',
    deleteTitle: 'Delete route?',
    deleteDesc: 'Remove “{name}” and its router-side domain groups.',

    // Custom / import
    customTitle: 'Custom route',
    customName: 'Route name',
    customNamePlaceholder: 'e.g. My services',
    customDomains: 'Domains',
    customDomainsPlaceholder: 'one domain per line\nexample.com\ncdn.example.com',
    customSubnets: 'Subnets (CIDR)',
    customSubnetsPlaceholder: 'one CIDR per line\n104.16.0.0/13',
    createCustom: 'Create custom route',
    importTitle: 'Import from a list URL',
    importDesc:
      'Paste a v2fly domain-list, a plain domain list, or a hosts/AdBlock URL. include: directives and @attribute tags are expanded automatically.',
    importUrlPlaceholder:
      'https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/youtube',
    importAttr: 'Attribute filter (optional)',
    importAttrPlaceholder: 'e.g. cn, ads — leave empty for all',
    importBtn: 'Fetch list',
    importing: 'Fetching…',
    imported: 'Fetched {count} domains from {sources} source(s)',
    importedSkipped: '{count} entries skipped (keyword/regexp)',
    importTruncated: 'List truncated at the safety cap.',
    importError: 'Could not fetch or parse that list URL.',
    importEmpty: 'No usable domains found in that list.',
    appendToDomains: 'Added to the domains field below.',

    // Keenetic limit hint
    chunkHint:
      'Large lists are split into ≤300-domain groups automatically to fit Keenetic’s per-list limit.',

    // Toasts
    created: 'Route created',
    createdDesc: '“{name}” now routes through {target}.',
    createError: 'Could not create the route',
    toggled: 'Route updated',
    toggleError: 'Could not update the route',
    deleted: 'Route deleted',
    deleteError: 'Could not delete the route',
    selectTargetFirst: 'Choose a target connection first.',

    // Editor (open a rule to edit its domains/subnets)
    editRoute: 'Edit route',
    editHint: 'Tap a rule to edit its domains',
    editTitle: 'Edit “{name}”',
    editDesc:
      'Change which domains and subnets use this route. Saving re-applies it on the router.',
    editDomainsLabel: 'Domains',
    editSubnetsLabel: 'Subnets (CIDR)',
    editSave: 'Save changes',
    editSaved: 'Route updated',
    editError: 'Could not update the route',
    editEmpty: 'A route needs at least one domain or subnet.',
    editLoading: 'Loading domains…',
    editLoadError: 'Could not load this route.',
  },
  ru: {
    title: 'Маршруты',
    desc: 'Направляйте выбранные сервисы через подключение штатной маршрутизацией роутера (object-group + dns-proxy).',

    tabCatalog: 'Каталог сервисов',
    tabActive: 'Активные маршруты',
    tabCustom: 'Свои и импорт',

    targetLabel: 'Направить через',
    targetPlaceholder: 'Выберите цель',
    targetHint:
      'Маршрут через AmneziaWG-подключение keen-manager, WireGuard-интерфейс роутера (тянется вживую из KeeneticOS) или Xray-подключение. Для Xray выбранные сервисы идут через туннель, остальное — напрямую: keen-manager остаётся единой точкой выхода и делает разделение под капотом.',
    groupConnections: 'Подключения keen-manager',
    groupInterfaces: 'Интерфейсы роутера',
    groupXray: 'Xray-туннели',
    groupSubscriptions: 'Подписки (все серверы)',
    xrayTargetHint: 'выбранные сервисы через Xray, остальное напрямую',
    groupBypass: 'Обход DPI',
    bypassTarget: 'Обход DPI',
    bypassTargetHint: 'выбранные домены через десинхронизацию nfqws/tpws',
    ifaceDown: 'выкл',
    dnsUnavailable:
      'На этой прошивке штатная DNS-маршрутизация недоступна, применить маршруты нельзя — нужен KeeneticOS 5.x с поддержкой AWG2.',
    noTargets: 'Пока нет целей для маршрута',
    noTargetsHint:
      'Активируйте AmneziaWG-подключение или создайте WireGuard-интерфейс на роутере — он появится здесь, вживую из KeeneticOS.',

    searchPlaceholder: 'Поиск сервисов…',
    selected: 'Выбрано: {count}',
    selectedNone: 'Выберите сервисы для маршрутизации',
    domains: 'доменов: {count}',
    subnets: 'подсетей: {count}',
    remoteList: 'удалённый список',
    createSelected: 'Создать маршрут',
    createSelectedN: 'Маршрут для {count} сервисов',
    clearSelection: 'Сбросить',
    noResults: 'Нет сервисов по запросу «{query}».',

    catSocial: 'Соцсети',
    catMedia: 'Медиа и стриминг',
    catAi: 'ИИ',
    catGaming: 'Игры',
    catDeveloper: 'Разработка',
    catCloud: 'Облака и CDN',
    catBlock: 'Списки блокировок',
    catCustom: 'Свои',

    activeTitle: 'Активные маршруты',
    emptyActiveTitle: 'Маршрутов пока нет',
    emptyActiveDesc:
      'Выберите сервисы из каталога и целевое подключение, чтобы направить их через туннель.',
    routeVia: 'через',
    applied: 'Применён',
    pending: 'Ожидает',
    notApplied: 'Не применён',
    deleteRoute: 'Удалить маршрут',
    deleteTitle: 'Удалить маршрут?',
    deleteDesc: 'Удалить «{name}» и его доменные группы на роутере.',

    customTitle: 'Свой маршрут',
    customName: 'Название маршрута',
    customNamePlaceholder: 'напр. Мои сервисы',
    customDomains: 'Домены',
    customDomainsPlaceholder: 'по одному домену на строку\nexample.com\ncdn.example.com',
    customSubnets: 'Подсети (CIDR)',
    customSubnetsPlaceholder: 'по одному CIDR на строку\n104.16.0.0/13',
    createCustom: 'Создать свой маршрут',
    importTitle: 'Импорт из URL списка',
    importDesc:
      'Вставьте v2fly domain-list, обычный список доменов или hosts/AdBlock URL. Директивы include: и теги @attribute разворачиваются автоматически.',
    importUrlPlaceholder:
      'https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/youtube',
    importAttr: 'Фильтр по атрибуту (необязательно)',
    importAttrPlaceholder: 'напр. cn, ads — пусто = все',
    importBtn: 'Загрузить список',
    importing: 'Загрузка…',
    imported: 'Загружено {count} доменов из {sources} источник(ов)',
    importedSkipped: 'Пропущено записей: {count} (keyword/regexp)',
    importTruncated: 'Список обрезан по защитному лимиту.',
    importError: 'Не удалось загрузить или разобрать этот URL списка.',
    importEmpty: 'В списке не найдено пригодных доменов.',
    appendToDomains: 'Добавлено в поле доменов ниже.',

    chunkHint:
      'Большие списки автоматически делятся на группы ≤300 доменов под лимит одного списка Keenetic.',

    created: 'Маршрут создан',
    createdDesc: '«{name}» теперь идёт через {target}.',
    createError: 'Не удалось создать маршрут',
    toggled: 'Маршрут обновлён',
    toggleError: 'Не удалось обновить маршрут',
    deleted: 'Маршрут удалён',
    deleteError: 'Не удалось удалить маршрут',
    selectTargetFirst: 'Сначала выберите целевое подключение.',

    // Редактор (открыть правило и изменить домены/подсети)
    editRoute: 'Изменить маршрут',
    editHint: 'Нажмите на правило, чтобы изменить домены',
    editTitle: 'Изменить «{name}»',
    editDesc:
      'Измените, какие домены и подсети идут через этот маршрут. Сохранение переприменит его на роутере.',
    editDomainsLabel: 'Домены',
    editSubnetsLabel: 'Подсети (CIDR)',
    editSave: 'Сохранить',
    editSaved: 'Маршрут обновлён',
    editError: 'Не удалось обновить маршрут',
    editEmpty: 'Нужен хотя бы один домен или подсеть.',
    editLoading: 'Загрузка доменов…',
    editLoadError: 'Не удалось загрузить маршрут.',
  },
} as const
