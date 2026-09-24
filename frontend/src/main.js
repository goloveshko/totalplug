import './style.css';
import { ScanPlugins, UpdatePlugin, SaveOverride, GetUpdateHistory } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime';
import { t, currentLang, toggleLang } from './i18n.js';

const app = document.getElementById('app');

let currentPlugins = [];
let editingPlugin = null;

// Reassigned by buildDom() on every (re)render, e.g. after a language switch.
let statusEl, tbody, btnScan, btnRefresh;
let editorDialog, editorForm, editorPluginEl, editorVersionEl, editorSourceEl, editorCancelEl;
let historyDialog, historyBody;
let appDialog, appDialogMsg, appDialogOk, appDialogCancel;
let dialogResolve = null;

// In-app replacement for native confirm()/alert(): those render with the
// "wails.localhost" origin header and cannot be styled or moved.
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

function makeDraggable(dlg, handle) {
    handle.addEventListener('pointerdown', (e) => {
        if (e.button !== 0) return;
        const rect = dlg.getBoundingClientRect();
        // Switch from the UA-centered layout to explicit coordinates once.
        dlg.style.margin = '0';
        dlg.style.left = rect.left + 'px';
        dlg.style.top = rect.top + 'px';
        const dx = e.clientX - rect.left;
        const dy = e.clientY - rect.top;
        const move = (ev) => {
            dlg.style.left = Math.max(0, Math.min(window.innerWidth - rect.width, ev.clientX - dx)) + 'px';
            dlg.style.top = Math.max(0, Math.min(window.innerHeight - 30, ev.clientY - dy)) + 'px';
        };
        const up = () => {
            handle.removeEventListener('pointermove', move);
            handle.removeEventListener('pointerup', up);
        };
        handle.addEventListener('pointermove', move);
        handle.addEventListener('pointerup', up);
    });
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
                <td><small style="color:#aaa">${p.confidence}</small></td>
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

function buildDom() {
    app.innerHTML = `
      <div class="header">
        <div>
          <h1>TotalPlug</h1>
          <div class="status-bar" id="status">${t('status.initial')}</div>
        </div>
        <div class="actions">
          <button id="btn-lang" class="btn-lang" title="${t('btn.lang.title')}">${t('btn.lang')}</button>
          <button id="btn-history" class="btn-history">${t('btn.history')}</button>
          <button id="btn-scan">${t('btn.scan')}</button>
          <button id="btn-refresh" style="background:#555">${t('btn.refresh')}</button>
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

    btnScan.addEventListener('click', () => runScan(false));
    btnRefresh.addEventListener('click', () => runScan(true));
    document.getElementById('btn-history').addEventListener('click', openHistory);

    makeDraggable(appDialog, document.getElementById('app-dialog-header'));
    makeDraggable(editorDialog, editorDialog.querySelector('h2'));
    makeDraggable(historyDialog, historyDialog.querySelector('h2'));
    appDialogOk.addEventListener('click', () => { settleAppDialog(true); appDialog.close(); });
    appDialogCancel.addEventListener('click', () => { settleAppDialog(false); appDialog.close(); });
    // Esc or any other dismissal path must resolve a pending confirm as "no".
    appDialog.addEventListener('close', () => settleAppDialog(false));
    document.getElementById('btn-lang').addEventListener('click', () => {
        toggleLang();
        buildDom();
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

// Фоновое обновление каталога (запускается Go-мостом после каждого скана)
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

// Автозапуск кэш-скана при старте окна (без сетевого ожидания)
runScan(false);
