import './style.css';
import { ScanPlugins, UpdatePlugin, SaveOverride, GetUpdateHistory } from '../wailsjs/go/main/App';
import { EventsOn, BrowserOpenURL, WindowMinimise, WindowToggleMaximise, Quit } from '../wailsjs/runtime';
import { t, currentLang, setLang } from './i18n.js';
import { currentTheme, setTheme, currentScale, setScale } from './theme.js';

const app = document.getElementById('app');

let currentPlugins = [];
let editingPlugin = null;

// Reassigned by buildDom() on every (re)render, e.g. after a language switch.
let tbody, btnScan, btnRefresh, searchEl;
let cardDialog, cardNameEl, cardSubEl, cardInfoEl;
let editorForm, editorVersionEl, editorSourceEl, editorCancelEl;
let historyDialog, historyBody;
let appDialog, appDialogMsg, appDialogOk, appDialogCancel;
let progressDialog, progressTitle, progressLive, progressTrack, progressFill;
let progressPhase, progressUrl, progressResult, progressOk, progressClose;
let mainView, settingsView, langSelect, fontScaleEl, fontScaleValueEl;
let titlebarActionsEl, pluginCountEl;
let dialogResolve = null;
// Progress dialog state: while an update runs, the dialog cannot be dismissed.
let progressResolve = null;
let progressRunning = false;
// Catalog freshness shown as a colored dot on the refresh icon: cache (amber),
// fresh (green), error (red). Survives the buildDom rebuild on language switch.
let catalogState = '';
let catalogStateDetail = '';
const catalogStateLabels = { cache: 'refresh.state.cache', fresh: 'refresh.state.fresh', error: 'refresh.state.error' };

// `detail` (the raw background-refresh error) is appended to the tooltip —
// there is no status bar left to show it.
function setCatalogState(state, detail) {
    catalogState = state || '';
    catalogStateDetail = detail || '';
    if (!catalogState) {
        delete btnRefresh.dataset.catalog;
        btnRefresh.title = t('btn.refresh');
        return;
    }
    btnRefresh.dataset.catalog = catalogState;
    const label = t(catalogStateLabels[catalogState]);
    btnRefresh.title = catalogStateDetail
        ? `${t('btn.refresh')} — ${label}: ${catalogStateDetail}`
        : `${t('btn.refresh')} — ${label}`;
}

// Plugin-count chip next to the app title: visible rows, or shown/total while
// the search filter hides some. Kept as state so the language-switch rebuild
// and the settings view (chrome hidden) restore it correctly.
let pluginCountShown = 0;
let pluginCountTotal = 0;

function setPluginCount(shown, total) {
    pluginCountShown = shown;
    pluginCountTotal = total;
    renderPluginCount();
}

function renderPluginCount() {
    const filtered = searchQuery.trim() && pluginCountShown !== pluginCountTotal;
    const label = t('count.plugins');
    const value = filtered ? `${pluginCountShown}/${pluginCountTotal}` : String(pluginCountTotal);
    pluginCountEl.textContent = `${label} ${value}`;
    pluginCountEl.hidden = pluginCountTotal === 0 || settingsOpen;
}

// Case-insensitive name filter from the toolbar search box; survives the DOM
// rebuild on language switch, like the settings state below.
let searchQuery = '';

// Settings is a full-window view; these survive the DOM rebuild that a
// language switch triggers so the user stays where they were.
let settingsOpen = false;
let settingsSection = 'appearance';

// In-app replacement for native confirm()/alert(): those render with the
// "wails" origin header and cannot be styled.
const escapeHtml = (s) => String(s)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

// Turn http(s) URLs in already-escaped text into clickable links. Plain anchors
// would navigate the webview, so clicks are intercepted and routed to the
// system browser (see the appDialogMsg listener in buildDom).
const linkify = (escaped) => escaped.replace(
    /(https?:\/\/[^\s<]+)/g,
    '<a href="$1" class="dlg-link">$1</a>',
);

function openAppDialog(html, isConfirm) {
    return new Promise((resolve) => {
        dialogResolve = resolve;
        appDialogMsg.innerHTML = html;
        appDialogCancel.style.display = isConfirm ? '' : 'none';
        appDialog.showModal();
    });
}
const textToDialogHtml = (message) => linkify(escapeHtml(message));
const showConfirm = (message) => openAppDialog(textToDialogHtml(message), true);

