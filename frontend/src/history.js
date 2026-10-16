import { GetUpdateHistory, RollbackUpdate } from '../wailsjs/go/gui/App';
import { t, currentLang } from './i18n.js';
import { dom } from './dom.js';
import { runScan } from './catalog.js';
import { escapeHtml, formatStamp } from './text.js';
import { openAppDialog, errorDialogHtml } from './dialog.js';

// Update history: what the updater actually did, newest data first as the Go
// side stores it. A line whose kept copy is still on disk can put that copy
// back, which is the only action a past update takes here.

let entries = [];
// Which line is being decided on, and why the last attempt did not work.
let pending = -1;
let failure = '';
let busy = false;

export const historyMarkup = () => `
      <dialog id="history-dialog">
        <h2>${t('history.title')}</h2>
        <table>
          <thead>
            <tr>
              <th>${t('history.col.plugin')}</th>
              <th>${t('history.col.version')}</th>
              <th>${t('history.col.date')}</th>
              <th>${t('history.col.action')}</th>
            </tr>
          </thead>
          <tbody id="history-body"></tbody>
        </table>
        <div class="editor-actions">
          <button type="button" id="history-close">${t('dialog.close')}</button>
        </div>
      </dialog>`;

const stampFormatter = () =>
    new Intl.DateTimeFormat(currentLang() === 'ru' ? 'ru-RU' : 'en-GB', {
        dateStyle: 'medium',
        timeStyle: 'short',
    });

// Dates follow the UI language, not the OS locale.
function rowMarkup(entry, idx, fmt) {
    if (idx === pending) return confirmMarkup(entry, idx);
    const rollback = entry.backup_path
        ? `<button type="button" class="btn-update" data-rollback="${idx}">${t('history.rollback')}</button>`
        : '';
    const kind = entry.restored ? `<span class="history-kind">${t('history.restored')}</span>` : '';
    return `
                <tr>
                    <td>${escapeHtml(entry.plugin_name || entry.plugin_id)}</td>
                    <td><code>${escapeHtml(entry.version)}</code>${kind}</td>
                    <td>${escapeHtml(formatStamp(fmt, entry.applied_at))}</td>
                    <td class="history-action">${rollback}</td>
                </tr>`;
}

// The confirm sits in the row it is about. A second dialog over this one would
// cover the line the user is deciding on.
function confirmMarkup(entry, idx) {
    const question = entry.replaced_version
        ? t('history.confirm', { version: entry.replaced_version, current: entry.version })
        : t('history.confirm_unknown');
    const note = failure || t('history.confirm_note');
    return `
                <tr class="history-confirm">
                    <td colspan="4">
                        <p>${escapeHtml(question)}</p>
                        <p class="history-note${failure ? ' history-failed' : ''}">${escapeHtml(note)}</p>
                        <div class="editor-actions">
                            <button type="button" class="btn-update" data-confirm="${idx}"${busy ? ' disabled' : ''}>${t('history.rollback')}</button>
                            <button type="button" data-cancel${busy ? ' disabled' : ''}>${t('dialog.cancel')}</button>
                        </div>
                    </td>
                </tr>`;
}

function render() {
    if (entries.length === 0) {
        dom.historyBody.innerHTML = `<tr class="table-empty"><td colspan="4">${t('history.empty')}</td></tr>`;
        return;
    }
    const fmt = stampFormatter();
    dom.historyBody.innerHTML = entries.map((entry, idx) => rowMarkup(entry, idx, fmt)).join('');
    // The keyboard should land on the action it was offered, not on the row it
    // just left behind.
    dom.historyBody.querySelector('button[data-confirm]')?.focus();
}

async function reload() {
    entries = (await GetUpdateHistory()) || [];
    render();
}

async function confirmRollback(idx) {
    const entry = entries[idx];
    if (!entry || busy) return;
    busy = true;
    render();
    try {
        const result = await RollbackUpdate(entry);
        if (result?.locked) {
            failure = t('history.locked');
            return;
        }
        pending = -1;
        failure = '';
        await reload();
        // The plugin table below shows the version this changed, so it has to be
        // read again — the same refresh an update ends with.
        runScan(false);
    } catch (err) {
        failure = String(err?.message ?? err);
        render();
    } finally {
        busy = false;
        if (pending >= 0) render();
    }
}

export async function openHistory() {
    try {
        pending = -1;
        failure = '';
        await reload();
        dom.historyDialog.showModal();
        dom.historyClose.focus();
    } catch (err) {
        openAppDialog(errorDialogHtml(err, t('history.error_friendly')));
    }
}

export function bindHistory() {
    dom.historyClose.addEventListener('click', () => dom.historyDialog.close());
    dom.historyBody.addEventListener('click', (e) => {
        const offer = e.target.closest('button[data-rollback]');
        if (offer) {
            pending = Number(offer.dataset.rollback);
            failure = '';
            render();
            return;
        }
        if (e.target.closest('button[data-cancel]')) {
            pending = -1;
            failure = '';
            render();
            return;
        }
        const confirm = e.target.closest('button[data-confirm]');
        if (confirm) confirmRollback(Number(confirm.dataset.confirm));
    });
}
