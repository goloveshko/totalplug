import { UpdatePlugin, UpdatePluginInsecure } from '../wailsjs/go/gui/App';
import { EventsOn } from '../wailsjs/runtime';
import { get, writable } from 'svelte/store';
import { t, tp } from './i18n.js';
import { plugins, progressRunning } from './state.js';
import { isUpdatable } from './plugin.js';
import { escapeHtml, linkify } from './text.js';
import { openAppDialog, errorDialogHtml, showConfirm, showMessage } from './dialog.js';
import { runScan } from './catalog.js';

// What one updater call does, and the two ways it gets called: the button in a
// row or in the card, and the batch queue. Progress lives in the row being
// updated rather than in a dialog, so the list stays readable while it runs.

// The row that is being worked on. The Go updater serialises calls and its
// progress events carry no plugin id, so there is never more than one.
export const activeUpdate = writable({ id: '', phase: '', percent: -1 });

const idle = { id: '', phase: '', percent: -1 };

// Registered once at startup: the listener survives a language switch, so the
// row it feeds reads the phase label through the language store.
export function initUpdateEvents() {
    EventsOn('update:progress', (p) => {
        const current = get(activeUpdate);
        if (!current.id) return;

        const phase = p?.phase || 'download';
        const percent = Number(p?.percent ?? -1);
        // Only the download has a known total; extract and install report -1 and
        // get the indeterminate bar.
        activeUpdate.set({ id: current.id, phase, percent: phase === 'download' ? percent : -1 });
    });
}

// One updater call mapped onto the outcomes the UI has to tell apart: success,
// a file lock (systemic: Total Commander holds every plugin file), a source the
// HTTPS policy refused, and a per-plugin error.
async function updateOnePlugin(plugin, allowInsecure = false) {
    try {
        const result = allowInsecure ? await UpdatePluginInsecure(plugin) : await UpdatePlugin(plugin);
        if (result && result.locked) return { kind: 'locked' };
        if (result && result.insecure) return { kind: 'insecure' };
        return { kind: 'ok' };
    } catch (err) {
        return { kind: 'error', err };
    }
}

// The catalog is a community archive of old plugin sites: plenty of them hand the
// archive off to the author's plain-HTTP server, which the updater refuses rather
// than unpack an interceptable file over the plugin folder. The refusal is not
// something the frontend can predict from the record — it happens mid-redirect —
// so it is answered where it comes up: the run stops, the user is asked whether
// this plugin may be fetched unencrypted, and only a yes downloads it.
// The question is per plugin: agreeing to one archive says nothing about another
// author's server.
async function updateWithConsent(plugin) {
    const outcome = await updateOnePlugin(plugin, false);
    if (outcome.kind !== 'insecure') return outcome;

    // Nothing was fetched, so quiet the row while the question is open: a bar
    // moving behind a modal reads as an update that is still running. The caller
    // that runs a queue needs the flag back up either way, and the one that ran a
    // single update clears it as soon as this returns.
    activeUpdate.set(idle);
    progressRunning.set(false);
    const allowed = await showConfirm(t('update.insecure_confirm', { name: plugin.name }));
    progressRunning.set(true);
    if (!allowed) return { kind: 'declined' };

    activeUpdate.set({ id: plugin.id, phase: 'download', percent: -1 });
    return updateOnePlugin(plugin, true);
}

// The row is the progress display, so a finished plugin has to look finished
// there before the rescan lands: patching the record is what removes the button
// and the offer count in the same frame the update ends.
function markUpdated(plugin) {
    plugins.update((list) =>
        list.map((p) =>
            p.id === plugin.id
                ? {
                      ...p,
                      installed_version: plugin.latest_version,
                      status: 'up_to_date',
                      last_checked: new Date().toISOString(),
                  }
                : p,
        ),
    );
}

// Reached from the table's delegated handler and from the card, always by the
// stable id: a background refresh is free to replace the list under either.
export async function handleUpdate(id) {
    if (get(progressRunning)) return;

    const plugin = get(plugins).find((p) => p.id === id);
    if (!plugin || !isUpdatable(plugin)) return;

    progressRunning.set(true);
    activeUpdate.set({ id, phase: 'download', percent: -1 });
    const outcome = await updateWithConsent(plugin);
    activeUpdate.set(idle);
    progressRunning.set(false);

    if (outcome.kind === 'declined') {
        // The row still offers the same update; nothing happened to report.
        return;
    }
    if (outcome.kind === 'locked') {
        showMessage(t('update.locked', { name: plugin.name }));
    } else if (outcome.kind === 'error') {
        openAppDialog(errorDialogHtml(outcome.err, t('update.error_friendly')), false);
    } else {
        markUpdated(plugin);
        runScan(false);
    }
}