// Raw Go errors are technical English; show the localized friendly sentence
// with a named link and hide the exact error behind a disclosure. The link
// label depends on where the URL came from (plugin page vs catalog).
function errorDialogHtml(err, friendlyMessage, linkLabel) {
    const raw = String(err?.message ?? err);
    const urlMatch = raw.match(/https?:\/\/[^\s]+/);
    let html = escapeHtml(friendlyMessage);
    if (urlMatch) {
        html += '\n<a href="' + escapeHtml(urlMatch[0]) + '" class="dlg-link">' +
            escapeHtml(linkLabel || t('dialog.plugin_page')) + '</a>';
    }
    html += '<details class="dlg-tech"><summary>' + escapeHtml(t('dialog.tech_details')) +
        '</summary><div class="dlg-tech-body">' + linkify(escapeHtml(raw)) + '</div></details>';
    return html;
}

function settleAppDialog(result) {
    if (dialogResolve) {
        dialogResolve(result);
        dialogResolve = null;
    }
}

function settleProgress() {
    if (progressResolve) {
        progressResolve();
        progressResolve = null;
    }
}

// renderUpdateProgress maps one updater event onto the dialog. Events arriving
// outside a running update (stale listener, closed dialog) are ignored.
function renderUpdateProgress(p) {
    if (!progressDialog || !progressDialog.open || !progressRunning) return;

    const percent = Number(p?.percent ?? -1);
    const phase = p?.phase || 'download';
    if (phase === 'download' && percent >= 0) {
        progressPhase.textContent = `${t('update.progress.download')} — ${Math.min(100, Math.round(percent))}%`;
        progressTrack.classList.remove('indeterminate');
        progressFill.style.width = `${Math.min(100, percent)}%`;
    } else {
        progressPhase.textContent = `${t('update.progress.' + phase) || phase}…`;
        progressTrack.classList.add('indeterminate');
    }
    const url = String(p?.url || '');
    // Sticky: extract/install events carry no URL, keep showing the download source.
    if (url) {
        progressUrl.textContent = `${t('update.progress.source')}: ${url}`;
        progressUrl.title = url;
    }
}

function getStatusBadge(status) {
    switch (status) {
        case 'up_to_date':
            return `<span class="badge badge-up">✓ ${t('badge.up')}</span>`;
        case 'update_avail':
            return `<span class="badge badge-update">🚀 ${t('badge.avail')}</span>`;
        case 'not_found':
            return `<span class="badge badge-err">✗ ${t('badge.missing')}</span>`;
        default:
            return `<span class="badge badge-muted">⚠️ ${t('badge.unknown')}</span>`;
    }
}

async function runScan(forceRefresh = false) {
    try {
        btnScan.disabled = true;
        btnRefresh.disabled = true;

        currentPlugins = await ScanPlugins(forceRefresh);
        // Whatever the scan returned is cache-first; the network refresh lands
        // later through plugins:refreshed.
        setCatalogState('cache');

        if (!currentPlugins || currentPlugins.length === 0) {
            tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; padding:20px;">${t('table.none')}</td></tr>`;
            setPluginCount(0, 0);
            return;
        }

        renderTable();
    } catch (err) {
        // No status bar left: a failed scan is an error like any other; the
        // URL in it points at the catalog, not a plugin page.
        openAppDialog(errorDialogHtml(err, t('scan.error_friendly'), t('about.catalog')));
        console.error(err);
    } finally {
        btnScan.disabled = false;
        btnRefresh.disabled = false;
    }
}

function renderTable() {
    const q = searchQuery.trim().toLowerCase();
    const rows = currentPlugins
        .map((p, idx) => ({ p, idx }))
        .filter(({ p }) => !q || p.name.toLowerCase().includes(q));
    setPluginCount(rows.length, currentPlugins.length);
    if (rows.length === 0) {
        tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; padding:20px;">${t('table.noresults')}</td></tr>`;
        return;
    }
    tbody.innerHTML = rows.map(({ p, idx }) => {
        const canUpdate = p.status === 'update_avail' && p.download_url;
        return `
            <tr data-idx="${idx}">
                <td><strong>${p.type}</strong></td>
                <td>${p.name}</td>
                <td><code>${p.installed_version}</code></td>
                <td><small style="color:var(--text-muted)">${p.confidence}</small></td>
                <td><code>${p.latest_version || '-'}</code></td>
                <td>${getStatusBadge(p.status)}</td>
                <td>
                    ${canUpdate ? `<button class="btn-update" onclick="handleUpdate(${idx})">${t('btn.update')}</button>` : ''}
                </td>
            </tr>
        `;
    }).join('');
}

