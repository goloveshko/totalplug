// Minimal frontend i18n: EN is the default, the choice persists in localStorage.
const STRINGS = {
    en: {
        'status.initial': 'Press "Scan" to check plugins...',
        'btn.scan': '🔍 Scan plugins',
        'btn.refresh': '🔄 Refresh catalog',
        'btn.lang': 'Язык: RU',
        'btn.lang.title': 'Switch to Russian',
        'col.type': 'Type',
        'col.plugin': 'Plugin',
        'col.installed': 'Installed',
        'col.source': 'Source',
        'col.latest': 'In catalog',
        'col.status': 'Status',
        'col.actions': 'Actions',
        'table.empty': 'No plugins loaded',
        'table.none': 'No plugins found in wincmd.ini',
        'badge.up': 'Up to date',
        'badge.avail': 'Update!',
        'badge.missing': 'Missing',
        'badge.unknown': 'Unknown',
        'scan.cached': 'Scanning wincmd.ini and PE resources (catalog from cache)...',
        'scan.fresh': 'Cache shown, fresh catalog loading in background...',
        'scan.none': 'Scan finished: no plugins found.',
        'status.found': 'Found {n} plugin(s)',
        'status.error': 'Error: {err}',
        'btn.update': 'Update',
        'btn.edit': '✎ version',
        'btn.edit.title': 'Set version manually',
        'editor.title': 'Plugin version',
        'editor.version': 'Installed version',
        'editor.version.ph': 'e.g. 1.8.6.3',
        'editor.source': 'Update source',
        'editor.hint': 'Defaults come from auto-detection. Empty fields reset the manual override.',
        'editor.save': 'Save',
        'editor.cancel': 'Cancel',
        'editor.saved': 'Override for {name} saved, rescanning...',
        'editor.save_error': 'Failed to save override: {err}',
        'update.confirm': 'Update {name} to version {version}?',
        'update.running': 'Updating {name}...',
        'update.locked': '⚠ Files of {name} are locked. Close Total Commander and retry the update.',
        'update.done_backup': '{name} updated! Backup: {path}',
        'update.done': '{name} updated successfully!',
        'update.error_alert': 'Update failed: {err}',
        'update.error_status': 'Update failed for {name}',
        'events.refreshed': 'Cache shown, catalog updated. Plugins: {n}',
        'events.refresh_failed': '⚠ Background catalog refresh failed: {msg}',
        'btn.history': '🕘 History',
        'history.title': 'Applied updates',
        'history.col.plugin': 'Plugin',
        'history.col.version': 'Version',
        'history.col.date': 'Applied at',
        'history.empty': 'No updates applied yet.',
        'history.error': 'Failed to load history: {err}',
        'dialog.ok': 'OK',
        'dialog.cancel': 'Cancel',
    },
    ru: {
        'status.initial': 'Нажмите «Сканировать», чтобы проверить плагины...',
        'btn.scan': '🔍 Сканировать плагины',
        'btn.refresh': '🔄 Обновить каталог',
        'btn.lang': 'Language: EN',
        'btn.lang.title': 'Переключить на английский',
        'col.type': 'Тип',
        'col.plugin': 'Имя плагина',
        'col.installed': 'Установлена',
        'col.source': 'Источник',
        'col.latest': 'В каталоге',
        'col.status': 'Статус',
        'col.actions': 'Действие',
        'table.empty': 'Плагины не загружены',
        'table.none': 'Плагины не найдены в wincmd.ini',
        'badge.up': 'Актуален',
        'badge.avail': 'Обновление!',
        'badge.missing': 'Отсутствует',
        'badge.unknown': 'Неизвестно',
        'scan.cached': 'Сканирование wincmd.ini и PE-ресурсов (каталог из кэша)...',
        'scan.fresh': 'Кэш показан, свежий каталог загружается в фоне...',
        'scan.none': 'Сканирование завершено: плагины не обнаружены.',
        'status.found': 'Найдено плагинов: {n}',
        'status.error': 'Ошибка: {err}',
        'btn.update': 'Обновить',
        'btn.edit': '✎ версия',
        'btn.edit.title': 'Указать версию вручную',
        'editor.title': 'Версия плагина',
        'editor.version': 'Установленная версия',
        'editor.version.ph': 'например, 1.8.6.3',
        'editor.source': 'Источник обновлений',
        'editor.hint': 'Поля по умолчанию берутся из авто-определения. Пустые значения сбрасывают ручное переопределение.',
        'editor.save': 'Сохранить',
        'editor.cancel': 'Отмена',
        'editor.saved': 'Переопределение для {name} сохранено, пересканирую...',
        'editor.save_error': 'Ошибка сохранения переопределения: {err}',
        'update.confirm': 'Обновить плагин {name} до версии {version}?',
        'update.running': 'Обновление {name}...',
        'update.locked': '⚠ Файлы плагина {name} заняты. Закройте Total Commander и повторите обновление.',
        'update.done_backup': 'Плагин {name} обновлен! Резервная копия: {path}',
        'update.done': 'Плагин {name} успешно обновлен!',
        'update.error_alert': 'Ошибка обновления: {err}',
        'update.error_status': 'Ошибка при обновлении {name}',
        'events.refreshed': 'Данные из кэша, каталог обновлён. Плагинов: {n}',
        'events.refresh_failed': '⚠ Фоновое обновление каталога не удалось: {msg}',
        'btn.history': '🕘 История',
        'history.title': 'Применённые обновления',
        'history.col.plugin': 'Плагин',
        'history.col.version': 'Версия',
        'history.col.date': 'Применено',
        'history.empty': 'Обновлений ещё не было.',
        'history.error': 'Не удалось загрузить историю: {err}',
        'dialog.ok': 'ОК',
        'dialog.cancel': 'Отмена',
    },
};

const LANG_KEY = 'totalplug.lang';

let lang = localStorage.getItem(LANG_KEY);
if (lang !== 'ru' && lang !== 'en') {
    lang = 'en';
}

export function currentLang() {
    return lang;
}

export function toggleLang() {
    lang = lang === 'en' ? 'ru' : 'en';
    localStorage.setItem(LANG_KEY, lang);
}

export function t(key, vars) {
    let s = STRINGS[lang][key] ?? STRINGS.en[key] ?? key;
    for (const [name, value] of Object.entries(vars || {})) {
        s = s.replaceAll(`{${name}}`, value);
    }
    return s;
}
