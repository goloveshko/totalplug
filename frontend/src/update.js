import { UpdatePlugin } from '../wailsjs/go/gui/App';
import { t, tp } from './i18n.js';
import { get } from 'svelte/store';
import { plugins, progressRunning } from './state.js';
import { isUpdatable } from './plugin.js';
import { firstHttpUrl } from './text.js';
import { showConfirm } from './dialog.js';
import {
    beginProgress,
    beginProgressItem,
    finishProgress,
    setProgressTitle,
    waitProgressDismissed,
} from './progress.js';
import { runScan } from './catalog.js';

// What one updater call does, and the two ways it gets called: the button in a
// row, and the batch queue. Both drive the same progress dialog.

// One updater call mapped onto the three outcomes the UI has to tell apart:
// success, a file lock (systemic: Total Commander holds every plugin file),
// and a per-plugin error.
async function updateOnePlugin(plugin) {
    try {
        const result = await UpdatePlugin(plugin);
        if (result && result.locked) return { kind: 'locked' };
        return { kind: 'ok', backupPath: result && result.backup_path };
    } catch (err) {
        return { kind: 'error', err };
    }
}

// Reached only from the table's delegated click handler — one listener for every
// row, so a re-render cannot leave a button wired to a stale index.
export async function handleUpdate(index) {
    // The confirm and the progress dialog are separate modals: without this
    // guard a second click starts a parallel UpdatePlugin for the same plugin.
    if (get(progressRunning)) return;

    const clicked = get(plugins)[index];
    if (!clicked) return;

    if (!(await showConfirm(t('update.confirm', { name: clicked.name, version: clicked.latest_version })))) {
        return;
    }

    // A plugins:refreshed can land while the confirm modal is open and swap the
    // plugin list, so the object captured above may now carry a stale
    // download_url/version. Re-resolve by the stable id and update the live
    // record; if the refresh dropped the plugin, there is nothing to update.
    const plugin = get(plugins).find((p) => p.id === clicked.id);
    if (!plugin) return;

    // One dialog shows progress, then the final result in the same place.
    beginProgress(t('update.progress.title', { name: plugin.name }));
    const outcome = await updateOnePlugin(plugin);

    if (outcome.kind === 'locked') {
        finishProgress({ text: t('update.locked', { name: plugin.name }) });
    } else if (outcome.kind === 'error') {
        const raw = String(outcome.err?.message ?? outcome.err);
        finishProgress({
            text: t('update.error_friendly'),
            // The address the failing request went to is the one action this error
            // offers, so it is handed over as data and the component names it.
            link: firstHttpUrl(raw),
            tech: raw,
        });
    } else {
        finishProgress({
            text: outcome.backupPath
                ? t('update.done_backup', { name: plugin.name, path: outcome.backupPath })
                : t('update.done', { name: plugin.name }),
        });
    }

    await waitProgressDismissed();
    if (outcome.kind === 'ok') runScan(false);
}

export async function handleUpdateAll() {
    if (get(progressRunning)) return;

    const offered = get(plugins).filter(isUpdatable);
    if (offered.length === 0) return;

    // The count is the whole queue, not the visible rows: the search filter is a
    // way to find a plugin, not a way to scope a batch, so the confirm has to
    // spell out what is actually about to happen.
    if (!(await showConfirm(t('update_all.confirm', { count: tp('count.plugins', offered.length) })))) {
        return;
    }

    // A plugins:refreshed can land while the confirm is open and swap the plugin
    // list, the hazard handleUpdate already re-resolves for: rebuild the
    // queue from the live records by stable id, so the batch never downloads from a
    // superseded download_url and drops whatever is no longer offerable.
    const queue = offered
        .map((stale) => get(plugins).find((p) => p.id === stale.id))
        .filter((p) => p && isUpdatable(p));
    if (queue.length === 0) return;

    const done = [];
    const failed = [];
    const rawErrors = [];
    let lockedAt = -1;

    beginProgress(t('update_all.title'));
    for (let i = 0; i < queue.length; i++) {
        const plugin = queue[i];
        setProgressTitle(t('update_all.item', { current: i + 1, total: queue.length, name: plugin.name }));
        beginProgressItem();

        const outcome = await updateOnePlugin(plugin);
        if (outcome.kind === 'ok') {
            done.push(plugin);
        } else if (outcome.kind === 'error') {
            failed.push(plugin);
            rawErrors.push(`${plugin.name}: ${String(outcome.err?.message ?? outcome.err)}`);
        } else {
            // One locked file means Total Commander is running, so every
            // remaining plugin would fail the same way. Stop and say so.
            lockedAt = i;
            break;
        }
    }

    const skipped = lockedAt >= 0 ? queue.slice(lockedAt + 1) : [];
    // One row per plugin the queue touched, in the order they were worked on:
    // updated first, then the failures, then whatever a lock cut off.
    const rows = [
        ...done.map((p) => ({ glyph: '✓', cls: 'ua-ok', text: `${p.name} — ${p.latest_version}` })),
        ...failed.map((p) => ({ glyph: '✗', cls: 'ua-err', text: p.name })),
        ...skipped.map((p) => ({ glyph: '—', cls: 'ua-skip', text: p.name })),
    ];
    finishProgress({
        // The count is the headline, the rows below it are what the user acts on.
        text: done.length ? tp('update_all.updated', done.length) : t('update_all.none'),
        rows,
        note: lockedAt >= 0 ? t('update_all.locked_stop', { name: queue[lockedAt].name }) : '',
        tech: rawErrors.join('\n'),
    });

    await waitProgressDismissed();
    if (done.length) runScan(false);
}
