import './style.css';
import './mock.js'; // dev-only fake backend; compiles away in production builds
import {
    ScanPlugins,
    UpdatePlugin,
    SaveOverride,
    GetUpdateHistory,
    AppInfo,
    CheckForAppUpdate,
    GetTcConfig,
    SetTcConfig,
    BrowseTcIni,
} from '../wailsjs/go/main/App';
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
let updateAllEl, updateAllCountEl, btnUpdateAll;
let dialogResolve = null;
// Element that opened the current dialog; focus goes back to it on close so
// keyboard users do not land at the top of the document.
let appDialogOpener = null;
// The same for the plugin card, whose opener is a table row. A row is not
// focusable until it is asked to be, so opening the card gives it a tabindex and
// closing hands the keyboard back to it.
let cardOpener = null;
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
const escapeHtml = (s) =>
    String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

// Placeholder row for the plugin table (scanning / empty / no search hits).
const emptyRow = (message) => `<tr class="table-empty"><td colspan="6">${message}</td></tr>`;

// Turn http(s) URLs in already-escaped text into clickable links. Plain anchors
// would navigate the webview, so clicks are intercepted and routed to the
// system browser (see the appDialogMsg listener in buildDom).
const linkify = (escaped) => escaped.replace(/(https?:\/\/[^\s<]+)/g, '<a href="$1" class="dlg-link">$1</a>');

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
        html +=
            '\n<a href="' +
            escapeHtml(url) +
            '" class="dlg-link">' +
            escapeHtml(linkLabel || t('dialog.plugin_page')) +
            '</a>';
    }
    html +=
        '<details class="dlg-tech"><summary>' +
        escapeHtml(t('dialog.tech_details')) +
        '</summary><div class="dlg-tech-body">' +
        linkify(escapeHtml(raw)) +
        '</div></details>';
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

// The batch bar is contextual: it exists only while an update is actually
// offerable, and it counts every offer, not just the rows the search filter
// shows. The number keeps a label («Доступно 3 обновления»), like the count chip.
function renderUpdateAllBar() {
    if (!updateAllEl) return;
    const count = currentPlugins.filter(isUpdatable).length;
    updateAllEl.hidden = count === 0;
    updateAllCountEl.textContent = tp('update_all.count', count);
    btnUpdateAll.disabled = progressRunning;
}

function renderTable() {
    const q = searchQuery.trim().toLowerCase();
    const rows = currentPlugins.map((p, idx) => ({ p, idx })).filter(({ p }) => !q || p.name.toLowerCase().includes(q));
    setPluginCount(rows.length, currentPlugins.length);
    renderUpdateAllBar();
    if (rows.length === 0) {
        tbody.innerHTML = emptyRow(t('table.noresults'));
        return;
    }
    // Every cell is built from wincmd.ini / catalog data: a name with "<" or "&"
    // breaks the markup unless it is escaped. The detection confidence is not a
    // column any more — it lives in the plugin card.
    tbody.innerHTML = rows
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

const cardRow = (label, valueHtml) => `<div class="k">${label}</div><div class="v">${valueHtml}</div>`;

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
        ? // octicons/mark-github-16 (MIT, (c) GitHub Inc.)
          '<svg viewBox="0 0 16 16"><path fill="currentColor" d="M6.766 11.328c-2.063-.25-3.516-1.734-3.516-3.656 0-.781.281-1.625.75-2.188-.203-.515-.172-1.609.063-2.062.625-.078 1.468.25 1.968.703.594-.187 1.219-.281 1.985-.281.765 0 1.39.094 1.953.265.484-.437 1.344-.765 1.969-.687.218.422.25 1.515.046 2.047.5.593.766 1.39.766 2.203 0 1.922-1.453 3.375-3.547 3.64.531.344.89 1.094.89 1.954v1.625c0 .468.391.734.86.547C13.781 14.359 16 11.53 16 8.03 16 3.61 12.406 0 7.984 0 3.563 0 0 3.61 0 8.031a7.88 7.88 0 0 0 5.172 7.422c.422.156.828-.125.828-.547v-1.25c-.219.094-.5.156-.75.156-1.031 0-1.64-.562-2.078-1.609-.172-.422-.36-.672-.719-.719-.187-.015-.25-.093-.25-.187 0-.188.313-.328.625-.328.453 0 .844.281 1.25.86.313.452.64.655 1.031.655s.641-.14 1-.5c.266-.265.47-.5.657-.656"/></svg>'
        : // Stand-in until the catalog ships its own mark.
          '<svg viewBox="0 0 24 24"><g fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="7" height="7" rx="1.5"/><rect x="13" y="4" width="7" height="7" rx="1.5"/><rect x="4" y="13" width="7" height="7" rx="1.5"/><rect x="13" y="13" width="7" height="7" rx="1.5"/></g></svg>';
    return `<span class="src-icon" role="img" title="${escapeHtml(label)}" aria-label="${escapeHtml(label)}">${glyph}</span>`;
}

