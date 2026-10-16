import { ScanPlugins } from '../wailsjs/go/gui/App';
import { EventsOn } from '../wailsjs/runtime';
import { get, writable } from 'svelte/store';
import { t, tp } from './i18n.js';
import { dom } from './dom.js';
import { state } from './state.js';
import { isUpdatable, getStatusBadge } from './plugin.js';
import { escapeHtml, emptyRow } from './text.js';
import { openAppDialog, errorDialogHtml } from './dialog.js';

// The plugin view: the table, the batch bar, and the toolbar state the titlebar
// island shows (count chip, search query, refresh dot, in-flight scan). Also the
// only screen that receives backend pushes, which is why its event registration
// lives here.

// Case-insensitive name filter from the toolbar search box. A store because the
// search box lives in the island, which never rebuilds, while the table below it
// is rewritten on every language switch.
export const searchQuery = writable('');

// Catalog freshness shown as a colored dot on the refresh icon: cache (amber),
// fresh (green), error (red). `{ state: '' }` means no dot at all.
export const catalogStatus = writable({ state: '', detail: '' });

// Plugin-count chip next to the app title: visible rows, or shown/total while
// the search filter hides some.
export const pluginCount = writable({ shown: 0, total: 0 });

// True while a scan is in flight; the island disables both toolbar buttons.
export const scanRunning = writable(false);

export const catalogMarkup = () => `
      <div id="view-main">
        <div class="update-all" id="update-all" hidden>
          <span class="update-all-count" id="update-all-count"></span>
          <button type="button" class="btn-save" id="btn-update-all">${t('update_all.btn')}</button>
        </div>
        <div class="table-container">
          <table>
            <thead>
              <tr>
                <th>${t('col.type')}</th>
                <th>${t('col.plugin')}</th>
                <th>${t('col.installed')}</th>
                <th>${t('col.latest')}</th>
                <th>${t('col.status')}</th>
                <th>${t('col.actions')}</th>
              </tr>
            </thead>
            <tbody id="plugins-body">
              ${emptyRow(t('table.scanning'))}
            </tbody>
          </table>
        </div>
      </div>`;

// `detail` (the raw background-refresh error) is appended to the tooltip by the
// island — there is no status bar left to show it.
export function setCatalogState(catalog, detail) {
    catalogStatus.set({ state: catalog || '', detail: detail || '' });
}

function setCount(shown, total) {
    pluginCount.set({ shown, total });
}

// The batch bar is contextual: it exists only while an update is actually
// offerable, and it counts every offer, not just the rows the search filter
// shows. The number keeps a label («Доступно 3 обновления»), like the count chip.
export function renderUpdateAllBar() {
    if (!dom.updateAll) return;
    const count = state.plugins.filter(isUpdatable).length;
    dom.updateAll.hidden = count === 0;
    dom.updateAllCount.textContent = tp('update_all.count', count);
    dom.btnUpdateAll.disabled = state.progressRunning;
}

export function renderTable() {
    const q = get(searchQuery).trim().toLowerCase();
    const rows = state.plugins.map((p, idx) => ({ p, idx })).filter(({ p }) => !q || p.name.toLowerCase().includes(q));
    setCount(rows.length, state.plugins.length);
    renderUpdateAllBar();
    if (rows.length === 0) {
        dom.tbody.innerHTML = emptyRow(t('table.noresults'));
        return;
    }
    // Every cell is built from wincmd.ini / catalog data: a name with "<" or "&"
    // breaks the markup unless it is escaped. The detection confidence is not a
    // column any more — it lives in the plugin card.
    dom.tbody.innerHTML = rows
        .map(({ p, idx }) => {
            const canUpdate = isUpdatable(p);
            return `
            <tr data-idx="${idx}">
                <td class="cell-type">${escapeHtml(p.type)}</td>
                <td>${escapeHtml(p.name)}</td>
                <td><code>${escapeHtml(p.installed_version)}</code></td>
                <td><code>${escapeHtml(p.latest_version || '—')}</code></td>
                <td>${getStatusBadge(p.status)}</td>
                <td>
                    ${canUpdate ? `<button type="button" class="btn-update" data-update="${idx}">${t('btn.update')}</button>` : ''}
                </td>
            </tr>
        `;
        })
        .join('');
}

export async function runScan(forceRefresh = false) {
    try {
        scanRunning.set(true);

        state.plugins = await ScanPlugins(forceRefresh);
        // Whatever the scan returned is cache-first; the network refresh lands
        // later through plugins:refreshed.
        setCatalogState('cache');

        if (!state.plugins || state.plugins.length === 0) {
            dom.tbody.innerHTML = emptyRow(t('table.none'));
            setCount(0, 0);
            return;
        }

        renderTable();
    } catch (err) {
        // The scan never produced rows, so the placeholder has to carry the reason:
        // the dialog below gets dismissed, and a row that only points at "the error
        // message" would then tell the user to look at something no longer on
        // screen. The exact error goes into a disclosure, as everywhere else.
        const raw = escapeHtml(String(err?.message ?? err));
        dom.tbody.innerHTML = emptyRow(
            escapeHtml(t('scan.error_friendly')) +
                '<details class="dlg-tech"><summary>' +
                escapeHtml(t('dialog.tech_details')) +
                '</summary><div class="dlg-tech-body">' +
                raw +
                '</div></details>',
        );
        openAppDialog(errorDialogHtml(err, t('scan.error_friendly'), t('about.catalog')));
        console.error(err);
    } finally {
        scanRunning.set(false);
    }
}

// Registered once, at startup: the Wails event handlers survive both a
// language switch and the scan that first fills the table.
export function initCatalogEvents() {
    EventsOn('plugins:refreshed', (plugins) => {
        if (!plugins || plugins.length === 0) return;
        state.plugins = plugins;
        renderTable();
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
    renderTable();
}
