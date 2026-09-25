import './style.css';
import './mock.js'; // dev-only fake backend; compiles away in production builds
import { ScanPlugins, UpdatePlugin, SaveOverride, GetUpdateHistory } from '../wailsjs/go/main/App';
import { EventsOn, BrowserOpenURL, WindowMinimise, WindowToggleMaximise, Quit } from '../wailsjs/runtime';
import { t, tp, currentLang, setLang } from './i18n.js';
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
// Element that opened the current dialog; focus goes back to it on close so
// keyboard users do not land at the top of the document.
let appDialogOpener = null;
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
    // The state lives in title + aria-label: a tooltip alone is mouse-only.
    if (!catalogState) {
        delete btnRefresh.dataset.catalog;
        btnRefresh.title = t('btn.refresh');
        btnRefresh.setAttribute('aria-label', t('btn.refresh'));
        return;
    }
    btnRefresh.dataset.catalog = catalogState;
    const label = t(catalogStateLabels[catalogState]);
    btnRefresh.title = catalogStateDetail
        ? `${t('btn.refresh')} — ${label}: ${catalogStateDetail}`
        : `${t('btn.refresh')} — ${label}`;
    btnRefresh.setAttribute('aria-label', `${t('btn.refresh')} — ${label}`);
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
    // A filtered pair is language-neutral; the plain count needs the localized
    // plural form («2 плагина», not «2 плагинов»).
    pluginCountEl.textContent = filtered
        ? `${pluginCountShown}/${pluginCountTotal}`
        : tp('count.plugins', pluginCountTotal);
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

// Placeholder row for the plugin table (scanning / empty / no search hits).
const emptyRow = (message) =>
    `<tr class="table-empty"><td colspan="6">${message}</td></tr>`;

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
        appDialogOpener = document.activeElement;
        appDialogMsg.innerHTML = html;
        appDialogCancel.style.display = isConfirm ? '' : 'none';
        appDialog.showModal();
        // Enter must never confirm an update by accident: focus the safe button.
        (isConfirm ? appDialogCancel : appDialogOk).focus();
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
        // Go quotes URLs in errors: Get "https://host/x": EOF — the closing
        // quote and trailing punctuation are not part of the address.
        const url = urlMatch[0].replace(/[.,;:!?)\]»"']+$/, '');
        html += '\n<a href="' + escapeHtml(url) + '" class="dlg-link">' +
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
    // Monochrome glyphs only: color emoji render differently in WebView2, do not
    // follow the theme and get read out by screen readers.
    switch (status) {
        case 'up_to_date':
            return `<span class="badge badge-up">✓ ${t('badge.up')}</span>`;
        case 'update_avail':
            return `<span class="badge badge-update">↑ ${t('badge.avail')}</span>`;
        case 'not_found':
            return `<span class="badge badge-err">✗ ${t('badge.missing')}</span>`;
        default:
            return `<span class="badge badge-muted">? ${t('badge.unknown')}</span>`;
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
            tbody.innerHTML = emptyRow(t('table.none'));
            setPluginCount(0, 0);
            return;
        }

        renderTable();
    } catch (err) {
        // The scan never produced rows, so the "scanning" placeholder must go:
        // no status bar is left, and the error itself opens a dialog (the URL in
        // it points at the catalog, not a plugin page).
        tbody.innerHTML = emptyRow(t('table.error'));
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
        tbody.innerHTML = emptyRow(t('table.noresults'));
        return;
    }
    // Every cell is built from wincmd.ini / catalog data: a name with "<" or "&"
    // breaks the markup unless it is escaped. The detection confidence is not a
    // column any more — it lives in the plugin card.
    tbody.innerHTML = rows.map(({ p, idx }) => {
        const canUpdate = p.status === 'update_avail' && p.download_url;
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
    }).join('');
}

const cardRow = (label, valueHtml) =>
    `<div class="k">${label}</div><div class="v">${valueHtml}</div>`;

