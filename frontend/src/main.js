import './style.css';
import { ScanPlugins, UpdatePlugin, SaveOverride, GetUpdateHistory } from '../wailsjs/go/main/App';
import { EventsOn, BrowserOpenURL } from '../wailsjs/runtime';
import { t, currentLang, setLang } from './i18n.js';
import { currentTheme, setTheme, currentScale, setScale } from './theme.js';

const app = document.getElementById('app');

let currentPlugins = [];
let editingPlugin = null;

// Reassigned by buildDom() on every (re)render, e.g. after a language switch.
let statusEl, tbody, btnScan, btnRefresh;
let editorDialog, editorForm, editorPluginEl, editorVersionEl, editorSourceEl, editorCancelEl;
let historyDialog, historyBody;
let appDialog, appDialogMsg, appDialogOk, appDialogCancel;
let settingsDialog, fontScaleEl, fontScaleValueEl;
let dialogResolve = null;

// Settings keeps no server-side state, so the last opened section survives
// the DOM rebuild that a language switch triggers.
let settingsSection = 'appearance';

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
        statusEl.innerText = forceRefresh ? t('scan.fresh') : t('scan.cached');

        currentPlugins = await ScanPlugins(forceRefresh);

        if (!currentPlugins || currentPlugins.length === 0) {
            tbody.innerHTML = `<tr><td colspan="7" style="text-align:center; padding:20px;">${t('table.none')}</td></tr>`;
            statusEl.innerText = t('scan.none');
            return;
        }

        renderTable(currentPlugins);
        statusEl.innerText = t('status.found', { n: currentPlugins.length });
    } catch (err) {
        statusEl.innerText = t('status.error', { err });
        console.error(err);
    } finally {
        btnScan.disabled = false;
        btnRefresh.disabled = false;
    }
}

function renderTable(plugins) {
    tbody.innerHTML = plugins.map((p, idx) => {
        const canUpdate = p.status === 'update_avail' && p.download_url;
        return `
            <tr>
                <td><strong>${p.type}</strong></td>
                <td>${p.name}</td>
                <td><code>${p.installed_version}</code></td>
                <td><small style="color:var(--text-muted)">${p.confidence}</small></td>
                <td><code>${p.latest_version || '-'}</code></td>
                <td>${getStatusBadge(p.status)}</td>
                <td>
                    ${canUpdate ? `<button class="btn-update" onclick="handleUpdate(${idx})">${t('btn.update')}</button>` : ''}
                    <button class="btn-edit" title="${t('btn.edit.title')}" onclick="handleEdit(${idx})">${t('btn.edit')}</button>
                </td>
            </tr>
        `;
    }).join('');
}

window.handleEdit = function(index) {
    const plugin = currentPlugins[index];
    if (!plugin) return;

    editingPlugin = plugin;
    editorPluginEl.textContent = `${plugin.name} (${plugin.type})`;

    const auto = plugin.installed_version || '';
    editorVersionEl.value = /^(unknown|n\/a)/i.test(auto) ? '' : auto;
    editorSourceEl.value = plugin.custom_source || '';

    editorDialog.showModal();
};

async function saveOverrideFromEditor() {
    if (!editingPlugin) return;

    const override = {
        custom_version: editorVersionEl.value.trim(),
        custom_source: editorSourceEl.value.trim(),
    };

    editorSaveGuard(true);
    try {
        await SaveOverride(editingPlugin.id, override);
        editorDialog.close();
        statusEl.innerText = t('editor.saved', { name: editingPlugin.name });
        runScan(false);
    } catch (err) {
        statusEl.innerText = t('editor.save_error', { err });
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
        statusEl.innerText = t('update.running', { name: plugin.name });
        const result = await UpdatePlugin(plugin);

        if (result && result.locked) {
            statusEl.innerText = t('update.locked', { name: plugin.name });
            return;
        }

        await showAlert(result && result.backup_path
            ? t('update.done_backup', { name: plugin.name, path: result.backup_path })
            : t('update.done', { name: plugin.name }));
        runScan(false);
    } catch (err) {
        await showAlert(t('update.error_alert', { err }));
        statusEl.innerText = t('update.error_status', { name: plugin.name });
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
        statusEl.innerText = t('history.error', { err });
    }
}

function refreshSettingsState() {
    settingsDialog.querySelectorAll('[data-theme-choice]').forEach((el) => {
        el.classList.toggle('selected', el.dataset.themeChoice === currentTheme());
    });
    settingsDialog.querySelectorAll('[data-lang-choice]').forEach((el) => {
        el.classList.toggle('selected', el.dataset.langChoice === currentLang());
    });
    fontScaleEl.value = String(currentScale());
    fontScaleValueEl.textContent = currentScale() + '%';
    settingsDialog.querySelectorAll('.settings-nav button').forEach((btn) => {
        btn.classList.toggle('active', btn.dataset.panel === settingsSection);
    });
    settingsDialog.querySelectorAll('.settings-panel').forEach((panel) => {
        panel.classList.toggle('active', panel.id === 'panel-' + settingsSection);
    });
}

function openSettings() {
    refreshSettingsState();
    settingsDialog.showModal();
}

