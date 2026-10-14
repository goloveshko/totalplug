import { ScanPlugins } from '../wailsjs/go/gui/App';
import { EventsOn } from '../wailsjs/runtime';
import { t, tp } from './i18n.js';
import { dom } from './dom.js';
import { state } from './state.js';
import { isUpdatable, getStatusBadge } from './plugin.js';
import { escapeHtml, emptyRow } from './text.js';
import { openAppDialog, errorDialogHtml } from './dialog.js';

// The plugin view: the table, the toolbar buttons that refetch it, the
// plugin-count chip and the batch bar. Also the only screen that receives
// backend pushes, which is why its event registration lives here.

// Case-insensitive name filter from the toolbar search box; survives the DOM
// rebuild on language switch because it is module state, not markup.
let searchQuery = '';

// Catalog freshness shown as a colored dot on the refresh icon: cache (amber),
// fresh (green), error (red). Survives the buildDom rebuild on language switch.
let catalogState = '';
let catalogStateDetail = '';
const catalogStateLabels = { cache: 'refresh.state.cache', fresh: 'refresh.state.fresh', error: 'refresh.state.error' };

// Plugin-count chip next to the app title: visible rows, or shown/total while
// the search filter hides some. Kept as state so the language-switch rebuild
// and the settings view (chrome hidden) restore it correctly.
let countShown = 0;
let countTotal = 0;

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

// `detail` (the raw background-refresh error) is appended to the tooltip —
// there is no status bar left to show it.
export function setCatalogState(catalog, detail) {
    catalogState = catalog || '';
    catalogStateDetail = detail || '';
    // The state lives in title + aria-label: a tooltip alone is mouse-only.
    if (!catalogState) {
        delete dom.btnRefresh.dataset.catalog;
        dom.btnRefresh.title = t('btn.refresh');
        dom.btnRefresh.setAttribute('aria-label', t('btn.refresh'));
        return;
    }
    dom.btnRefresh.dataset.catalog = catalogState;
    const label = t(catalogStateLabels[catalogState]);
    dom.btnRefresh.title = catalogStateDetail
        ? `${t('btn.refresh')} — ${label}: ${catalogStateDetail}`
        : `${t('btn.refresh')} — ${label}`;
    dom.btnRefresh.setAttribute('aria-label', `${t('btn.refresh')} — ${label}`);
}

function setCount(shown, total) {
    countShown = shown;
    countTotal = total;
    renderPluginCount();
}

export function renderPluginCount() {
    const filtered = searchQuery.trim() && countShown !== countTotal;
    // A filtered pair is language-neutral; the plain count needs the localized
    // plural form («2 плагина», not «2 плагинов»).
    dom.pluginCount.textContent = filtered ? `${countShown}/${countTotal}` : tp('count.plugins', countTotal);
    dom.pluginCount.hidden = countTotal === 0 || state.settingsOpen;
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
    const q = searchQuery.trim().toLowerCase();
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
        dom.btnScan.disabled = true;
        dom.btnRefresh.disabled = true;

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
        // The scan never produced rows, so the "scanning" placeholder must go:
        // no status bar is left, and the error itself opens a dialog (the URL in
        // it points at the catalog, not a plugin page).
        dom.tbody.innerHTML = emptyRow(t('table.error'));
        openAppDialog(errorDialogHtml(err, t('scan.error_friendly'), t('about.catalog')));
        console.error(err);
    } finally {
        dom.btnScan.disabled = false;
        dom.btnRefresh.disabled = false;
    }
}

// Registered once, at startup, before the first render: a language switch
// rebuilds the DOM but must not add a second listener to the same event.
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

export function bindCatalog() {
    // The refresh dot is state, not markup: restore it after the rebuild.
    if (catalogState) setCatalogState(catalogState, catalogStateDetail);
    // The count chip is state too (language switch rebuilds the titlebar).
    renderPluginCount();

    dom.btnScan.addEventListener('click', () => runScan(false));
    dom.btnRefresh.addEventListener('click', () => runScan(true));

    dom.search.value = searchQuery;
    dom.search.addEventListener('input', () => {
        searchQuery = dom.search.value;
        renderTable();
    });
}