// "https://" is noise in a narrow dialog and costs a line wrap; the href keeps it.
const shortUrl = (url) => url.replace(/^https?:\/\//i, '').replace(/\/+$/, '');

// Which provider resolved the latest version, shown as a glyph beside it. It
// replaces a whole "Update source" row: for a GitHub plugin that row duplicated
// the homepage, and the raw keys ("totalcmd", "github:Owner/Repo") are only
// interesting to a user debugging something, so they moved to the details block.
// Each glyph paints itself: a CSS `fill` would override the markup attribute.
function sourceIcon(source) {
    if (!source) return '';
    const github = /^github:/i.test(source);
    const label = github ? 'GitHub' : t('card.source.catalog');
    const glyph = github
        // octicons/mark-github-16 (MIT, (c) GitHub Inc.)
        ? '<svg viewBox="0 0 16 16"><path fill="currentColor" d="M6.766 11.328c-2.063-.25-3.516-1.734-3.516-3.656 0-.781.281-1.625.75-2.188-.203-.515-.172-1.609.063-2.062.625-.078 1.468.25 1.968.703.594-.187 1.219-.281 1.985-.281.765 0 1.39.094 1.953.265.484-.437 1.344-.765 1.969-.687.218.422.25 1.515.046 2.047.5.593.766 1.39.766 2.203 0 1.922-1.453 3.375-3.547 3.64.531.344.89 1.094.89 1.954v1.625c0 .468.391.734.86.547C13.781 14.359 16 11.53 16 8.03 16 3.61 12.406 0 7.984 0 3.563 0 0 3.61 0 8.031a7.88 7.88 0 0 0 5.172 7.422c.422.156.828-.125.828-.547v-1.25c-.219.094-.5.156-.75.156-1.031 0-1.64-.562-2.078-1.609-.172-.422-.36-.672-.719-.719-.187-.015-.25-.093-.25-.187 0-.188.313-.328.625-.328.453 0 .844.281 1.25.86.313.452.64.655 1.031.655s.641-.14 1-.5c.266-.265.47-.5.657-.656"/></svg>'
        // Stand-in until the catalog ships its own mark.
        : '<svg viewBox="0 0 24 24"><g fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="7" height="7" rx="1.5"/><rect x="13" y="4" width="7" height="7" rx="1.5"/><rect x="4" y="13" width="7" height="7" rx="1.5"/><rect x="13" y="13" width="7" height="7" rx="1.5"/></g></svg>';
    return `<span class="src-icon" role="img" title="${escapeHtml(label)}" aria-label="${escapeHtml(label)}">${glyph}</span>`;
}

function openCard(index) {
    const plugin = currentPlugins[index];
    if (!plugin) return;

    editingPlugin = plugin;
    cardNameEl.textContent = plugin.name;
    cardSubEl.textContent = plugin.type + (plugin.arch ? ' · ' + plugin.arch : '');
    const latest = plugin.latest_version
        ? `<code>${escapeHtml(plugin.latest_version)}</code>${sourceIcon(plugin.source)}`
        : '—';
    const site = plugin.web_url
        ? `<a href="${escapeHtml(plugin.web_url)}" class="dlg-link">${escapeHtml(shortUrl(plugin.web_url))}</a>`
        : '';
    const tech = (plugin.source ? cardRow(t('card.source'), `<code>${escapeHtml(plugin.source)}</code>`) : '') +
        cardRow(t('card.path'), `<code class="path">${escapeHtml(plugin.path)}</code>`);
    cardInfoEl.innerHTML =
        cardRow(t('col.installed'), `<code>${escapeHtml(plugin.installed_version)}</code> <small class="muted">${escapeHtml(plugin.confidence)}</small>`) +
        cardRow(t('col.latest'), latest) +
        cardRow(t('col.status'), getStatusBadge(plugin.status)) +
        (site ? cardRow(t('card.site'), site) : '') +
        `<details class="card-tech"><summary>${t('card.details')}</summary><div class="card-tech-body">${tech}</div></details>`;

    const auto = plugin.installed_version || '';
    editorVersionEl.value = /^(unknown|n\/a)/i.test(auto) ? '' : auto;
    editorSourceEl.value = displaySource(plugin.custom_source);

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

// Inverse of normalizeSource for the same value: the stored key stays compact,
// the UI shows the page URL it came from (and re-folds it on save).
function displaySource(source) {
    const match = /^github:([\w.-]+\/[\w.-]+)\/?$/i.exec(source || '');
    return match ? `https://github.com/${match[1]}` : (source || '');
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

// Reached only through the delegated tbody click handler (no inline onclick, so
// a future script-src CSP stays possible).
async function handleUpdate(index) {
    // The confirm and the progress dialog are separate modals: without this
    // guard a second click starts a parallel UpdatePlugin for the same plugin.
    if (progressRunning) return;

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
        progressOk.focus();
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
            historyBody.innerHTML = `<tr class="table-empty"><td colspan="3">${t('history.empty')}</td></tr>`;
        } else {
            // Dates follow the UI language, not the OS locale.
            const fmt = new Intl.DateTimeFormat(
                currentLang() === 'ru' ? 'ru-RU' : 'en-GB',
                { dateStyle: 'medium', timeStyle: 'short' });
            historyBody.innerHTML = entries.map((e) => `
                <tr>
                    <td>${escapeHtml(e.plugin_name || e.plugin_id)}</td>
                    <td><code>${escapeHtml(e.version)}</code></td>
                    <td>${fmt.format(new Date(e.applied_at))}</td>
                </tr>
            `).join('');
        }
        historyDialog.showModal();
        document.getElementById('history-close').focus();
    } catch (err) {
        openAppDialog(errorDialogHtml(err, t('history.error_friendly')));
    }
}

function refreshSettingsState() {
    settingsView.querySelectorAll('[data-theme-choice]').forEach((el) => {
        const selected = el.dataset.themeChoice === currentTheme();
        el.classList.toggle('selected', selected);
        el.setAttribute('aria-pressed', String(selected));
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
        <input type="search" id="plugin-search" placeholder="${t('search.ph')}" aria-label="${t('search.ph')}" autocomplete="off">
        <div class="titlebar-actions" id="titlebar-actions">
          <button type="button" id="btn-history" class="icon-btn" title="${t('btn.history')}" aria-label="${t('btn.history')}">
            <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
          </button>
          <button type="button" id="btn-refresh" class="icon-btn" title="${t('btn.refresh')}" aria-label="${t('btn.refresh')}">
            <svg viewBox="0 0 24 24"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
          </button>
          <button type="button" id="btn-scan" class="icon-btn primary" title="${t('btn.scan')}" aria-label="${t('btn.scan')}">
            <svg viewBox="0 0 24 24"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
          </button>
          <button type="button" id="btn-settings" class="icon-btn" title="${t('btn.settings')}" aria-label="${t('btn.settings')}">
            <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
          </button>
        </div>
        <div class="caption-buttons">
          <button type="button" id="win-min" title="${t('win.min')}" aria-label="${t('win.min')}">&#8722;</button>
          <button type="button" id="win-max" title="${t('win.max')}" aria-label="${t('win.max')}">&#9633;</button>
          <button type="button" id="win-close" class="win-close" title="${t('win.close')}" aria-label="${t('win.close')}">&#10005;</button>
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
              <button type="button" class="theme-card" data-theme-choice="light" aria-pressed="false">
                <span class="theme-preview preview-light"></span>
                <span class="name">${t('settings.theme.light')}</span>
                <span class="desc">${t('settings.theme.light.desc')}</span>
              </button>
              <button type="button" class="theme-card" data-theme-choice="dark" aria-pressed="false">
                <span class="theme-preview preview-dark"></span>
                <span class="name">${t('settings.theme.dark')}</span>
                <span class="desc">${t('settings.theme.dark.desc')}</span>
              </button>
              <button type="button" class="theme-card" data-theme-choice="system" aria-pressed="false">
                <span class="theme-preview preview-system"></span>
                <span class="name">${t('settings.theme.system')}</span>
                <span class="desc">${t('settings.theme.system.desc')}</span>
              </button>
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
          <button type="button" id="history-close">${t('dialog.close')}</button>
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
        // Double-clicking the search box (select a word) or any button must not
        // maximise the window — only the empty drag area does.
        if (e.target.closest('input, button')) return;
        WindowToggleMaximise();
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
    // One delegated handler for the table: the update button keeps its own
    // action, a click anywhere else in the row opens the plugin card.
    tbody.addEventListener('click', (e) => {
        const updateBtn = e.target.closest('button.btn-update');
        if (updateBtn) {
            handleUpdate(Number(updateBtn.dataset.update));
            return;
        }
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
    appDialog.addEventListener('close', () => {
        settleAppDialog(false);
        if (appDialogOpener) {
            appDialogOpener.focus?.();
            appDialogOpener = null;
        }
    });

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
    // Real <button>s, so Enter/Space reach the handler through the native click.
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