const cardRow = (label, valueHtml) =>
    `<div class="k">${label}</div><div class="v">${valueHtml}</div>`;

function openCard(index) {
    const plugin = currentPlugins[index];
    if (!plugin) return;

    editingPlugin = plugin;
    cardNameEl.textContent = plugin.name;
    cardSubEl.textContent = plugin.type + (plugin.arch ? ' · ' + plugin.arch : '');
    cardInfoEl.innerHTML =
        cardRow(t('col.installed'), `<code>${plugin.installed_version}</code> <small style="color:var(--text-muted)">${plugin.confidence}</small>`) +
        cardRow(t('col.latest'), plugin.latest_version ? `<code>${plugin.latest_version}</code>` : '—') +
        cardRow(t('col.status'), getStatusBadge(plugin.status)) +
        cardRow(t('editor.source'), plugin.source ? `<code class="path">${escapeHtml(plugin.source)}</code>` : '—') +
        (plugin.web_url ? cardRow(t('card.home'), `<a href="${escapeHtml(plugin.web_url)}" class="dlg-link">${escapeHtml(plugin.web_url)}</a>`) : '') +
        cardRow(t('card.path'), `<code class="path">${plugin.path}</code>`);

    const auto = plugin.installed_version || '';
    editorVersionEl.value = /^(unknown|n\/a)/i.test(auto) ? '' : auto;
    editorSourceEl.value = plugin.custom_source || '';

    cardDialog.showModal();
}

