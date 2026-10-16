import { derived, writable } from 'svelte/store';

// Minimal frontend i18n: EN is the default, the choice persists in localStorage.
const STRINGS = {
    en: {
        'btn.scan': 'Scan plugins',
        'btn.refresh': 'Refresh catalog',
        'refresh.state.cache': 'catalog from cache, updating...',
        'refresh.state.fresh': 'catalog up to date',
        'refresh.state.error': 'catalog refresh failed',
        'count.plugins.one': '{n} plugin',
        'count.plugins.other': '{n} plugins',
        'col.type': 'Type',
        'col.plugin': 'Plugin',
        'col.installed': 'Installed',
        'col.latest': 'In catalog',
        'col.status': 'Status',
        'col.actions': 'Actions',
        'table.scanning': 'Scanning wincmd.ini...',
        'table.none': 'No plugins found in wincmd.ini',
        'table.noresults': 'No plugins match the search',
        'search.ph': 'Search plugins...',
        'badge.up': 'Up to date',
        'badge.avail': 'Update!',
        'badge.missing': 'Missing',
        'badge.unknown': 'Unknown',
        'scan.error_friendly': "Couldn't scan plugins.",
        'btn.update': 'Update',
        'editor.title': 'Plugin version',
        'editor.version': 'Installed version',
        'editor.version.ph': 'e.g. 1.8.6.3',
        'editor.source': 'Update source',
        'editor.source.ph': 'e.g. https://github.com/Owner/Repo',
        'card.path': 'Path',
        'card.source': 'Source',
        'card.site': 'Site',
        'card.details': 'Details',
        'card.source.catalog': 'Catalog',
        'editor.hint': 'Defaults come from auto-detection. Empty fields reset the manual override.',
        'editor.save': 'Save',
        'editor.cancel': 'Cancel',
        'editor.save_friendly': 'Could not save the override.',
        'update.confirm': 'Update {name} to version {version}?',
        'update.locked': 'Files of {name} are locked. Close Total Commander and retry the update.',
        'update.done_backup': '{name} updated! Backup: {path}',
        'update.done': '{name} updated successfully!',
        'update.error_friendly':
            "Couldn't install the update. The plugin page may be outdated or the file is temporarily unavailable. Try again later or install manually from the plugin page.",
        'update.progress.title': 'Updating {name}',
        'update.progress.download': 'Downloading',
        'update.progress.extract': 'Extracting archive',
        'update.progress.install': 'Installing files',
        'update.progress.source': 'from',
        'update_all.btn': 'Update all',
        'update_all.count.one': '{n} plugin update is available',
        'update_all.count.other': '{n} plugin updates are available',
        'update_all.confirm':
            'Update {count} one after another. Each plugin is backed up before it changes. Plugins whose installed version is unknown stay untouched.',
        'update_all.title': 'Updating plugins',
        'update_all.item': '{current} of {total} — {name}',
        'update_all.updated.one': '{n} plugin updated',
        'update_all.updated.other': '{n} plugins updated',
        'update_all.none': 'No plugin was updated.',
        'update_all.locked_stop':
            'Total Commander is holding {name}, so the rest of the queue was skipped. Close it and press «Update all» again.',
        'btn.history': 'History',
        'history.title': 'Applied updates',
        'history.col.plugin': 'Plugin',
        'history.col.version': 'Version',
        'history.col.date': 'Applied at',
        'history.empty': 'No updates applied yet.',
        'history.error_friendly': 'Failed to load the update history.',
        'history.col.action': 'Action',
        'history.rollback': 'Roll back',
        'history.restored': 'rolled back',
        'history.confirm': 'Put {version} back instead of {current}?',
        'history.confirm_unknown': 'Put the kept copy back?',
        'history.confirm_note':
            'The version in the folder now is kept as a copy, so updating the plugin again brings it back.',
        'history.locked': 'Files are locked. Close Total Commander and roll back again.',
        'dialog.ok': 'OK',
        'dialog.cancel': 'Cancel',
        'dialog.close': 'Close',
        'dialog.plugin_page': 'Open plugin page',
        'dialog.tech_details': 'Technical details',
        'btn.settings': 'Settings',
        'settings.title': 'Settings',
        'settings.back': '‹ Back',
        'settings.save_hint': 'All changes save themselves',
        'settings.nav.appearance': 'Appearance',
        'settings.nav.language': 'Language',
        'settings.nav.about': 'About',
        'settings.appearance': 'Theme and font size',
        'settings.theme': 'Theme',
        'settings.theme.light': 'Light',
        'settings.theme.light.desc': 'Default',
        'settings.theme.dark': 'Dark',
        'settings.theme.dark.desc': 'Easy on the eyes',
        'settings.theme.system': 'Follow system',
        'settings.theme.system.desc': 'Switches automatically',
        'settings.font': 'Font size',
        'settings.nav.tc': 'Total Commander',
        'settings.tc': 'Where the installed plugins are read from.',
        'settings.tc.ini': 'wincmd.ini',
        'settings.tc.browse': 'Browse…',
        'settings.tc.browse_title': 'Choose your wincmd.ini',
        'settings.tc.hint': 'A typed path saves when you leave the field; an empty one means auto-detection.',
        'settings.tc.mode.auto': 'Found automatically',
        'settings.tc.mode.manual': 'Chosen manually',
        'settings.tc.none': 'Total Commander was not found — pick the file yourself.',
        'settings.tc.gone': 'This file no longer opens, so a scan would fail.',
        'settings.tc.back_auto': 'Use auto-detection',
        'settings.tc.not_saved': 'the change was not saved',
        'settings.nav.backups': 'Backups',
        'settings.backups': 'What an update leaves behind, and for how long.',
        'settings.backups.keep': 'Newest copies always kept',
        'settings.backups.days': 'Delete copies older than, days',
        'settings.backups.hint':
            'A copy is deleted only once it is past both numbers, so the newest ones survive however old they get. 0 days deletes nothing.',
        'settings.backups.location': 'Copies are kept in',
        'settings.backups.not_saved': 'the change was not saved',
        'settings.language': 'Interface language',
        'about.desc':
            'TotalPlug keeps your Total Commander plugins up to date: it finds what is installed, compares it with the community catalog and updates safely, with a backup and rollback behind every change.',
        'about.catalog': 'Plugin catalog',
        'about.releases': 'Releases',
        'about.check': 'Check for updates',
        'about.checking': 'Checking…',
        'about.up_to_date': 'You are on the latest release.',
        'about.available': 'TotalPlug {latest} is available.',
        'about.open_release': 'Open the release page',
        'about.checked_at': 'Checked at {time}',
        'about.dev_build': 'Development build: there is no released version to compare it with.',
        'about.check.error': 'Could not reach GitHub to check for updates.',
        'about.privacy':
            'TotalPlug only contacts the plugin catalog and GitHub, and only to look up updates. Nothing about you leaves this machine.',
        'about.details': 'Details',
        'about.data_dir': 'Settings and history',
        'about.build': 'Built with',
        'win.min': 'Minimize',
        'win.max': 'Maximize',
        'win.close': 'Close',
    },
    ru: {
        'btn.scan': 'Сканировать плагины',
        'btn.refresh': 'Обновить каталог',
        'refresh.state.cache': 'каталог из кэша, обновление...',
        'refresh.state.fresh': 'каталог актуален',
        'refresh.state.error': 'не удалось обновить каталог',
        'count.plugins.one': '{n} плагин',
        'count.plugins.few': '{n} плагина',
        'count.plugins.many': '{n} плагинов',
        'count.plugins.other': '{n} плагинов',
        'col.type': 'Тип',
        'col.plugin': 'Имя плагина',
        'col.installed': 'Установлена',
        'col.latest': 'В каталоге',
        'col.status': 'Статус',
        'col.actions': 'Действие',
        'table.scanning': 'Сканирую wincmd.ini...',
        'table.none': 'Плагины не найдены в wincmd.ini',
        'table.noresults': 'Ничего не найдено по запросу',
        'search.ph': 'Поиск плагинов...',
        'badge.up': 'Актуален',
        'badge.avail': 'Обновление!',
        'badge.missing': 'Отсутствует',
        'badge.unknown': 'Неизвестно',
        'scan.error_friendly': 'Не удалось просканировать плагины.',
        'btn.update': 'Обновить',
        'editor.title': 'Версия плагина',
        'editor.version': 'Установленная версия',
        'editor.version.ph': 'например, 1.8.6.3',
        'editor.source': 'Источник обновлений',
        'editor.source.ph': 'например https://github.com/Owner/Repo',
        'card.path': 'Путь',
        'card.source': 'Источник',
        'card.site': 'Сайт',
        'card.details': 'Подробности',
        'card.source.catalog': 'Каталог',
        'editor.hint':
            'Поля по умолчанию берутся из авто-определения. Пустые значения сбрасывают ручное переопределение.',
        'editor.save': 'Сохранить',
        'editor.cancel': 'Отмена',
        'editor.save_friendly': 'Не удалось сохранить переопределение.',
        'update.confirm': 'Обновить плагин {name} до версии {version}?',
        'update.locked': 'Файлы плагина {name} заняты. Закройте Total Commander и повторите обновление.',
        'update.done_backup': 'Плагин {name} обновлён! Резервная копия: {path}',
        'update.done': 'Плагин {name} успешно обновлён!',
        'update.error_friendly':
            'Не удалось установить обновление. Возможно, страница плагина устарела или файл временно недоступен. Попробуйте позже или установите вручную со страницы плагина.',
        'update.progress.title': 'Обновление {name}',
        'update.progress.download': 'Загрузка',
        'update.progress.extract': 'Распаковка архива',
        'update.progress.install': 'Установка файлов',
        'update.progress.source': 'источник',
        'update_all.btn': 'Обновить все',
        'update_all.count.one': 'Доступно {n} обновление',
        'update_all.count.few': 'Доступно {n} обновления',
        'update_all.count.many': 'Доступно {n} обновлений',
        'update_all.count.other': 'Доступно {n} обновлений',
        'update_all.confirm':
            'Обновить по очереди: {count}. Перед изменением каждого плагина будет создана резервная копия. Плагины с неизвестной установленной версией останутся нетронутыми.',
        'update_all.title': 'Обновление плагинов',
        'update_all.item': '{current} из {total} — {name}',
        'update_all.updated.one': '{n} плагин обновлён',
        'update_all.updated.few': '{n} плагина обновлено',
        'update_all.updated.many': '{n} плагинов обновлено',
        'update_all.updated.other': '{n} плагинов обновлено',
        'update_all.none': 'Не обновлено ни одного плагина.',
        'update_all.locked_stop':
            'Total Commander держит файлы плагина {name}, поэтому остальная очередь пропущена. Закройте его и нажмите «Обновить все» снова.',
        'btn.history': 'История',
        'history.title': 'Применённые обновления',
        'history.col.plugin': 'Плагин',
        'history.col.version': 'Версия',
        'history.col.date': 'Применено',
        'history.empty': 'Обновлений ещё не было.',
        'history.error_friendly': 'Не удалось загрузить историю обновлений.',
        'history.col.action': 'Действие',
        'history.rollback': 'Откатить',
        'history.restored': 'откат',
        'history.confirm': 'Вернуть {version} вместо {current}?',
        'history.confirm_unknown': 'Вернуть сохранённую копию?',
        'history.confirm_note': 'Установленная сейчас версия сохранится копией: повторное обновление вернёт её.',
        'history.locked': 'Файлы заняты. Закройте Total Commander и повторите откат.',
        'dialog.ok': 'ОК',
        'dialog.cancel': 'Отмена',
        'dialog.close': 'Закрыть',
        'dialog.plugin_page': 'Открыть страницу плагина',
        'dialog.tech_details': 'Технические детали',
        'btn.settings': 'Настройки',
        'settings.title': 'Настройки',
        'settings.back': '‹ Назад',
        'settings.save_hint': 'Все изменения сохраняются сами',
        'settings.nav.appearance': 'Оформление',
        'settings.nav.language': 'Язык',
        'settings.nav.about': 'О программе',
        'settings.appearance': 'Тема и размер шрифта',
        'settings.theme': 'Тема',
        'settings.theme.light': 'Светлая',
        'settings.theme.light.desc': 'По умолчанию',
        'settings.theme.dark': 'Тёмная',
        'settings.theme.dark.desc': 'Бережёт глаза',
        'settings.theme.system': 'Как в системе',
        'settings.theme.system.desc': 'Переключается сама',
        'settings.font': 'Размер шрифта',
        'settings.nav.tc': 'Total Commander',
        'settings.tc': 'Откуда читать установленные плагины.',
        'settings.tc.ini': 'файл wincmd.ini',
        'settings.tc.browse': 'Обзор…',
        'settings.tc.browse_title': 'Укажите ваш wincmd.ini',
        'settings.tc.hint': 'Путь сохраняется, когда вы уходите из поля; пустое поле — автопоиск.',
        'settings.tc.mode.auto': 'Найден автоматически',
        'settings.tc.mode.manual': 'Выбран вручную',
        'settings.tc.none': 'Total Commander не найден — укажите файл сами.',
        'settings.tc.gone': 'Файл больше не открывается, поэтому сканирование не удастся.',
        'settings.tc.back_auto': 'Вернуть автопоиск',
        'settings.tc.not_saved': 'изменение не сохранено',
        'settings.nav.backups': 'Резервные копии',
        'settings.backups': 'Что остаётся после обновления и сколько это хранится.',
        'settings.backups.keep': 'Сколько последних копий всегда хранить',
        'settings.backups.days': 'Удалять копии старше, дней',
        'settings.backups.hint':
            'Копия удаляется, только когда не проходит оба условия: самые новые остаются независимо от возраста. 0 дней — не удалять ничего.',
        'settings.backups.location': 'Копии хранятся в',
        'settings.backups.not_saved': 'изменение не сохранено',
        'settings.language': 'Язык интерфейса',
        'about.desc':
            'TotalPlug держит плагины Total Commander в актуальном состоянии: находит установленные, сверяет их с каталогом сообщества и обновляет безопасно — с резервной копией и откатом.',
        'about.catalog': 'Каталог плагинов',
        'about.releases': 'Релизы',
        'about.check': 'Проверить обновления',
        'about.checking': 'Проверяю…',
        'about.up_to_date': 'У вас самый свежий релиз.',
        'about.available': 'Доступен TotalPlug {latest}.',
        'about.open_release': 'Открыть страницу релиза',
        'about.checked_at': 'Проверено в {time}',
        'about.dev_build': 'Разработочная сборка: сравнивать её не с чем, релиза с таким номером нет.',
        'about.check.error': 'Не удалось заглянуть на GitHub и проверить обновления.',
        'about.privacy':
            'TotalPlug обращается только к каталогу плагинов и к GitHub — и только чтобы проверить обновления. Ничего о вас с этой машины не уходит.',
        'about.details': 'Подробности',
        'about.data_dir': 'Настройки и история',
        'about.build': 'Собрано',
        'win.min': 'Свернуть',
        'win.max': 'Развернуть',
        'win.close': 'Закрыть',
    },
};

