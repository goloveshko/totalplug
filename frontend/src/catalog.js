import { ScanPlugins } from '../wailsjs/go/gui/App';
import { EventsOn } from '../wailsjs/runtime';
import { derived, writable } from 'svelte/store';
import { t } from './i18n.js';
import { plugins } from './state.js';
import { isUpdatable, matchesQuery } from './plugin.js';
import { openAppDialog, errorDialogHtml } from './dialog.js';

// The plugin view's state and its one backend call. The markup is
// PluginTable.svelte, so nothing here touches an element: a scan writes the
// plugin list, and the table and the titlebar island read it.

// Case-insensitive name filter from the toolbar search box. A store because the
// search box lives in the island, which never rebuilds, while the rows under it
// used to be rewritten on every language switch.
export const searchQuery = writable('');

// The count in the strip over the table is also a filter: while it is on, the
// table shows only what that number counted. The strip disappears when nothing
// is offerable, and PluginTable clears this flag with it — otherwise the filter
// would silently come back on the next scan that finds an update.
export const updatesOnly = writable(false);

// Catalog freshness shown as a colored dot on the check icon: cache (amber),
// fresh (green), error (red). `{ state: '' }` means no dot at all.
export const catalogStatus = writable({ state: '', detail: '' });

// Plugin-count chip next to the app title: visible rows, or shown/total while
// either filter hides some. Derived, so the chip and the table cannot disagree
// about what a row is.
export const pluginCount = derived([plugins, searchQuery, updatesOnly], ([$plugins, $query, $updatesOnly]) => {
    const matches = (p) => matchesQuery(p, $query) && (!$updatesOnly || isUpdatable(p));
    return {
        shown: $plugins.filter(matches).length,
        total: $plugins.length,
    };
});

// True while a scan is in flight; the island disables the check button.
export const scanRunning = writable(false);

// The raw reason the last scan failed, empty when there is none. It outranks the
// rows, because the dialog it also opens gets dismissed and a row that only
// pointed at "the error message" would then refer to something no longer on
// screen. Anything that paints the table again — a scan, a refresh, a keystroke —
// clears it.
export const scanFailure = writable('');

function setCatalogState(catalog, detail) {
    catalogStatus.set({ state: catalog || '', detail: detail || '' });
}

export async function runScan(forceRefresh = false) {
    try {
        scanRunning.set(true);
        scanFailure.set('');

        const list = await ScanPlugins(forceRefresh);
        plugins.set(list || []);
        // Whatever the scan returned is cache-first; the network refresh lands
        // later through plugins:refreshed.
        setCatalogState('cache');
    } catch (err) {
        const message = String(err?.message ?? err);
        scanFailure.set(message);
        // The scan could not even start: TotalPlug did not find wincmd.ini. The
        // remedy has to be in the visible text, not behind a disclosure — a
        // portable Total Commander is a common case, and an empty table with
        // "couldn't scan" is how the app looks broken.
        const remedy = message.includes('wincmd.ini') ? t('scan.error_no_ini') : t('scan.error_friendly');
        openAppDialog(errorDialogHtml(err, remedy, t('about.catalog')));
        console.error(err);
    } finally {
        scanRunning.set(false);
    }
}

// Registered once, at startup: the Wails event handlers survive both a
// language switch and the scan that first fills the table.
export function initCatalogEvents() {
    EventsOn('plugins:refreshed', (list) => {
        if (!list || list.length === 0) return;
        plugins.set(list);
        scanFailure.set('');
        setCatalogState('fresh');
    });

    EventsOn('plugins:refresh:failed', (msg) => {
        // The dot plus its tooltip is the only surface left for this failure.
        setCatalogState('error', String(msg || ''));
    });
}

// The titlebar island owns the search box; this is the only call that turns a
// keystroke into a redrawn table.
export function setSearchQuery(query) {
    searchQuery.set(query);
    scanFailure.set('');
}