// The only scheme the checker understands is "github:Owner/Repo", but nobody
// remembers that format — accept what users actually paste (a GitHub page URL,
// with or without .git/trailing slash, or a bare owner/repo) and canonicalize.
// Anything else is stored as typed.
function normalizeSource(raw) {
    let s = raw.trim()
        .replace(/^https?:\/\/(?:www\.)?github\.com\//i, '')
        .replace(/^github:/i, '')
        .replace(/\.git$/i, '')
        .replace(/\/+$/, '');
    return /^[\w.-]+\/[\w.-]+$/.test(s) ? `github:${s}` : raw.trim();
}

async function saveOverrideFromEditor() {
    if (!editingPlugin) return;

    const override = {
        custom_version: editorVersionEl.value.trim(),
        custom_source: normalizeSource(editorSourceEl.value),
    };

    editorSaveGuard(true);
    try {
        await SaveOverride(editingPlugin.id, override);
        cardDialog.close();
        runScan(false);
    } catch (err) {
        openAppDialog(errorDialogHtml(err, t('editor.save_friendly')));
    } finally {
        editorSaveGuard(false);
    }
}

function editorSaveGuard(busy) {
    document.getElementById('editor-save').disabled = busy;
}

window.handleUpdate = async function(index) {
    const plugin = currentPlugins[index];
    if (!plugin) return;

    if (!await showConfirm(t('update.confirm', { name: plugin.name, version: plugin.latest_version }))) {
        return;
    }

    // One dialog shows progress, then the final result in the same place.
    progressTitle.textContent = t('update.progress.title', { name: plugin.name });
    progressPhase.textContent = `${t('update.progress.download')}…`;
    progressUrl.textContent = '';
    progressFill.style.width = '0%';
    progressTrack.classList.add('indeterminate');
    progressLive.hidden = false;
    progressResult.hidden = true;
    progressResult.innerHTML = '';
    progressOk.hidden = true;
    progressClose.hidden = true;
    progressRunning = true;
    progressDialog.showModal();

    const finishProgress = (html) => {
        progressRunning = false;
        progressLive.hidden = true;
        progressResult.innerHTML = html;
        progressResult.hidden = false;
        progressOk.hidden = false;
        progressClose.hidden = false;
    };

    let succeeded = false;
    try {
        const result = await UpdatePlugin(plugin);

        if (result && result.locked) {
            finishProgress(escapeHtml(t('update.locked', { name: plugin.name })));
        } else {
            finishProgress(escapeHtml(result && result.backup_path
                ? t('update.done_backup', { name: plugin.name, path: result.backup_path })
                : t('update.done', { name: plugin.name })));
            succeeded = true;
        }
    } catch (err) {
        finishProgress(errorDialogHtml(err, t('update.error_friendly')));
    }

    await new Promise((resolve) => { progressResolve = resolve; });
    if (succeeded) runScan(false);
};

async function openHistory() {
    try {
        const entries = await GetUpdateHistory();
        if (!entries || entries.length === 0) {
            historyBody.innerHTML = `<tr><td colspan="3" style="text-align:center; color:#777; padding:16px;">${t('history.empty')}</td></tr>`;
        } else {
            historyBody.innerHTML = entries.map((e) => `
                <tr>
                    <td>${e.plugin_name || e.plugin_id}</td>
                    <td><code>${e.version}</code></td>
                    <td>${new Date(e.applied_at).toLocaleString()}</td>
                </tr>
            `).join('');
        }
        historyDialog.showModal();
    } catch (err) {
        openAppDialog(errorDialogHtml(err, t('history.error_friendly')));
    }
}

function refreshSettingsState() {
    settingsView.querySelectorAll('[data-theme-choice]').forEach((el) => {
        el.classList.toggle('selected', el.dataset.themeChoice === currentTheme());
    });
    langSelect.value = currentLang();
    fontScaleEl.value = String(currentScale());
    fontScaleValueEl.textContent = currentScale() + '%';
    settingsView.querySelectorAll('.settings-nav button').forEach((btn) => {
        btn.classList.toggle('active', btn.dataset.panel === settingsSection);
    });
    settingsView.querySelectorAll('.settings-panel').forEach((panel) => {
        panel.classList.toggle('active', panel.id === 'panel-' + settingsSection);
    });
}

// The titlebar search box and action icons belong to the plugin view only;
// they hide while the full-window settings view is open.
function setMainChromeVisible(visible) {
    searchEl.hidden = !visible;
    titlebarActionsEl.hidden = !visible;
    renderPluginCount();
}

function openSettings() {
    settingsOpen = true;
    mainView.hidden = true;
    settingsView.hidden = false;
    setMainChromeVisible(false);
    refreshSettingsState();
}

function closeSettings() {
    settingsOpen = false;
    settingsView.hidden = true;
    mainView.hidden = false;
    setMainChromeVisible(true);
}

function buildDom() {
    app.innerHTML = `
      <div class="titlebar" id="titlebar">
        <div class="titlebar-left">
          <span class="titlebar-title">TotalPlug</span>
          <span class="titlebar-count" id="plugin-count" hidden></span>
        </div>
        <input type="search" id="plugin-search" placeholder="${t('search.ph')}" autocomplete="off">
        <div class="titlebar-actions" id="titlebar-actions">
          <button type="button" id="btn-history" class="icon-btn" title="${t('btn.history')}">
            <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
          </button>
          <button type="button" id="btn-refresh" class="icon-btn" title="${t('btn.refresh')}">
            <svg viewBox="0 0 24 24"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
          </button>
          <button type="button" id="btn-scan" class="icon-btn primary" title="${t('btn.scan')}">
            <svg viewBox="0 0 24 24"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
          </button>
          <button type="button" id="btn-settings" class="icon-btn" title="${t('btn.settings')}">
            <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
          </button>
        </div>
        <div class="caption-buttons">
          <button type="button" id="win-min" title="${t('win.min')}">&#8722;</button>
          <button type="button" id="win-max" title="${t('win.max')}">&#9633;</button>
          <button type="button" id="win-close" class="win-close" title="${t('win.close')}">&#10005;</button>
        </div>
      </div>
      <div id="view-main">
        <div class="table-container">
          <table>
            <thead>
              <tr>
                <th>${t('col.type')}</th>
                <th>${t('col.plugin')}</th>
                <th>${t('col.installed')}</th>
                <th>${t('col.source')}</th>
                <th>${t('col.latest')}</th>
                <th>${t('col.status')}</th>
                <th>${t('col.actions')}</th>
              </tr>
            </thead>
            <tbody id="plugins-body">
              <tr><td colspan="7" style="text-align:center; color:var(--text-muted); padding: 24px;">${t('table.empty')}</td></tr>
            </tbody>
          </table>
        </div>
      </div>

      <div id="view-settings" hidden>
        <div class="settings-top">
          <button type="button" id="settings-back" class="btn-back">${t('settings.back')}</button>
          <h2>${t('settings.title')}</h2>
          <span class="settings-save-hint">${t('settings.save_hint')}</span>
        </div>
        <div class="settings-body">
          <div class="settings-nav">
            <button type="button" data-panel="appearance">${t('settings.nav.appearance')}</button>
            <button type="button" data-panel="language">${t('settings.nav.language')}</button>
            <button type="button" data-panel="about">${t('settings.nav.about')}</button>
          </div>
          <div class="settings-panel" id="panel-appearance">
            <h3>${t('settings.nav.appearance')}</h3>
            <p class="section-desc">${t('settings.appearance')}</p>
            <div class="section-label">${t('settings.theme')}</div>
            <div class="theme-cards">
              <div class="theme-card" data-theme-choice="light" role="button" tabindex="0">
                <div class="theme-preview preview-light"></div>
                <div class="name">${t('settings.theme.light')}</div>
                <div class="desc">${t('settings.theme.light.desc')}</div>
              </div>
              <div class="theme-card" data-theme-choice="dark" role="button" tabindex="0">
                <div class="theme-preview preview-dark"></div>
                <div class="name">${t('settings.theme.dark')}</div>
                <div class="desc">${t('settings.theme.dark.desc')}</div>
              </div>
              <div class="theme-card" data-theme-choice="system" role="button" tabindex="0">
                <div class="theme-preview preview-system"></div>
                <div class="name">${t('settings.theme.system')}</div>
                <div class="desc">${t('settings.theme.system.desc')}</div>
              </div>
            </div>
            <div class="section-label">${t('settings.font')}</div>
            <div class="font-row">
              <input type="range" id="font-scale" min="80" max="140" step="5">
              <span class="font-value" id="font-scale-value">100%</span>
            </div>
          </div>
          <div class="settings-panel" id="panel-language">
            <h3>${t('settings.nav.language')}</h3>
            <p class="section-desc">${t('settings.language')}</p>
            <select id="lang-select">
              <option value="en">English</option>
              <option value="ru">Русский</option>
            </select>
          </div>
          <div class="settings-panel" id="panel-about">
            <h3>${t('settings.nav.about')}</h3>
            <div class="about-name">TotalPlug</div>
            <p class="about-desc">${t('about.desc')}</p>
            <div class="about-links">
              <button type="button" class="btn-secondary" id="link-catalog">${t('about.catalog')}</button>
            </div>
            <p class="editor-hint">${t('about.records')}</p>
          </div>
        </div>
      </div>

      <dialog id="plugin-card">
        <h2 id="card-name"></h2>
        <div class="card-sub" id="card-sub"></div>
        <div class="card-info" id="card-info"></div>
        <div class="card-edit-title">${t('editor.title')}</div>
        <form method="dialog" id="editor-form">
          <label>
            ${t('editor.version')}
            <input type="text" id="editor-version" placeholder="${t('editor.version.ph')}" autocomplete="off">
          </label>
          <label>
            ${t('editor.source')}
            <input type="text" id="editor-source" placeholder="${t('editor.source.ph')}" autocomplete="off">
          </label>
          <p class="editor-hint">${t('editor.hint')}</p>
          <div class="editor-actions">
            <button type="submit" class="btn-save" id="editor-save">${t('editor.save')}</button>
            <button type="button" id="editor-cancel">${t('editor.cancel')}</button>
          </div>
        </form>
      </dialog>

      <dialog id="history-dialog">
        <h2>${t('history.title')}</h2>
        <table>
          <thead>
            <tr>
              <th>${t('history.col.plugin')}</th>
              <th>${t('history.col.version')}</th>
              <th>${t('history.col.date')}</th>
            </tr>
          </thead>
          <tbody id="history-body"></tbody>
        </table>
        <div class="editor-actions">
          <button type="button" id="history-close">${t('editor.cancel')}</button>
        </div>
      </dialog>

      <dialog id="app-dialog">
        <button type="button" class="dlg-close" id="app-dialog-close" title="${t('dialog.close')}" aria-label="${t('dialog.close')}">
          <svg viewBox="0 0 24 24"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
        <div class="dlg-message" id="app-dialog-msg"></div>
        <div class="editor-actions">
          <button type="button" class="btn-save" id="app-dialog-ok">${t('dialog.ok')}</button>
          <button type="button" id="app-dialog-cancel">${t('dialog.cancel')}</button>
        </div>
      </dialog>

      <dialog id="update-progress">
        <button type="button" class="dlg-close" id="progress-close" title="${t('dialog.close')}" aria-label="${t('dialog.close')}" hidden>
          <svg viewBox="0 0 24 24"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
        <h2 id="progress-title"></h2>
        <div id="progress-live">
          <div class="progress-track" id="progress-track"><div class="progress-fill" id="progress-fill"></div></div>
          <div class="progress-phase" id="progress-phase"></div>
          <div class="progress-url" id="progress-url"></div>
        </div>
        <div class="dlg-message" id="progress-result" hidden></div>
        <div class="editor-actions">
          <button type="button" class="btn-save" id="progress-ok" hidden>${t('dialog.ok')}</button>
        </div>
      </dialog>
    `;

    tbody = document.getElementById('plugins-body');
    btnScan = document.getElementById('btn-scan');
    btnRefresh = document.getElementById('btn-refresh');
    // The refresh dot is state, not markup: restore it after the rebuild.
    if (catalogState) setCatalogState(catalogState, catalogStateDetail);
    searchEl = document.getElementById('plugin-search');
    pluginCountEl = document.getElementById('plugin-count');
    // The count chip is state too (language switch rebuilds the titlebar).
    renderPluginCount();

    cardDialog = document.getElementById('plugin-card');
    cardNameEl = document.getElementById('card-name');
    cardSubEl = document.getElementById('card-sub');
    cardInfoEl = document.getElementById('card-info');
    editorForm = document.getElementById('editor-form');
    editorVersionEl = document.getElementById('editor-version');
    editorSourceEl = document.getElementById('editor-source');
    editorCancelEl = document.getElementById('editor-cancel');
    historyDialog = document.getElementById('history-dialog');
    historyBody = document.getElementById('history-body');
    appDialog = document.getElementById('app-dialog');
    appDialogMsg = document.getElementById('app-dialog-msg');
    appDialogOk = document.getElementById('app-dialog-ok');
    appDialogCancel = document.getElementById('app-dialog-cancel');
    progressDialog = document.getElementById('update-progress');
    progressTitle = document.getElementById('progress-title');
    progressLive = document.getElementById('progress-live');
    progressTrack = document.getElementById('progress-track');
    progressFill = document.getElementById('progress-fill');
    progressPhase = document.getElementById('progress-phase');
    progressUrl = document.getElementById('progress-url');
    progressResult = document.getElementById('progress-result');
    progressOk = document.getElementById('progress-ok');
    progressClose = document.getElementById('progress-close');
    mainView = document.getElementById('view-main');
    settingsView = document.getElementById('view-settings');
    titlebarActionsEl = document.getElementById('titlebar-actions');
    langSelect = document.getElementById('lang-select');
    fontScaleEl = document.getElementById('font-scale');
    fontScaleValueEl = document.getElementById('font-scale-value');

    document.getElementById('win-min').addEventListener('click', WindowMinimise);
    document.getElementById('win-max').addEventListener('click', WindowToggleMaximise);
    document.getElementById('win-close').addEventListener('click', Quit);
    document.getElementById('titlebar').addEventListener('dblclick', (e) => {
        if (!e.target.closest('.caption-buttons')) WindowToggleMaximise();
    });

    btnScan.addEventListener('click', () => runScan(false));
    btnRefresh.addEventListener('click', () => runScan(true));
    document.getElementById('btn-history').addEventListener('click', openHistory);
    document.getElementById('btn-settings').addEventListener('click', openSettings);

    searchEl.value = searchQuery;
    searchEl.addEventListener('input', () => {
        searchQuery = searchEl.value;
        renderTable();
    });
    // Row click opens the plugin card; buttons inside the row keep their own action.
    tbody.addEventListener('click', (e) => {
        if (e.target.closest('button')) return;
        const tr = e.target.closest('tr[data-idx]');
        if (tr) openCard(Number(tr.dataset.idx));
    });

    appDialogOk.addEventListener('click', () => { settleAppDialog(true); appDialog.close(); });
    appDialogCancel.addEventListener('click', () => { settleAppDialog(false); appDialog.close(); });
    document.getElementById('app-dialog-close').addEventListener('click', () => { settleAppDialog(false); appDialog.close(); });
    // Links in dialog messages open in the system browser, never in the webview.
    appDialogMsg.addEventListener('click', (e) => {
        const a = e.target.closest('a.dlg-link');
        if (a) {
            e.preventDefault();
            BrowserOpenURL(a.href);
        }
    });
    // Same rule for the plugin card homepage link.
    cardInfoEl.addEventListener('click', (e) => {
        const a = e.target.closest('a.dlg-link');
        if (a) {
            e.preventDefault();
            BrowserOpenURL(a.href);
        }
    });
    // Esc or any other dismissal path must resolve a pending confirm as "no".
    appDialog.addEventListener('close', () => settleAppDialog(false));

    // Update progress dialog: not dismissable while the update is running;
    // afterwards OK or the cross closes it and handleUpdate continues.
    progressOk.addEventListener('click', () => { settleProgress(); progressDialog.close(); });
    progressClose.addEventListener('click', () => { settleProgress(); progressDialog.close(); });
    progressDialog.addEventListener('cancel', (e) => { if (progressRunning) e.preventDefault(); });
    progressDialog.addEventListener('close', () => settleProgress());
    progressResult.addEventListener('click', (e) => {
        const a = e.target.closest('a.dlg-link');
        if (a) {
            e.preventDefault();
            BrowserOpenURL(a.href);
        }
    });

    document.getElementById('settings-back').addEventListener('click', closeSettings);
    settingsView.querySelectorAll('.settings-nav button').forEach((btn) => {
        btn.addEventListener('click', () => {
            settingsSection = btn.dataset.panel;
            refreshSettingsState();
        });
    });
    settingsView.querySelectorAll('[data-theme-choice]').forEach((card) => {
        card.addEventListener('click', () => {
            setTheme(card.dataset.themeChoice);
            refreshSettingsState();
        });
    });
    fontScaleEl.addEventListener('input', () => {
        setScale(Number(fontScaleEl.value));
        fontScaleValueEl.textContent = currentScale() + '%';
    });
    langSelect.addEventListener('change', () => {
        setLang(langSelect.value);
        // Rebuild all labels; buildDom() below keeps the settings view open.
        buildDom();
    });
    document.getElementById('link-catalog').addEventListener('click', () => {
        BrowserOpenURL('https://sergey.is-a.dev/totalplug-catalog/');
    });

    // Restore the visible view after a rebuild (language switch happens inside
    // settings, which must stay open).
    settingsView.hidden = !settingsOpen;
    mainView.hidden = settingsOpen;
    setMainChromeVisible(!settingsOpen);
    if (settingsOpen) refreshSettingsState();

    editorForm.addEventListener('submit', async (e) => {
        // method="dialog" would close before the async save finishes
        e.preventDefault();
        await saveOverrideFromEditor();
    });
    editorCancelEl.addEventListener('click', () => cardDialog.close());
    document.getElementById('history-close').addEventListener('click', () => historyDialog.close());

    if (currentPlugins.length > 0) {
        renderTable();
    }
}

// Background catalog refresh (pushed by the Go bridge after every scan)
EventsOn('plugins:refreshed', (plugins) => {
    if (!plugins || plugins.length === 0) return;
    currentPlugins = plugins;
    renderTable();
    setCatalogState('fresh');
});

EventsOn('plugins:refresh:failed', (msg) => {
    // The dot plus its tooltip is the only surface left for this failure.
    setCatalogState('error', String(msg || ''));
});

// Update progress stream (pushed by the updater through a progress listener).
EventsOn('update:progress', (p) => {
    renderUpdateProgress(p);
});

buildDom();

// Startup: cache-only scan, no network wait before the window is useful
runScan(false);