const LANG_KEY = 'totalplug.lang';

const savedLang = localStorage.getItem(LANG_KEY);

// Svelte screens subscribe to the store; the vanilla ones keep using the mirror
// below so t() stays a plain object lookup.
export const lang = writable(savedLang === 'ru' ? 'ru' : 'en');

let activeLang = 'en';

// A writable store runs a new subscriber immediately, so this is also the
// startup path: it seeds activeLang and tags the document before first render.
lang.subscribe((value) => {
    activeLang = value;
    localStorage.setItem(LANG_KEY, value);
    // Screen readers and hyphenation follow the document language, not CSS.
    document.documentElement.lang = value;
});

const pluralRules = { en: new Intl.PluralRules('en'), ru: new Intl.PluralRules('ru') };

export function currentLang() {
    return activeLang;
}

export function setLang(next) {
    lang.set(next === 'ru' ? 'ru' : 'en');
}

// A replacement string would interpret "$&" etc., and plugin names come from
// an untrusted wincmd.ini — the function form inserts the value literally.
function interpolate(s, vars) {
    let out = s;
    for (const [name, value] of Object.entries(vars || {})) {
        out = out.replaceAll(`{${name}}`, () => value);
    }
    return out;
}

function translate(value, key, vars) {
    return interpolate(STRINGS[value][key] ?? STRINGS.en[key] ?? key, vars);
}

// Countable strings live as `${key}.one|few|many|other` (CLDR categories), so
// Russian gets «2 плагина» instead of a fixed «плагинов».
function pluralize(value, key, n, vars) {
    const category = pluralRules[value].select(n);
    const table = STRINGS[value];
    const s =
        table[`${key}.${category}`] ??
        STRINGS.en[`${key}.${category}`] ??
        table[`${key}.other`] ??
        STRINGS.en[`${key}.other`] ??
        key;
    return interpolate(s, { ...vars, n });
}

export function t(key, vars) {
    return translate(activeLang, key, vars);
}

export function tp(key, n, vars) {
    return pluralize(activeLang, key, n, vars);
}

// A Svelte component cannot see the mirror above, so it would never re-label
// itself on a language switch. These two read the store instead: the function
// they hand out changes identity with the language, which is what makes the
// component re-render. Vanilla screens keep calling t() and tp().
export const tr = derived(lang, (value) => (key, vars) => translate(value, key, vars));
export const trp = derived(lang, (value) => (key, n, vars) => pluralize(value, key, n, vars));
