import { GetUpdateHistory } from '../wailsjs/go/gui/App';
import { t, currentLang } from './i18n.js';
import { dom } from './dom.js';
import { escapeHtml, formatStamp } from './text.js';
import { openAppDialog, errorDialogHtml } from './dialog.js';

// Update history: what the updater actually installed, newest data first as the
// Go side stores it.

export const historyMarkup = () => `
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
      </dialog>`;

export async function openHistory() {
    try {
        const entries = await GetUpdateHistory();
        if (!entries || entries.length === 0) {
            dom.historyBody.innerHTML = `<tr class="table-empty"><td colspan="3">${t('history.empty')}</td></tr>`;
        } else {
            // Dates follow the UI language, not the OS locale.
            const fmt = new Intl.DateTimeFormat(currentLang() === 'ru' ? 'ru-RU' : 'en-GB', {
                dateStyle: 'medium',
                timeStyle: 'short',
            });
            dom.historyBody.innerHTML = entries
                .map(
                    (e) => `
                <tr>
                    <td>${escapeHtml(e.plugin_name || e.plugin_id)}</td>
                    <td><code>${escapeHtml(e.version)}</code></td>
                    <td>${escapeHtml(formatStamp(fmt, e.applied_at))}</td>
                </tr>
            `,
                )
                .join('');
        }
        dom.historyDialog.showModal();
        dom.historyClose.focus();
    } catch (err) {
        openAppDialog(errorDialogHtml(err, t('history.error_friendly')));
    }
}

export function bindHistory() {
    dom.historyClose.addEventListener('click', () => dom.historyDialog.close());
}
