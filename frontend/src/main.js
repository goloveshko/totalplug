import './style.css';
import { ScanPlugins, UpdatePlugin, SaveOverride, GetUpdateHistory, GetWindowEffect, SetWindowEffect } from '../wailsjs/go/main/App';
import { EventsOn, BrowserOpenURL, WindowMinimise, WindowToggleMaximise, Quit } from '../wailsjs/runtime';
import { t, currentLang, setLang } from './i18n.js';
import { currentTheme, setTheme, currentScale, setScale } from './theme.js';

const app = document.getElementById('app');

let currentPlugins = [];
let editingPlugin = null;

// Reassigned by buildDom() on every (re)render, e.g. after a language switch.
let statusEl, tbody, btnScan, btnRefresh, searchEl;
let cardDialog, cardNameEl, cardSubEl, cardInfoEl;
let editorForm, editorVersionEl, editorSourceEl, editorCancelEl;
let historyDialog, historyBody;
let appDialog, appDialogMsg, appDialogOk, appDialogCancel;
let mainView, settingsView, langSelect, fontScaleEl, fontScaleValueEl, effectSelect;
let dialogResolve = null;

// Backdrop chosen in Settings; applied by Go at startup, so the UI only shows
// it and warns that a restart is needed.
let effectChoice = 'none';

// Case-insensitive name filter from the toolbar search box; survives the DOM
// rebuild on language switch, like the settings state below.
let searchQuery = '';

// Settings is a full-window view; these survive the DOM rebuild that a
// language switch triggers so the user stays where they were.
let settingsOpen = false;
let settingsSection = 'appearance';

function setStatus(message) {
    statusEl.innerText = message;
    // The bar is a single ellipsized line; the tooltip keeps long messages readable.
    statusEl.title = message;
}

// In-app replacement for native confirm()/alert(): those render with the
// "wails.localhost" origin header and cannot be styled.
function openAppDialog(message, isConfirm) {
    return new Promise((resolve) => {
        dialogResolve = resolve;
        appDialogMsg.textContent = message;
        appDialogCancel.style.display = isConfirm ? '' : 'none';
        appDialog.showModal();
    });
}
const showConfirm = (message) => openAppDialog(message, true);
const showAlert = (message) => openAppDialog(message, false);

