import { EventsOn } from '../wailsjs/runtime';
import { get, writable } from 'svelte/store';
import { t } from './i18n.js';
import { progressRunning } from './state.js';

// One dialog shows the updater's progress stream and then the final result in
// the same place. It is shared by the single update and the "update all" queue,
// which is why opening it and rewinding the bar are separate calls.
// ProgressOverlay.svelte draws this state; the waiter below stays here because
// the caller must not re-scan the plugin list until the result is dismissed.

// `url` holds the bare address and the component puts the translated word in
// front of it, so a language switch re-labels the source line while it is shown.
export const progressState = writable({
    open: false,
    title: '',
    phase: '',
    pct: '',
    indeterminate: true,
    // The last download percentage: while the bar is indeterminate the animation
    // is styled from the track's class, and the width is left where it was.
    width: 0,
    url: '',
    resultHtml: '',
});

// While an update runs, the dialog cannot be dismissed.
let resolve = null;

const patchProgress = (patch) => progressState.update((state) => ({ ...state, ...patch }));

// #progress-phase is a live region, so writing the value it already has would
// make a screen reader repeat the phase for every item of a batch update.
function setPhase(text) {
    progressState.update((state) => (state.phase === text ? state : { ...state, phase: text }));
}

// renderUpdateProgress maps one updater event onto the dialog. Events arriving
// outside a running update (stale listener, closed dialog) are ignored.
export function renderUpdateProgress(p) {
    const state = get(progressState);
    if (!state.open || !get(progressRunning)) return;

    const percent = Number(p?.percent ?? -1);
    const phase = p?.phase || 'download';
    const downloading = phase === 'download' && percent >= 0;
    if (downloading) {
        setPhase(t('update.progress.download'));
        patchProgress({
            pct: `— ${Math.min(100, Math.round(percent))}%`,
            indeterminate: false,
            width: Math.min(100, percent),
        });
    } else {
        setPhase(t('update.progress.' + phase));
        patchProgress({ pct: '…', indeterminate: true });
    }
    const url = String(p?.url || '');
    // Sticky: extract/install events carry no URL, keep showing the download source.
    if (url) patchProgress({ url });
}

export function beginProgress(titleText) {
    progressRunning.set(true);
    patchProgress({ open: true, title: titleText, resultHtml: '' });
    beginProgressItem();
}

// The batch loop renames the dialog for the plugin it is starting.
export const setProgressTitle = (titleText) => patchProgress({ title: titleText });

export function beginProgressItem() {
    setPhase(t('update.progress.download'));
    patchProgress({ pct: '…', indeterminate: true, width: 0, url: '' });
}

export function finishProgress(html) {
    progressRunning.set(false);
    patchProgress({ resultHtml: html });
}

// The caller waits for the user to acknowledge the result before the plugin list
// is re-scanned.
export function waitProgressDismissed() {
    return new Promise((done) => {
        resolve = done;
    });
}

// A button asks for the close and the dialog's own close event settles the
// waiter, so the rescan happens once the result has really left the screen.
export const closeProgress = () => patchProgress({ open: false });

export function settleProgress() {
    if (resolve) {
        resolve();
        resolve = null;
    }
}

// Registered once at startup: the listeners survive a language switch, which is
// why the progress state is read through the store instead of cached in a closure.
export function initProgressEvents() {
    EventsOn('update:progress', (p) => {
        renderUpdateProgress(p);
    });
}
