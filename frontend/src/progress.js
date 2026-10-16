import { EventsOn } from '../wailsjs/runtime';
import { get } from 'svelte/store';
import { t } from './i18n.js';
import { dom } from './dom.js';
import { progressRunning } from './state.js';
import { escapeHtml, interceptLinks } from './text.js';

// One dialog shows the updater's progress stream and then the final result in
// the same place. It is shared by the single update and the "update all" queue,
// which is why opening it and rewinding the bar are separate calls.

// While an update runs, the dialog cannot be dismissed.
let resolve = null;

export const progressMarkup = () => `
      <dialog id="update-progress">
        <button type="button" class="dlg-close" id="progress-close" title="${t('dialog.close')}" aria-label="${t('dialog.close')}" hidden>
          <svg viewBox="0 0 24 24"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
        </button>
        <h2 id="progress-title"></h2>
        <div id="progress-live">
          <div class="progress-track" id="progress-track"><div class="progress-fill" id="progress-fill"></div></div>
          <!-- The percent sits in its own aria-hidden span: a live region that
               announced every download tick would talk over itself, so only the
               phase name is worth announcing. -->
          <div class="progress-phase"><span id="progress-phase" role="status"></span><span id="progress-pct" aria-hidden="true"></span></div>
          <div class="progress-url" id="progress-url"></div>
        </div>
        <div class="dlg-message" id="progress-result" role="status" hidden></div>
        <div class="editor-actions">
          <button type="button" class="btn-save" id="progress-ok" hidden>${t('dialog.ok')}</button>
        </div>
      </dialog>`;

// #progress-phase is a live region, so writing the value it already has would
// make a screen reader repeat the phase for every item of a batch update.
function setProgressPhase(text) {
    if (dom.progressPhase.textContent !== text) dom.progressPhase.textContent = text;
}

// renderUpdateProgress maps one updater event onto the dialog. Events arriving
// outside a running update (stale listener, closed dialog) are ignored.
export function renderUpdateProgress(p) {
    if (!dom.progressDialog || !dom.progressDialog.open || !get(progressRunning)) return;

    const percent = Number(p?.percent ?? -1);
    const phase = p?.phase || 'download';
    if (phase === 'download' && percent >= 0) {
        setProgressPhase(t('update.progress.download'));
        dom.progressPct.textContent = ` — ${Math.min(100, Math.round(percent))}%`;
        dom.progressTrack.classList.remove('indeterminate');
        dom.progressFill.style.width = `${Math.min(100, percent)}%`;
    } else {
        setProgressPhase(t('update.progress.' + phase));
        dom.progressPct.textContent = '…';
        dom.progressTrack.classList.add('indeterminate');
    }
    const url = String(p?.url || '');
    // Sticky: extract/install events carry no URL, keep showing the download source.
    if (url) {
        dom.progressUrl.textContent = `${t('update.progress.source')}: ${url}`;
        dom.progressUrl.title = url;
    }
}

export function beginProgress(titleText) {
    dom.progressTitle.textContent = titleText;
    dom.progressResult.innerHTML = '';
    dom.progressResult.hidden = true;
    dom.progressOk.hidden = true;
    dom.progressClose.hidden = true;
    dom.progressLive.hidden = false;
    progressRunning.set(true);
    if (!dom.progressDialog.open) dom.progressDialog.showModal();
    beginProgressItem();
}

export function beginProgressItem() {
    setProgressPhase(t('update.progress.download'));
    dom.progressPct.textContent = '…';
    dom.progressUrl.textContent = '';
    dom.progressFill.style.width = '0%';
    dom.progressTrack.classList.add('indeterminate');
}

export function finishProgress(html) {
    progressRunning.set(false);
    dom.progressLive.hidden = true;
    dom.progressResult.innerHTML = html;
    dom.progressResult.hidden = false;
    dom.progressOk.hidden = false;
    dom.progressClose.hidden = false;
    dom.progressOk.focus();
}

function settle() {
    if (resolve) {
        resolve();
        resolve = null;
    }
}

// The caller waits for the user to acknowledge the result before the plugin list
// is re-scanned.
export function waitProgressDismissed() {
    return new Promise((done) => {
        resolve = done;
    });
}

// Registered once at startup: the listeners survive a language switch, which is
// why the progress state is read through the store instead of cached in a closure.
export function initProgressEvents() {
    EventsOn('update:progress', (p) => {
        renderUpdateProgress(p);
    });
}

export function bindProgress() {
    dom.progressOk.addEventListener('click', () => {
        settle();
        dom.progressDialog.close();
    });
    dom.progressClose.addEventListener('click', () => {
        settle();
        dom.progressDialog.close();
    });
    dom.progressDialog.addEventListener('cancel', (e) => {
        if (get(progressRunning)) e.preventDefault();
    });
    dom.progressDialog.addEventListener('close', settle);
    interceptLinks(dom.progressResult);
}