function openCard(index, opener) {
    const plugin = currentPlugins[index];
    if (!plugin) return;

    // Remembered before the dialog opens: closing must put the keyboard back on
    // the row it came from, and a row only accepts focus once it is named.
    cardOpener = opener || null;
    cardOpener?.setAttribute('tabindex', '-1');

    editingPlugin = plugin;
    cardNameEl.textContent = plugin.name;
    cardSubEl.textContent = plugin.type + (plugin.arch ? ' · ' + plugin.arch : '');
    const latest = plugin.latest_version
        ? `<code>${escapeHtml(plugin.latest_version)}</code>${sourceIcon(plugin.source)}`
        : '—';
    const site = plugin.web_url
        ? `<a href="${escapeHtml(plugin.web_url)}" class="dlg-link">${escapeHtml(shortUrl(plugin.web_url))}</a>`
        : '';
    const tech =
        (plugin.source ? cardRow(t('card.source'), `<code>${escapeHtml(plugin.source)}</code>`) : '') +
        cardRow(t('card.path'), `<code class="path">${escapeHtml(plugin.path)}</code>`);
    cardInfoEl.innerHTML =
        cardRow(
            t('col.installed'),
            `<code>${escapeHtml(plugin.installed_version)}</code> <small class="muted">${escapeHtml(plugin.confidence)}</small>`,
        ) +
        cardRow(t('col.latest'), latest) +
        cardRow(t('col.status'), getStatusBadge(plugin.status)) +
        (site ? cardRow(t('card.site'), site) : '') +
        `<details class="card-tech"><summary>${t('card.details')}</summary><div class="card-tech-body">${tech}</div></details>`;

    const auto = plugin.installed_version || '';
    editorVersionEl.value = /^(unknown|n\/a)/i.test(auto) ? '' : auto;
    editorSourceEl.value = displaySource(plugin.custom_source);

    cardDialog.showModal();
    // Without this the dialog picks the first focusable element it finds, which is
    // the SITE link whenever the plugin has a page - so the link opens framed and
    // Enter navigates it. Cancel is the safe place to start: the version field is
    // prefilled from auto-detection, and confirming that would pin a manual
    // override over the catalog value the user never typed.
    editorCancelEl.focus();
}

function closeCard() {
    cardDialog.close();
    returnCardFocus();
}

// The keyboard goes back where it came from. Done at the dismissal sites rather
// than only in the `close` handler: that event is what catches Esc, but a dialog
// that our own button closes should not depend on it arriving.
function returnCardFocus() {
    // A background refresh can repaint the table while the card is open, so the
    // opener row may be detached by now; focusing such an element does nothing.
    cardOpener?.focus();
    cardOpener = null;
}