function buildDom() {
    app.innerHTML = `
      <div class="header">
        <div>
          <h1>TotalPlug</h1>
          <div class="status-bar" id="status">${t('status.initial')}</div>
        </div>
        <div class="actions">
          <button id="btn-history" class="btn-history">${t('btn.history')}</button>
          <button id="btn-settings" class="btn-history">${t('btn.settings')}</button>
          <button id="btn-scan">${t('btn.scan')}</button>
          <button id="btn-refresh" class="btn-secondary">${t('btn.refresh')}</button>
        </div>
      </div>
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
            <tr><td colspan="7" style="text-align:center; color:#777; padding: 24px;">${t('table.empty')}</td></tr>
          </tbody>
        </table>
      </div>

      <dialog id="version-editor">
        <form method="dialog" id="editor-form">
          <h2>${t('editor.title')}</h2>
          <div class="editor-plugin" id="editor-plugin"></div>
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

      <dialog id="settings-dialog">
        <div class="settings-header">
          <h2>${t('settings.title')}</h2>
          <span class="settings-save-hint">${t('settings.save_hint')}</span>
          <button type="button" class="btn-secondary" id="settings-close" title="${t('settings.close')}">✕</button>
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
            <div class="lang-cards">
              <div class="theme-card" data-lang-choice="en" role="button" tabindex="0">
                <div class="name">${t('settings.lang.en')}</div>
              </div>
              <div class="theme-card" data-lang-choice="ru" role="button" tabindex="0">
                <div class="name">${t('settings.lang.ru')}</div>
              </div>
            </div>
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
      </dialog>
    `;

    statusEl = document.getElementById('status');
    tbody = document.getElementById('plugins-body');
    btnScan = document.getElementById('btn-scan');
    btnRefresh = document.getElementById('btn-refresh');

    editorDialog = document.getElementById('version-editor');
    editorForm = document.getElementById('editor-form');
    editorPluginEl = document.getElementById('editor-plugin');
    editorVersionEl = document.getElementById('editor-version');
    editorSourceEl = document.getElementById('editor-source');
    editorCancelEl = document.getElementById('editor-cancel');
    historyDialog = document.getElementById('history-dialog');
    historyBody = document.getElementById('history-body');
    appDialog = document.getElementById('app-dialog');
    appDialogMsg = document.getElementById('app-dialog-msg');
    appDialogOk = document.getElementById('app-dialog-ok');
    appDialogCancel = document.getElementById('app-dialog-cancel');
    settingsDialog = document.getElementById('settings-dialog');
    fontScaleEl = document.getElementById('font-scale');
    fontScaleValueEl = document.getElementById('font-scale-value');

    btnScan.addEventListener('click', () => runScan(false));
    btnRefresh.addEventListener('click', () => runScan(true));
    document.getElementById('btn-history').addEventListener('click', openHistory);
    document.getElementById('btn-settings').addEventListener('click', openSettings);

    appDialogOk.addEventListener('click', () => { settleAppDialog(true); appDialog.close(); });
    appDialogCancel.addEventListener('click', () => { settleAppDialog(false); appDialog.close(); });
    // Esc or any other dismissal path must resolve a pending confirm as "no".
    appDialog.addEventListener('close', () => settleAppDialog(false));

    document.getElementById('settings-close').addEventListener('click', () => settingsDialog.close());
    settingsDialog.querySelectorAll('.settings-nav button').forEach((btn) => {
        btn.addEventListener('click', () => {
            settingsSection = btn.dataset.panel;
            refreshSettingsState();
        });
    });
    settingsDialog.querySelectorAll('[data-theme-choice]').forEach((card) => {
        card.addEventListener('click', () => {
            setTheme(card.dataset.themeChoice);
            refreshSettingsState();
        });
    });
    fontScaleEl.addEventListener('input', () => {
        setScale(Number(fontScaleEl.value));
        fontScaleValueEl.textContent = currentScale() + '%';
    });
    settingsDialog.querySelectorAll('[data-lang-choice]').forEach((card) => {
        card.addEventListener('click', () => {
            setLang(card.dataset.langChoice);
            // buildDom() replaces the whole tree (open dialogs included),
            // so reopen settings to keep the user where they were.
            buildDom();
            openSettings();
        });
    });
    document.getElementById('link-catalog').addEventListener('click', () => {
        BrowserOpenURL('https://sergey.is-a.dev/totalplug-catalog/');
    });

    editorForm.addEventListener('submit', async (e) => {
        // method="dialog" would close before the async save finishes
        e.preventDefault();
        await saveOverrideFromEditor();
    });
    editorCancelEl.addEventListener('click', () => editorDialog.close());
    document.getElementById('history-close').addEventListener('click', () => historyDialog.close());

    if (currentPlugins.length > 0) {
        renderTable(currentPlugins);
        statusEl.innerText = t('status.found', { n: currentPlugins.length });
    }
}

// Background catalog refresh (pushed by the Go bridge after every scan)
EventsOn('plugins:refreshed', (plugins) => {
    if (!plugins || plugins.length === 0) return;
    currentPlugins = plugins;
    renderTable(currentPlugins);
    statusEl.innerText = t('events.refreshed', { n: currentPlugins.length });
});

EventsOn('plugins:refresh:failed', (msg) => {
    statusEl.innerText = t('events.refresh_failed', { msg });
});

buildDom();

// Startup: cache-only scan, no network wait before the window is useful
runScan(false);
