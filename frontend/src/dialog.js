import { t } from './i18n.js';
import { dom } from './dom.js';
import { escapeHtml, linkify, interceptLinks } from './text.js';

// The in-app replacement for native confirm()/alert(): those render with the
// "wails" origin header and cannot be styled. One modal dialog serves both the
// yes/no confirmations and the error messages, and it resolves as a promise.

// Element that opened the current dialog; focus goes back to it on close so
// keyboard users do not land at the top of the document.
let opener = null;
let resolve = null;

export const dialogMarkup = () => `
      <dialog id="app-dialog">
        <button type="button" class="dlg-close" id="app-dialog-close" title="${t('dialog.close')}" aria-label="${t('dialog.close')}">
          <svg viewBox="0 0 24 24"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
        <div class="dlg-message" id="app-dialog-msg"></div>
        <div class="editor-actions">
          <button type="button" class="btn-save" id="app-dialog-ok">${t('dialog.ok')}</button>
          <button type="button" id="app-dialog-cancel">${t('dialog.cancel')}</button>
        </div>
      </dialog>`;

// Raw Go errors are technical English; show the localized friendly sentence
// with a named link and hide the exact error behind a disclosure. The link
// label depends on where the URL came from (plugin page vs catalog).
export function errorDialogHtml(err, friendlyMessage, linkLabel) {
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

export function openAppDialog(html, isConfirm) {
    return new Promise((done) => {
        resolve = done;
        opener = document.activeElement;
        dom.appDialogMsg.innerHTML = html;
        dom.appDialogCancel.style.display = isConfirm ? '' : 'none';
        dom.appDialog.showModal();
        // Enter must never confirm an update by accident: focus the safe button.
        (isConfirm ? dom.appDialogCancel : dom.appDialogOk).focus();
    });
}

const textToDialogHtml = (message) => linkify(escapeHtml(message));

export const showConfirm = (message) => openAppDialog(textToDialogHtml(message), true);

function settle(result) {
    if (resolve) {
        resolve(result);
        resolve = null;
    }
}

export function bindDialog() {
    dom.appDialogOk.addEventListener('click', () => {
        settle(true);
        dom.appDialog.close();
    });
    dom.appDialogCancel.addEventListener('click', () => {
        settle(false);
        dom.appDialog.close();
    });
    document.getElementById('app-dialog-close').addEventListener('click', () => {
        settle(false);
        dom.appDialog.close();
    });
    // Links in dialog messages open in the system browser, never in the webview.
    interceptLinks(dom.appDialogMsg);
    // Esc or any other dismissal path must resolve a pending confirm as "no".
    dom.appDialog.addEventListener('close', () => {
        settle(false);
        if (opener) {
            opener.focus?.();
            opener = null;
        }
    });
}