// The only scheme the checker understands is "github:Owner/Repo", but nobody
// remembers that format — accept what users actually paste (a GitHub page URL,
// with or without .git/trailing slash, or a bare owner/repo) and canonicalize.
// Anything else is stored as typed.
function normalizeSource(raw) {
    let s = raw
        .trim()
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
    return match ? `https://github.com/${match[1]}` : source || '';
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
        closeCard();
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

// Progress dialog plumbing shared by the single update and the "update all"
// queue. beginProgress opens the dialog and arms the event stream; beginProgressItem
// only rewinds the bar, so a queue reuses the same open dialog.
function beginProgress(titleText) {
    progressTitle.textContent = titleText;
    progressResult.innerHTML = '';
    progressResult.hidden = true;
    progressOk.hidden = true;
    progressClose.hidden = true;
    progressLive.hidden = false;
    progressRunning = true;
    renderUpdateAllBar();
    if (!progressDialog.open) progressDialog.showModal();
    beginProgressItem();
}

function beginProgressItem() {
    progressPhase.textContent = `${t('update.progress.download')}…`;
    progressUrl.textContent = '';
    progressFill.style.width = '0%';
    progressTrack.classList.add('indeterminate');
}

function finishProgress(html) {
    progressRunning = false;
    progressLive.hidden = true;
    progressResult.innerHTML = html;
    progressResult.hidden = false;
    progressOk.hidden = false;
    progressClose.hidden = false;
    progressOk.focus();
    renderUpdateAllBar();
}

// One updater call mapped onto the three outcomes the UI has to tell apart:
// success, a file lock (systemic: Total Commander holds every plugin file),
// and a per-plugin error.
async function updateOnePlugin(plugin) {
    try {
        const result = await UpdatePlugin(plugin);
        if (result && result.locked) return { kind: 'locked' };
        return { kind: 'ok', backupPath: result && result.backup_path };
    } catch (err) {
        return { kind: 'error', err };
    }
}

// Reached only through the delegated tbody click handler (no inline onclick, so
// a future script-src CSP stays possible).
async function handleUpdate(index) {
    // The confirm and the progress dialog are separate modals: without this
    // guard a second click starts a parallel UpdatePlugin for the same plugin.
    if (progressRunning) return;

    const plugin = currentPlugins[index];
    if (!plugin) return;

    if (!(await showConfirm(t('update.confirm', { name: plugin.name, version: plugin.latest_version })))) {
        return;
    }

    // One dialog shows progress, then the final result in the same place.
    beginProgress(t('update.progress.title', { name: plugin.name }));
    const outcome = await updateOnePlugin(plugin);

    if (outcome.kind === 'locked') {
        finishProgress(escapeHtml(t('update.locked', { name: plugin.name })));
    } else if (outcome.kind === 'error') {
        finishProgress(errorDialogHtml(outcome.err, t('update.error_friendly')));
    } else {
        finishProgress(
            escapeHtml(
                outcome.backupPath
                    ? t('update.done_backup', { name: plugin.name, path: outcome.backupPath })
                    : t('update.done', { name: plugin.name }),
            ),
        );
    }

    await new Promise((resolve) => {
        progressResolve = resolve;
    });
    if (outcome.kind === 'ok') runScan(false);
}

// A row offers the update button only when a download source is resolved, and
// the same rule defines the queue: "blind" updates (installed version unknown,
// hence StatusUncertain) stay a per-plugin decision, exactly like --update-all
// in the CLI.
const isUpdatable = (p) => p.status === 'update_avail' && !!p.download_url;

const summaryRow = (glyph, className, text) =>
    `<div class="ua-row ${className}"><span class="ua-glyph" aria-hidden="true">${glyph}</span><span>${escapeHtml(text)}</span></div>`;

async function handleUpdateAll() {
    if (progressRunning) return;

    const queue = currentPlugins.filter(isUpdatable);
    if (queue.length === 0) return;

    // The count is the whole queue, not the visible rows: the search filter is a
    // way to find a plugin, not a way to scope a batch, so the confirm has to
    // spell out what is actually about to happen.
    if (!(await showConfirm(t('update_all.confirm', { count: tp('count.plugins', queue.length) })))) {
        return;
    }

    const done = [];
    const failed = [];
    const rawErrors = [];
    let lockedAt = -1;

    beginProgress(t('update_all.title'));
    for (let i = 0; i < queue.length; i++) {
        const plugin = queue[i];
        progressTitle.textContent = t('update_all.item', {
            current: i + 1,
            total: queue.length,
            name: plugin.name,
        });
        beginProgressItem();

        const outcome = await updateOnePlugin(plugin);
        if (outcome.kind === 'ok') {
            done.push(plugin);
        } else if (outcome.kind === 'error') {
            failed.push(plugin);
            rawErrors.push(`${plugin.name}: ${String(outcome.err?.message ?? outcome.err)}`);
        } else {
            // One locked file means Total Commander is running, so every
            // remaining plugin would fail the same way. Stop and say so.
            lockedAt = i;
            break;
        }
    }

    const skipped = lockedAt >= 0 ? queue.slice(lockedAt + 1) : [];
    const rows = [
        ...done.map((p) => summaryRow('✓', 'ua-ok', `${p.name} — ${p.latest_version}`)),
        ...failed.map((p) => summaryRow('✗', 'ua-err', p.name)),
        ...skipped.map((p) => summaryRow('—', 'ua-skip', p.name)),
    ];
    // Head line first, then one line per plugin: the count is what the user
    // scans for, the rows are what they act on.
    let html = `<p class="ua-head">${escapeHtml(
        done.length ? tp('update_all.updated', done.length) : t('update_all.none'),
    )}</p>`;
    html += `<div class="ua-summary">${rows.join('')}</div>`;
    if (lockedAt >= 0) {
        html += `<p class="ua-note">${escapeHtml(t('update_all.locked_stop', { name: queue[lockedAt].name }))}</p>`;
    }
    if (rawErrors.length) {
        html +=
            '<details class="dlg-tech"><summary>' +
            escapeHtml(t('dialog.tech_details')) +
            '</summary><div class="dlg-tech-body">' +
            linkify(escapeHtml(rawErrors.join('\n'))) +
            '</div></details>';
    }
    finishProgress(html);

    await new Promise((resolve) => {
        progressResolve = resolve;
    });
    if (done.length) runScan(false);
}

async function openHistory() {
    try {
        const entries = await GetUpdateHistory();
        if (!entries || entries.length === 0) {
            historyBody.innerHTML = `<tr class="table-empty"><td colspan="3">${t('history.empty')}</td></tr>`;
        } else {
            // Dates follow the UI language, not the OS locale.
            const fmt = new Intl.DateTimeFormat(currentLang() === 'ru' ? 'ru-RU' : 'en-GB', {
                dateStyle: 'medium',
                timeStyle: 'short',
            });
            historyBody.innerHTML = entries
                .map(
                    (e) => `
                <tr>
                    <td>${escapeHtml(e.plugin_name || e.plugin_id)}</td>
                    <td><code>${escapeHtml(e.version)}</code></td>
                    <td>${fmt.format(new Date(e.applied_at))}</td>
                </tr>
            `,
                )
                .join('');
        }
        historyDialog.showModal();
        document.getElementById('history-close').focus();
    } catch (err) {
        openAppDialog(errorDialogHtml(err, t('history.error_friendly')));
    }
}

// About panel state. The Go bridge owns the version and the data paths; the
// release check only runs when the user presses the button, so `appUpdate`
// stays null until then. Both survive the language-switch DOM rebuild.
let appInfo = null;
let appUpdate = null;
let appUpdateBusy = false;

// The build stamps a four-part version because the Windows PE version resource
// demands a number; the trailing ".0" carries no meaning for a person reading
// the About panel, so it is dropped here and nowhere else.
function formatDisplayVersion(v) {
    if (!v) return '';
    const m = v.match(/^(\d+\.\d+\.\d+)\.0$/);
    return m ? m[1] : v;
}

function renderAbout() {
    const nameEl = document.getElementById('about-version');
    if (!nameEl) return;
    nameEl.textContent = formatDisplayVersion(appInfo?.version);
    const dataDirEl = document.getElementById('about-data-dir');
    dataDirEl.textContent = appInfo?.data_dir ?? '';
    document.getElementById('about-build').textContent = appInfo?.go_version ?? '';
    renderAboutUpdate();
}

// One status line plus the button that produced it. The available case carries
// the release link too, because that is the whole point of the check.
function renderAboutUpdate() {
    const host = document.getElementById('about-update');
    if (!host) return;

    let status = '';
    if (appUpdateBusy) {
        status = `<span class="about-status">${t('about.checking')}</span>`;
    } else if (appUpdate) {
        const fmt = new Intl.DateTimeFormat(currentLang() === 'ru' ? 'ru-RU' : 'en-GB', { timeStyle: 'short' });
        const when = `<span class="about-checked">${escapeHtml(t('about.checked_at', { time: fmt.format(new Date(appUpdate.checked_at)) }))}</span>`;
        if (!appUpdate.comparable) {
            status = `<span class="about-status">${escapeHtml(t('about.dev_build'))}</span>${when}`;
        } else if (appUpdate.available) {
            status = `<span class="about-status accent">${escapeHtml(t('about.available', { latest: appUpdate.latest }))}</span>
                <a href="${escapeHtml(appUpdate.release_url || '')}" class="dlg-link">${escapeHtml(t('about.open_release'))}</a>${when}`;
        } else {
            status = `<span class="about-status">${escapeHtml(t('about.up_to_date'))}</span>${when}`;
        }
    }

    // The label stays put while a check runs: the status line already says
    // "checking", and a disabled button only has to stop a second request.
    const button = `<button type="button" class="btn-secondary" id="btn-app-update" ${appUpdateBusy ? 'disabled' : ''}>${escapeHtml(t('about.check'))}</button>`;
    host.innerHTML = `<div class="about-update-row">${button}</div>${status}`;
}

async function checkForAppUpdate() {
    if (appUpdateBusy) return;
    appUpdateBusy = true;
    renderAboutUpdate();
    try {
        appUpdate = await CheckForAppUpdate();
    } catch (err) {
        // Same shape as every other failure in the app: a friendly line, with
        // the raw reason available underneath.
        openAppDialog(errorDialogHtml(err, t('about.check.error')));
    } finally {
        appUpdateBusy = false;
        renderAboutUpdate();
    }
}

// Which wincmd.ini a scan opens is the Go side's decision, so the panel shows
// its answer instead of a path the frontend invented. `tcError` is a rejected
// edit: while it is set the field keeps the user's own text, because replacing
// it with the stored value would hide what they need to fix.
let tcConfig = null;
let tcError = '';

function renderTcPanel() {
    const pathEl = document.getElementById('tc-ini');
    const host = document.getElementById('tc-state');
    if (!pathEl || !host) return;

    if (!tcError && tcConfig) pathEl.value = tcConfig.effective;

    let line = '';
    if (tcError) {
        line = `<span class="tc-warn">${escapeHtml(tcError)}</span>`;
    } else if (tcConfig) {
        if (!tcConfig.effective) {
            line = `<span class="tc-warn">${escapeHtml(t('settings.tc.none'))}</span>`;
        } else if (!tcConfig.readable) {
            line = `<span class="tc-warn">${escapeHtml(t('settings.tc.gone'))}</span>`;
        } else {
            const key = tcConfig.configured ? 'settings.tc.mode.manual' : 'settings.tc.mode.auto';
            line = `<span class="tc-mode">${escapeHtml(t(key))}</span>`;
        }
    }
    // The way out of a manual path is offered wherever one is stored, healthy or
    // not: it is the only control that returns the panel to detection.
    const back = tcConfig?.configured
        ? `<button type="button" class="btn-secondary" id="tc-auto">${escapeHtml(t('settings.tc.back_auto'))}</button>`
        : '';
    host.innerHTML = line + back;
}

async function loadTcConfig() {
    try {
        tcConfig = await GetTcConfig();
        tcError = '';
    } catch (err) {
        // The panel has no other use for this call, so the reason is all that
        // can be shown.
        tcError = String(err?.message ?? err);
        tcConfig = null;
    }
    renderTcPanel();
}

// Store a path and re-read the plugins, because the table the user is looking
// at describes the Total Commander that was just replaced.
async function applyTcPath(path) {
    const trimmed = String(path ?? '').trim();
    tcError = '';
    // Retyping the path that is already in force is not an edit, but it does
    // retire a rejected one: the field goes back to what is actually stored.
    if (tcConfig && trimmed === tcConfig.effective) {
        renderTcPanel();
        return;
    }
    try {
        await SetTcConfig(trimmed);
    } catch (err) {
        // The reason plus the fact that nothing was stored: the field still
        // shows the rejected text, so the panel has to say it is not in effect.
        tcError = `${String(err?.message ?? err)} — ${t('settings.tc.not_saved')}`;
        renderTcPanel();
        return;
    }
    await loadTcConfig();
    runScan(true);
}

async function browseTcIni() {
    try {
        // An empty result is the user cancelling the picker, not a failure.
        const picked = await BrowseTcIni(t('settings.tc.browse_title'));
        if (!picked) return;
        const pathEl = document.getElementById('tc-ini');
        if (pathEl) pathEl.value = picked;
        await applyTcPath(picked);
    } catch (err) {
        tcError = String(err?.message ?? err);
        renderTcPanel();
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
    // The DOM rebuilds on a language switch, which empties the path field; this
    // is what puts the stored answer back into it.
    renderTcPanel();
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
    if (settingsSection === 'tc') loadTcConfig();
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
            <button type="button" data-panel="tc">${t('settings.nav.tc')}</button>
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
          <div class="settings-panel" id="panel-tc">
            <h3>${t('settings.nav.tc')}</h3>
            <p class="section-desc">${t('settings.tc')}</p>
            <div class="section-label">${t('settings.tc.ini')}</div>
            <div class="tc-row">
              <input type="text" id="tc-ini" aria-label="${t('settings.tc.ini')}" spellcheck="false" autocomplete="off">
              <button type="button" class="btn-secondary" id="tc-browse">${t('settings.tc.browse')}</button>
            </div>
            <div class="tc-state" id="tc-state"></div>
            <p class="editor-hint">${t('settings.tc.hint')}</p>
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
            <div class="about-name">TotalPlug <span class="about-version" id="about-version"></span></div>
            <p class="about-desc">${t('about.desc')}</p>
            <div class="about-update" id="about-update"></div>
            <div class="about-links">
              <button type="button" class="btn-secondary" id="link-catalog">${t('about.catalog')}</button>
              <button type="button" class="btn-secondary" id="link-releases">${t('about.releases')}</button>
            </div>
            <p class="editor-hint about-privacy">${t('about.privacy')}</p>
            <details class="about-details">
              <summary>${t('about.details')}</summary>
              <div class="about-tech">
                <div class="k">${t('about.data_dir')}</div>
                <div class="v"><code class="path" id="about-data-dir"></code></div>
                <div class="k">${t('about.build')}</div>
                <div class="v" id="about-build"></div>
              </div>
            </details>
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
    updateAllEl = document.getElementById('update-all');
    updateAllCountEl = document.getElementById('update-all-count');
    btnUpdateAll = document.getElementById('btn-update-all');
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
    btnUpdateAll.addEventListener('click', handleUpdateAll);
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
        if (tr) openCard(Number(tr.dataset.idx), tr);
    });

    appDialogOk.addEventListener('click', () => {
        settleAppDialog(true);
        appDialog.close();
    });
    appDialogCancel.addEventListener('click', () => {
        settleAppDialog(false);
        appDialog.close();
    });
    document.getElementById('app-dialog-close').addEventListener('click', () => {
        settleAppDialog(false);
        appDialog.close();
    });
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
    progressOk.addEventListener('click', () => {
        settleProgress();
        progressDialog.close();
    });
    progressClose.addEventListener('click', () => {
        settleProgress();
        progressDialog.close();
    });
    progressDialog.addEventListener('cancel', (e) => {
        if (progressRunning) e.preventDefault();
    });
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
            // Detection can move while the app runs (a portable TC mounted, an
            // install uninstalled), and asking is two stat calls, so the panel
            // is refreshed on entry rather than trusted from the last visit.
            if (settingsSection === 'tc') loadTcConfig();
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
    // The path saves on the same trigger the rest of the settings use — leaving
    // the field — and Enter counts as leaving it, because typing an absolute
    // Windows path and pressing Enter is how everyone expects that to work.
    const tcPathEl = document.getElementById('tc-ini');
    tcPathEl.addEventListener('change', () => applyTcPath(tcPathEl.value));
    tcPathEl.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            applyTcPath(tcPathEl.value);
        }
    });
    document.getElementById('tc-browse').addEventListener('click', browseTcIni);
    // The state line is rebuilt as HTML, so its button is reached the way the
    // About panel reaches its own row.
    document.getElementById('tc-state').addEventListener('click', (e) => {
        if (e.target.closest('#tc-auto')) applyTcPath('');
    });
    // The update row is re-rendered as HTML, so its button and release link are
    // reached through one delegated listener instead of per-render bindings.
    const aboutUpdateEl = document.getElementById('about-update');
    aboutUpdateEl.addEventListener('click', (e) => {
        const link = e.target.closest('a.dlg-link');
        if (link) {
            e.preventDefault();
            BrowserOpenURL(link.href);
            return;
        }
        if (e.target.closest('#btn-app-update')) checkForAppUpdate();
    });
    document.getElementById('link-releases').addEventListener('click', () => {
        if (appInfo) BrowserOpenURL(appInfo.releases_url);
    });
    // Re-render the state-backed parts of the panel after the DOM rebuild.
    renderAbout();

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
    editorCancelEl.addEventListener('click', closeCard);
    // Esc is not one of our buttons, so the close event is still what hands the
    // keyboard back for it; the paths above have already cleared the opener.
    cardDialog.addEventListener('close', returnCardFocus);
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

// The About panel needs only local values, so ask for them once up front
// instead of when the panel opens.
AppInfo().then((info) => {
    appInfo = info;
    renderAbout();
});

// Startup: cache-only scan, no network wait before the window is useful
runScan(false);