// What the batch dialog says when something did not go through. The headline is
// the count that succeeded — the rows the user can still act on are the failures,
// the downloads they declined, and the ones a lock cut off.
function batchSummaryHtml(doneCount, failed, skipped, lockedName, rawErrors) {
    const head = doneCount ? tp('update_all.updated', doneCount) : t('update_all.none');
    let html = `<p class="ua-head">${escapeHtml(head)}</p>`;

    const rows = [
        ...failed.map((p) => ({ cls: 'ua-err', glyph: '✗', text: p.name })),
        ...skipped.map((p) => ({ cls: 'ua-skip', glyph: '—', text: p.name })),
    ];
    if (rows.length) {
        html +=
            '<div class="ua-summary">' +
            rows
                .map(
                    (row) =>
                        `<div class="ua-row ${row.cls}"><span class="ua-glyph" aria-hidden="true">${row.glyph}</span><span>${escapeHtml(row.text)}</span></div>`,
                )
                .join('') +
            '</div>';
    }

    if (lockedName) {
        html += `<p class="ua-note">${escapeHtml(t('update_all.locked_stop', { name: lockedName }))}</p>`;
    }
    if (rawErrors.length) {
        html +=
            '<details class="dlg-tech"><summary>' +
            escapeHtml(t('dialog.tech_details')) +
            '</summary><div class="dlg-tech-body">' +
            linkify(escapeHtml(rawErrors.join('\n'))) +
            '</div></details>';
    }
    return html;
}

export async function handleUpdateAll() {
    if (get(progressRunning)) return;

    const offered = get(plugins).filter(isUpdatable);
    if (offered.length === 0) return;

    // The count is the whole queue, not the visible rows: the search and the
    // updates-only filter are ways to find a plugin, not ways to scope a batch,
    // so the confirm has to spell out what is actually about to happen.
    if (!(await showConfirm(t('update_all.confirm', { count: tp('count.plugins', offered.length) })))) {
        return;
    }

    // A plugins:refreshed can land while the confirm is open and swap the plugin
    // list, the hazard handleUpdate already re-resolves for: rebuild the queue
    // from the live records by stable id, so the batch never downloads from a
    // superseded download_url and drops whatever is no longer offerable.
    const queue = offered
        .map((stale) => get(plugins).find((p) => p.id === stale.id))
        .filter((p) => p && isUpdatable(p));
    if (queue.length === 0) return;

    const declined = [];
    const failed = [];
    const rawErrors = [];
    let doneCount = 0;
    let lockedName = '';
    let stoppedAt = -1;

    progressRunning.set(true);
    for (let i = 0; i < queue.length; i++) {
        const plugin = queue[i];
        activeUpdate.set({ id: plugin.id, phase: 'download', percent: -1 });

        const outcome = await updateWithConsent(plugin);
        activeUpdate.set(idle);

        if (outcome.kind === 'ok') {
            doneCount++;
            markUpdated(plugin);
        } else if (outcome.kind === 'declined') {
            // The queue moves on: refusing one author's plaintext server says
            // nothing about the next plugin's source.
            declined.push(plugin);
        } else if (outcome.kind === 'error') {
            failed.push(plugin);
            rawErrors.push(`${plugin.name}: ${String(outcome.err?.message ?? outcome.err)}`);
        } else {
            // One locked file means Total Commander is running, so every
            // remaining plugin would fail the same way. Stop and say so.
            lockedName = plugin.name;
            stoppedAt = i;
            break;
        }
    }
    progressRunning.set(false);

    if (doneCount) runScan(false);

    // Success is already visible in the rows; a dialog is spent only on what the
    // user still has to do something about.
    if (failed.length || lockedName) {
        const skipped = [...declined, ...(stoppedAt >= 0 ? queue.slice(stoppedAt + 1) : [])];
        openAppDialog(batchSummaryHtml(doneCount, failed, skipped, lockedName, rawErrors), false);
    }
}
