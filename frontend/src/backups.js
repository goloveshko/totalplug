import { BackupLocation, GetBackupPolicy, SetBackupPolicy } from '../wailsjs/go/gui/App';
import { t } from './i18n.js';
import { escapeHtml } from './text.js';

// Settings > Backups. How long a pre-update copy of a plugin is kept is stored in
// the settings file, because a command-line update honours the same answer, so
// this panel edits that file through the Go side instead of keeping a copy of the
// numbers here.

let policy = null;
let backupSpace = '';
let backupError = '';

export const backupMarkup = () => `
          <div class="settings-panel" id="panel-backups">
            <h3>${t('settings.nav.backups')}</h3>
            <p class="section-desc">${t('settings.backups')}</p>
            <div class="section-label">${t('settings.backups.keep')}</div>
            <input type="number" id="backup-keep" min="0" max="99" step="1" aria-label="${t('settings.backups.keep')}">
            <div class="section-label">${t('settings.backups.days')}</div>
            <input type="number" id="backup-days" min="0" max="3650" step="1" aria-label="${t('settings.backups.days')}">
            <p class="editor-hint">${t('settings.backups.hint')}</p>
            <div class="section-label">${t('settings.backups.location')}</div>
            <code class="backup-path" id="backup-path"></code>
            <div class="tc-state" id="backup-state"></div>
          </div>`;

export function renderBackupPanel() {
    const keepEl = document.getElementById('backup-keep');
    const daysEl = document.getElementById('backup-days');
    const pathEl = document.getElementById('backup-path');
    const host = document.getElementById('backup-state');
    if (!keepEl || !daysEl || !pathEl || !host) return;

    // A rejected edit keeps the user's own numbers: replacing them with what is
    // stored would hide the value they have to fix.
    if (!backupError && policy) {
        keepEl.value = String(policy.keep_newest);
        daysEl.value = String(policy.max_age_days);
    }
    pathEl.textContent = backupSpace;
    host.innerHTML = backupError
        ? `<span class="tc-warn">${escapeHtml(`${backupError} — ${t('settings.backups.not_saved')}`)}</span>`
        : '';
}

export async function loadBackupSettings() {
    try {
        const [stored, where] = await Promise.all([GetBackupPolicy(), BackupLocation()]);
        policy = { keep_newest: stored?.keep_newest ?? 0, max_age_days: stored?.max_age_days ?? 0 };
        backupSpace = where ?? '';
        backupError = '';
    } catch (err) {
        backupError = String(err?.message ?? err);
    }
    renderBackupPanel();
}

// Both numbers travel together: a policy is one answer, and storing half of it
// would leave the other half from an edit the user can no longer see.
async function applyBackupPolicy() {
    const keepEl = document.getElementById('backup-keep');
    const daysEl = document.getElementById('backup-days');
    if (!keepEl || !daysEl) return;

    const keep = Number(keepEl.value);
    const days = Number(daysEl.value);
    backupError = '';
    if (!Number.isInteger(keep) || !Number.isInteger(days)) {
        // Neither a number nor a blank can be stored, so the fields go back to
        // what is actually in force.
        if (policy) {
            keepEl.value = String(policy.keep_newest);
            daysEl.value = String(policy.max_age_days);
        }
        renderBackupPanel();
        return;
    }

    const next = { keep_newest: Math.max(0, keep), max_age_days: Math.max(0, days) };
    if (policy && next.keep_newest === policy.keep_newest && next.max_age_days === policy.max_age_days) {
        renderBackupPanel();
        return;
    }
    try {
        await SetBackupPolicy(next);
    } catch (err) {
        backupError = String(err?.message ?? err);
        renderBackupPanel();
        return;
    }
    policy = next;
    renderBackupPanel();
}

export function bindBackups() {
    for (const id of ['backup-keep', 'backup-days']) {
        const el = document.getElementById(id);
        if (!el) continue;
        el.addEventListener('change', applyBackupPolicy);
        el.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                applyBackupPolicy();
            }
        });
    }
}
