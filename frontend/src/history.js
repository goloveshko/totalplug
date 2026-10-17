import { GetUpdateHistory, RollbackUpdate } from '../wailsjs/go/gui/App';
import { get, writable } from 'svelte/store';
import { t } from './i18n.js';
import { runScan } from './catalog.js';
import { openAppDialog, errorDialogHtml } from './dialog.js';

// Update history: what the updater actually did, newest data first as the Go
// side stores it. A line whose kept copy is still on disk can put that copy
// back, which is the only action a past update takes here.
// HistoryDialog.svelte draws this state; the round trips stay here because a
// component that renders a row cannot wait on the Go call that produced it.

// `pending` is the row that shows its confirmation, `failure` says why the last
// attempt did not work, and `busy` keeps a second click off a rollback that is
// already running.
export const historyState = writable({ open: false, entries: [], pending: -1, failure: '', busy: false });

const patchHistory = (patch) => historyState.update((state) => ({ ...state, ...patch }));

const loadEntries = async () => patchHistory({ entries: (await GetUpdateHistory()) || [] });

// Reading the log happens before the dialog opens: an empty table is a failure
// to reach the data folder, and that belongs in the error dialog rather than in
// a window full of nothing.
export async function openHistory() {
    try {
        const entries = (await GetUpdateHistory()) || [];
        patchHistory({ open: true, entries, pending: -1, failure: '', busy: false });
    } catch (err) {
        openAppDialog(errorDialogHtml(err, t('history.error_friendly')));
    }
}

// A button closes it and the dialog's own close event arrives afterwards, so the
// second call finds the flag already down and leaves the state alone.
export const closeHistory = () => historyState.update((state) => (state.open ? { ...state, open: false } : state));

export const offerRollback = (idx) => patchHistory({ pending: idx, failure: '' });

export const cancelRollback = () => patchHistory({ pending: -1, failure: '' });

export async function confirmRollback(idx) {
    const { entries, busy } = get(historyState);
    const entry = entries[idx];
    if (!entry || busy) return;
    patchHistory({ busy: true });
    try {
        const result = await RollbackUpdate(entry);
        if (result?.locked) {
            patchHistory({ failure: t('history.locked') });
            return;
        }
        patchHistory({ pending: -1, failure: '' });
        await loadEntries();
        // The plugin table below shows the version this changed, so it has to be
        // read again — the same refresh an update ends with.
        runScan(false);
    } catch (err) {
        patchHistory({ failure: String(err?.message ?? err) });
    } finally {
        patchHistory({ busy: false });
    }
}