function settleAppDialog(result) {
    if (dialogResolve) {
        dialogResolve(result);
        dialogResolve = null;
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
        setStatus(forceRefresh ? t('scan.fresh') : t('scan.cached'));

        currentPlugins = await ScanPlugins(forceRefresh);

        if (!currentPlugins || currentPlugins.length === 0) {
            tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; padding:20px;">${t('table.none')}</td></tr>`;
            setStatus(t('scan.none'));
            return;
        }

        renderTable();
        setStatus(t('status.found', { n: currentPlugins.length }));
    } catch (err) {
        setStatus(t('status.error', { err }));
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
        (plugin.custom_source ? cardRow(t('editor.source'), `<code class="path">${plugin.custom_source}</code>`) : '') +
        cardRow(t('card.path'), `<code class="path">${plugin.path}</code>`);

    const auto = plugin.installed_version || '';
    editorVersionEl.value = /^(unknown|n\/a)/i.test(auto) ? '' : auto;
    editorSourceEl.value = plugin.custom_source || '';

    cardDialog.showModal();
}

async function saveOverrideFromEditor() {
    if (!editingPlugin) return;

    const override = {
        custom_version: editorVersionEl.value.trim(),
        custom_source: editorSourceEl.value.trim(),
    };

    editorSaveGuard(true);
    try {
        await SaveOverride(editingPlugin.id, override);
        cardDialog.close();
        setStatus(t('editor.saved', { name: editingPlugin.name }));
        runScan(false);
    } catch (err) {
        setStatus(t('editor.save_error', { err }));
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

    try {
        setStatus(t('update.running', { name: plugin.name }));
        const result = await UpdatePlugin(plugin);

        if (result && result.locked) {
            setStatus(t('update.locked', { name: plugin.name }));
            return;
        }

        await showAlert(result && result.backup_path
            ? t('update.done_backup', { name: plugin.name, path: result.backup_path })
            : t('update.done', { name: plugin.name }));
        runScan(false);
    } catch (err) {
        await showAlert(t('update.error_alert', { err }));
        setStatus(t('update.error_status', { name: plugin.name }));
    }
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
        setStatus(t('history.error', { err }));
    }
}

function refreshSettingsState() {
    settingsView.querySelectorAll('[data-theme-choice]').forEach((el) => {
        el.classList.toggle('selected', el.dataset.themeChoice === currentTheme());
    });
    langSelect.value = currentLang();
    fontScaleEl.value = String(currentScale());
    fontScaleValueEl.textContent = currentScale() + '%';
    effectSelect.value = effectChoice;
    settingsView.querySelectorAll('.settings-nav button').forEach((btn) => {
        btn.classList.toggle('active', btn.dataset.panel === settingsSection);
    });
    settingsView.querySelectorAll('.settings-panel').forEach((panel) => {
        panel.classList.toggle('active', panel.id === 'panel-' + settingsSection);
    });
}

function openSettings() {
    settingsOpen = true;
    mainView.hidden = true;
    settingsView.hidden = false;
    refreshSettingsState();
}

function closeSettings() {
    settingsOpen = false;
    settingsView.hidden = true;
    mainView.hidden = false;
}

function buildDom() {
    app.innerHTML = `
      <div class="titlebar" id="titlebar">
        <span class="titlebar-title">TotalPlug</span>
        <div class="caption-buttons">
          <button type="button" id="win-min" title="${t('win.min')}">&#8722;</button>
          <button type="button" id="win-max" title="${t('win.max')}">&#9633;</button>
          <button type="button" id="win-close" class="win-close" title="${t('win.close')}">&#10005;</button>
        </div>
      </div>
      <div id="view-main">
        <div class="toolbar">
          <input type="search" id="plugin-search" placeholder="${t('search.ph')}" autocomplete="off">
          <div class="actions">
            <button id="btn-history" class="btn-history">${t('btn.history')}</button>
            <button id="btn-settings" class="btn-history">${t('btn.settings')}</button>
            <button id="btn-refresh" class="btn-secondary">${t('btn.refresh')}</button>
            <button id="btn-scan">${t('btn.scan')}</button>
          </div>
        </div>
        <div class="status-bar" id="status">${t('status.initial')}</div>
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
          <div class="settings-top-left">
            <button type="button" id="settings-back" class="btn-back">${t('settings.back')}</button>
            <h2>${t('settings.title')}</h2>
          </div>
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
            <div class="section-label">${t('settings.effect')}</div>
            <select id="effect-select">
              <option value="mica">${t('effect.mica')}</option>
              <option value="acrylic">${t('effect.acrylic')}</option>
              <option value="none">${t('effect.none')}</option>
            </select>
            <p class="editor-hint">${t('settings.effect.hint')}</p>
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
            <input type="text" id="editor-source" placeholder="github:Owner/Repo" autocomplete="off">
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
        <div class="dlg-header" id="app-dialog-header">TotalPlug</div>
        <div class="dlg-message" id="app-dialog-msg"></div>
        <div class="editor-actions">
          <button type="button" class="btn-save" id="app-dialog-ok">${t('dialog.ok')}</button>
          <button type="button" id="app-dialog-cancel">${t('dialog.cancel')}</button>
        </div>
      </dialog>
    `;

    statusEl = document.getElementById('status');
    tbody = document.getElementById('plugins-body');
    btnScan = document.getElementById('btn-scan');
    btnRefresh = document.getElementById('btn-refresh');
    searchEl = document.getElementById('plugin-search');

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
    mainView = document.getElementById('view-main');
    settingsView = document.getElementById('view-settings');
    langSelect = document.getElementById('lang-select');
    fontScaleEl = document.getElementById('font-scale');
    fontScaleValueEl = document.getElementById('font-scale-value');
    effectSelect = document.getElementById('effect-select');

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
    // Esc or any other dismissal path must resolve a pending confirm as "no".
    appDialog.addEventListener('close', () => settleAppDialog(false));

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
    effectSelect.addEventListener('change', async () => {
        try {
            await SetWindowEffect(effectSelect.value);
            effectChoice = effectSelect.value;
        } catch (err) {
            setStatus(t('settings.effect.error', { err }));
            effectSelect.value = effectChoice;
        }
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
        setStatus(t('status.found', { n: currentPlugins.length }));
    }
}

// Background catalog refresh (pushed by the Go bridge after every scan)
EventsOn('plugins:refreshed', (plugins) => {
    if (!plugins || plugins.length === 0) return;
    currentPlugins = plugins;
    renderTable();
    setStatus(t('events.refreshed', { n: currentPlugins.length }));
});

EventsOn('plugins:refresh:failed', (msg) => {
    setStatus(t('events.refresh_failed', { msg }));
});

buildDom();

// Startup: mirror the Go-side backdrop (chosen before the window existed) so
// the settings select and the transparent-body styling match reality.
GetWindowEffect()
    .then((effect) => {
        effectChoice = effect;
        if (effect !== 'none') {
            document.documentElement.dataset.effect = effect;
        } else {
            delete document.documentElement.dataset.effect;
        }
        refreshSettingsState();
    })
    .catch((err) => console.error('GetWindowEffect:', err));

// Startup: cache-only scan, no network wait before the window is useful
runScan(false);
