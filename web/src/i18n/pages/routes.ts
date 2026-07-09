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
    targetPlaceholder: 'Select a connection',
    targetHint:
      'Only connections with a native interface (AmneziaWG) can back a route. Xray connections carry traffic transparently — use a policy instead.',
    noTargets: 'No routable connections yet',
    noTargetsHint:
      'Add and activate an AmneziaWG connection first — it appears here as a native interface.',

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
      'Large lists are split into ≤100-domain groups automatically to fit Keenetic’s per-list limit.',

    // Toasts
    created: 'Route created',
    createdDesc: '“{name}” now routes through {target}.',
    createError: 'Could not create the route',
    toggled: 'Route updated',
    toggleError: 'Could not update the route',
    deleted: 'Route deleted',
    deleteError: 'Could not delete the route',
    selectTargetFirst: 'Choose a target connection first.',
  },
  ru: {
    title: 'Маршруты',
    desc: 'Направляйте выбранные сервисы через подключение штатной маршрутизацией роутера (object-group + dns-proxy).',

    tabCatalog: 'Каталог сервисов',
    tabActive: 'Активные маршруты',
    tabCustom: 'Свои и импорт',

    targetLabel: 'Направить через',
    targetPlaceholder: 'Выберите подключение',
    targetHint:
      'Маршрут может опираться только на подключение с нативным интерфейсом (AmneziaWG). Xray ведёт трафик прозрачно — для него используйте политику.',
    noTargets: 'Пока нет маршрутизируемых подключений',
    noTargetsHint:
      'Сначала добавьте и активируйте AmneziaWG-подключение — оно появится здесь как нативный интерфейс.',

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
      'Большие списки автоматически делятся на группы ≤100 доменов под лимит одного списка Keenetic.',

    created: 'Маршрут создан',
    createdDesc: '«{name}» теперь идёт через {target}.',
    createError: 'Не удалось создать маршрут',
    toggled: 'Маршрут обновлён',
    toggleError: 'Не удалось обновить маршрут',
    deleted: 'Маршрут удалён',
    deleteError: 'Не удалось удалить маршрут',
    selectTargetFirst: 'Сначала выберите целевое подключение.',
  },
} as const
