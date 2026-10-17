<script>
    // Aliased, not `tr` like the other dialogs: a component that renders table rows
    // cannot import a binding named `tr`, because Svelte then has to guess whether
    // `<tr>` is the element or a component.
    import { lang, tr as msg } from './i18n.js';
    import { cancelRollback, closeHistory, confirmRollback, historyState, offerRollback } from './history.js';
    import { formatStamp } from './text.js';

    // The log of applied updates as an island that never rebuilds: the rows, the
    // question that takes the place of one of them, and the dates are read from
    // stores, so a language switch re-labels and re-formats a log that is open.

    let el = $state(null);
    let closeBtn = $state(null);
    let confirmBtn = $state(null);
    const state = $derived($historyState);

    // Dates follow the UI language, not the OS locale.
    const fmt = $derived(
        new Intl.DateTimeFormat($lang === 'ru' ? 'ru-RU' : 'en-GB', { dateStyle: 'medium', timeStyle: 'short' }),
    );

    const question = (entry) =>
        entry.replaced_version
            ? $msg('history.confirm', { version: entry.replaced_version, current: entry.version })
            : $msg('history.confirm_unknown');

    // A <dialog> has no attribute that opens it, so the store's flag drives the
    // two DOM-only calls. The browser would give focus to the first rollback
    // button in the table, so the way out is asked for instead.
    $effect(() => {
        if (!el) return;
        if (state.open) {
            if (!el.open) {
                el.showModal();
                closeBtn?.focus();
            }
        } else if (el.open) {
            el.close();
        }
    });

    // The question replaces the row the keyboard was on, so focus moves with it —
    // and comes back to it after an attempt that did not work, because a button
    // that stops being disabled no longer holds the focus itself.
    $effect(() => {
        if (state.pending >= 0) confirmBtn?.focus();
    });
</script>

<dialog id="history-dialog" bind:this={el} onclose={closeHistory}>
    <h2>{$msg('history.title')}</h2>
    <table>
        <thead>
            <tr>
                <th>{$msg('history.col.plugin')}</th>
                <th>{$msg('history.col.version')}</th>
                <th>{$msg('history.col.date')}</th>
                <th>{$msg('history.col.action')}</th>
            </tr>
        </thead>
        <tbody>
            {#if state.entries.length === 0}
                <tr class="table-empty">
                    <td colspan="4">{$msg('history.empty')}</td>
                </tr>
            {:else}
                {#each state.entries as entry, idx}
                    {#if idx === state.pending}
                        <tr class="history-confirm">
                            <td colspan="4">
                                <p>{question(entry)}</p>
                                <p class="history-note{state.failure ? ' history-failed' : ''}">
                                    {state.failure || $msg('history.confirm_note')}
                                </p>
                                <div class="editor-actions">
                                    <button
                                        type="button"
                                        class="btn-update"
                                        bind:this={confirmBtn}
                                        disabled={state.busy}
                                        onclick={() => confirmRollback(idx)}
                                    >
                                        {$msg('history.rollback')}
                                    </button>
                                    <button type="button" disabled={state.busy} onclick={cancelRollback}>
                                        {$msg('dialog.cancel')}
                                    </button>
                                </div>
                            </td>
                        </tr>
                    {:else}
                        <tr>
                            <td>{entry.plugin_name || entry.plugin_id}</td>
                            <td>
                                <code>{entry.version}</code>
                                {#if entry.restored}
                                    <span class="history-kind">{$msg('history.restored')}</span>
                                {/if}
                            </td>
                            <td>{formatStamp(fmt, entry.applied_at)}</td>
                            <td class="history-action">
                                {#if entry.backup_path}
                                    <button type="button" class="btn-update" onclick={() => offerRollback(idx)}>
                                        {$msg('history.rollback')}
                                    </button>
                                {/if}
                            </td>
                        </tr>
                    {/if}
                {/each}
            {/if}
        </tbody>
    </table>
    <div class="editor-actions">
        <button type="button" id="history-close" bind:this={closeBtn} onclick={closeHistory}>
            {$msg('dialog.close')}
        </button>
    </div>
</dialog>
