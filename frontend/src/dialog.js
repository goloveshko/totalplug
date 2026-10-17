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

// The pending answer, and the element that opened the dialog so focus can go
// back to it on close — keyboard users must not land at the top of the document.
let resolve = null;
let opener = null;

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
        resolve = done;
        opener = document.activeElement;
        dialogState.set({ open: true, html, isConfirm: Boolean(isConfirm) });
    });
}

const textToDialogHtml = (message) => linkify(escapeHtml(message));

export const showConfirm = (message) => openAppDialog(textToDialogHtml(message), true);

// Every way out of the dialog ends here with the answer it means. The pending
// promise is settled first, so a second call — the close event that follows the
// store change — finds nothing to answer and cannot turn a yes into a no.
export function answerDialog(result) {
    if (resolve) {
        resolve(result);
        resolve = null;
    }
    dialogState.update((state) => (state.open ? { ...state, open: false } : state));
}

// Runs on the dialog's own close event, after it has really left the top layer:
// while it was open the rest of the document was inert and could not take focus.
export function restoreOpener() {
    if (opener) {
        opener.focus?.();
        opener = null;
    }
}
