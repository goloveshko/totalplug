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
    //
    // The log is grouped by plugin, because a plugin updated several times is most
    // of it. A group shows its newest line, which is the only line the Go side
    // keeps a rollback for, and the earlier lines wait behind the +/− in its name.

    let el = $state(null);
    let closeBtn = $state(null);
    let confirmBtn = $state(null);
    let expanded = $state({});
    const state = $derived($historyState);

    // Dates follow the UI language, not the OS locale.
    const fmt = $derived(
        new Intl.DateTimeFormat($lang === 'ru' ? 'ru-RU' : 'en-GB', { dateStyle: 'medium', timeStyle: 'short' }),
    );

    // One group per plugin, in the order their newest line appears. The log comes
    // newest first, so the groups do too, and each keeps its own lines in order.
    const groups = $derived.by(() => {
        const byPlugin = new Map();
        state.entries.forEach((entry, idx) => {
            const key = entry.plugin_id || entry.plugin_name || String(idx);
            let group = byPlugin.get(key);
            if (!group) {
                group = { key, rows: [] };
                byPlugin.set(key, group);
            }
            group.rows.push({ entry, idx });
        });
        return [...byPlugin.values()];
    });

    const toggle = (key) => (expanded[key] = !expanded[key]);

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
                // The dialog is its own scroller and Close is the last control inside
                // it, so the plain focus() on open pulled the log to its end.
                closeBtn?.focus({ preventScroll: true });
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
                {#each groups as group}
                    {@const open = !!expanded[group.key]}
                    {#each group.rows as row, offset}
                        {#if row.idx === state.pending}
                            <tr class="history-confirm">
                                <td colspan="4">
                                    <p>{question(row.entry)}</p>
                                    <p class="history-note{state.failure ? ' history-failed' : ''}">
                                        {state.failure || $msg('history.confirm_note')}
                                    </p>
                                    <div class="editor-actions">
                                        <button
                                            type="button"
                                            class="btn-update"
                                            bind:this={confirmBtn}
                                            disabled={state.busy}
                                            onclick={() => confirmRollback(row.idx)}
                                        >
                                            {$msg('history.rollback')}
                                        </button>
                                        <button type="button" disabled={state.busy} onclick={cancelRollback}>
                                            {$msg('dialog.cancel')}
                                        </button>
                                    </div>
                                </td>
                            </tr>
                        {:else if offset === 0 || open}
                            <tr>
                                <td>
                                    {#if offset === 0}
                                        {#if group.rows.length > 1}
                                            <button
                                                type="button"
                                                class="history-toggle"
                                                aria-expanded={String(open)}
                                                aria-label={$msg(open ? 'history.collapse' : 'history.expand', {
                                                    plugin: row.entry.plugin_name || row.entry.plugin_id,
                                                })}
                                                onclick={() => toggle(group.key)}
                                            >
                                                {open ? '−' : '+'}
                                            </button>
                                        {/if}
                                        {row.entry.plugin_name || row.entry.plugin_id}
                                    {/if}
                                </td>
                                <td>
                                    <code>{row.entry.version}</code>
                                    {#if row.entry.restored}
                                        <span class="history-kind">{$msg('history.restored')}</span>
                                    {/if}
                                </td>
                                <td>{formatStamp(fmt, row.entry.applied_at)}</td>
                                <td class="history-action">
                                    {#if row.entry.backup_path}
                                        <button type="button" class="btn-update" onclick={() => offerRollback(row.idx)}>
                                            {$msg('history.rollback')}
                                        </button>
                                    {/if}
                                </td>
                            </tr>
                        {/if}
                    {/each}
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
