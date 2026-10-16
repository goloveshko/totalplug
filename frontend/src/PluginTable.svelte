<script>
    import { derived } from 'svelte/store';
    // Aliased, not `tr`/`trp` like the other island: a component that renders
    // table rows cannot import a binding named `tr`, because Svelte then has to
    // guess whether `<tr>` is the element or a component.
    import { tr as msg, trp as plural } from './i18n.js';
    import { scanFailure, scanRunning, searchQuery } from './catalog.js';
    import { badgeFor, isUpdatable } from './plugin.js';
    import { plugins, progressRunning, settingsOpen } from './state.js';
    import { handleUpdate, handleUpdateAll } from './update.js';
    import { openCard } from './card.js';

    // The plugin table as one component: rows, the batch bar over them and the
    // placeholder that stands in when there are none. Every value it shows comes
    // from a store, so a language switch re-labels it in place and a scan replaces
    // the rows without anyone rebuilding the tree under them.

    // A row keeps its index into the plugin list, not into the filtered view: the
    // card and both update paths look a plugin up by that index, and a background
    // refresh is free to swap the list underneath them.
    const rows = derived([plugins, searchQuery], ([$plugins, $query]) => {
        const q = $query.trim().toLowerCase();
        return $plugins
            .map((plugin, index) => ({ plugin, index }))
            .filter(({ plugin }) => !q || plugin.name.toLowerCase().includes(q));
    });

    const offered = derived(plugins, ($plugins) => $plugins.filter(isUpdatable).length);

    // One delegated handler, as the table had before: the update button keeps its
    // own action, a click anywhere else in the row opens the plugin card.
    function onTableClick(event) {
        const updateBtn = event.target.closest('button.btn-update');
        if (updateBtn) {
            handleUpdate(Number(updateBtn.dataset.update));
            return;
        }
        if (event.target.closest('button')) return;
        const row = event.target.closest('tr[data-idx]');
        if (row) openCard(Number(row.dataset.idx), row);
    }

    // A failed scan outranks everything: the dialog about it gets dismissed, and
    // the rows it replaced may be from a list that is no longer trustworthy.
    const notice = $derived(
        $scanFailure
            ? $msg('scan.error_friendly')
            : $rows.length > 0
              ? ''
              : $scanRunning
                ? $msg('table.scanning')
                : $plugins.length === 0
                  ? $msg('table.none')
                  : $msg('table.noresults'),
    );

    function badge(status) {
        const { cls, glyph, key } = badgeFor(status);
        return { cls, glyph, label: $msg(key) };
    }

    // The header and the placeholder's colspan come from the same list, so a
    // column added here cannot quietly make a placeholder row misaligned.
    const columns = ['type', 'plugin', 'installed', 'latest', 'status', 'actions'];
</script>

<div id="view-main" hidden={$settingsOpen}>
    {#if $offered > 0}
        <div class="update-all">
            <span class="update-all-count">{$plural('update_all.count', $offered)}</span>
            <button type="button" class="btn-save" disabled={$progressRunning} onclick={handleUpdateAll}>
                {$msg('update_all.btn')}
            </button>
        </div>
    {/if}
    <div class="table-container">
        <table>
            <thead>
                <tr>
                    {#each columns as column}
                        <th>{$msg(`col.${column}`)}</th>
                    {/each}
                </tr>
            </thead>
            <!-- The whole row opens the card, which is a mouse shortcut over the
                 buttons in it rather than the only way in — the checker is right,
                 and the keyboard pass is owed in ROADMAP. Ignored here so the
                 delegation stays one listener for every row. -->
            <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
            <tbody onclick={onTableClick}>
                {#if notice}
                    <tr class="table-empty">
                        <td colspan={columns.length}>
                            {notice}
                            {#if $scanFailure}
                                <details class="dlg-tech">
                                    <summary>{$msg('dialog.tech_details')}</summary>
                                    <div class="dlg-tech-body">{$scanFailure}</div>
                                </details>
                            {/if}
                        </td>
                    </tr>
                {:else}
                    {#each $rows as { plugin, index } (plugin.id)}
                        {@const mark = badge(plugin.status)}
                        <tr data-idx={index}>
                            <td class="cell-type">{plugin.type}</td>
                            <td>{plugin.name}</td>
                            <td><code>{plugin.installed_version}</code></td>
                            <td><code>{plugin.latest_version || '—'}</code></td>
                            <td>
                                <span class="badge {mark.cls}">{mark.glyph} {mark.label}</span>
                            </td>
                            <td>
                                {#if isUpdatable(plugin)}
                                    <button type="button" class="btn-update" data-update={index}>
                                        {$msg('btn.update')}
                                    </button>
                                {/if}
                            </td>
                        </tr>
                    {/each}
                {/if}
            </tbody>
        </table>
    </div>
</div>
