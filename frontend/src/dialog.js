import { writable } from 'svelte/store';
import { t } from './i18n.js';
import { escapeHtml, firstHttpUrl, linkify } from './text.js';

// The in-app replacement for native confirm()/alert(): those render with the
// "wails" origin header and cannot be styled. One modal dialog serves both the
// yes/no confirmations and the error messages, and it resolves as a promise.
// Dialog.svelte draws it; this module owns the answer, because the caller of
// showConfirm() waits on a promise and the dialog cannot hand one back from a
// component.

// What the dialog shows and whether it asks a question. `open` is the only way
// in or out of the top layer: a <dialog> has to be asked with showModal().
export const dialogState = writable({ open: false, html: '', isConfirm: false });

// Overlapping requests must preserve each answer and the original opener.
let active = null;
const queue = [];
let opener = null;

function showNextDialog() {
    active = queue.shift() ?? null;
    if (active) {
        dialogState.set({ open: true, html: active.html, isConfirm: active.isConfirm });
    } else {
        dialogState.update((state) => (state.open ? { ...state, open: false } : state));
    }
}

// Raw Go errors are technical English; show the localized friendly sentence
// with a named link and hide the exact error behind a disclosure. The link
// label depends on where the URL came from (plugin page vs catalog).
export function errorDialogHtml(err, friendlyMessage, linkLabel) {
    const raw = String(err?.message ?? err);
    const url = firstHttpUrl(raw);
    const label = linkLabel || t('dialog.plugin_page');
    const link = url ? `\n<a href="${escapeHtml(url)}" class="dlg-link">${escapeHtml(label)}</a>` : '';
    let html = escapeHtml(friendlyMessage) + link;
    html +=
        '<details class="dlg-tech"><summary>' +
        escapeHtml(t('dialog.tech_details')) +
        '</summary><div class="dlg-tech-body">' +
        linkify(escapeHtml(raw)) +
        '</div></details>';
    return html;
}

export function openAppDialog(html, isConfirm) {
    return new Promise((done) => {
        queue.push({ html, isConfirm: Boolean(isConfirm), done });
        if (!active) {
            opener ??= document.activeElement;
            showNextDialog();
        }
    });
}

const textToDialogHtml = (message) => linkify(escapeHtml(message));

export const showConfirm = (message) => openAppDialog(textToDialogHtml(message), true);

// A one-button message for an outcome the user only has to read: a locked file
// or a batch that partly failed. Errors that carry a raw Go reason still go
// through errorDialogHtml, which hides it behind the disclosure.
export const showMessage = (message) => openAppDialog(textToDialogHtml(message), false);

// Advance before resolving so a resumed caller cannot replace a queued request.
export function answerDialog(result) {
    if (!active) return;
    const answered = active;
    showNextDialog();
    answered.done(result);
}

// A delayed close event must not steal focus from an active request.
export function restoreOpener() {
    if (active) return;
    const target = opener;
    opener = null;
    target?.focus?.();
}
