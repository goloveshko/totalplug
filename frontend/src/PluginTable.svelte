<script>
    import { derived } from 'svelte/store';
    // Aliased, not `tr`/`trp` like the other island: a component that renders
    // table rows cannot import a binding named `tr`, because Svelte then has to
    // guess whether `<tr>` is the element or a component.
    import { lang, tr as msg, trp as plural } from './i18n.js';
    import { scanFailure, scanRunning, searchQuery, updatesOnly } from './catalog.js';
    import { badgeFor, compareVersions, isUpdatable, matchesQuery } from './plugin.js';
    import { plugins, progressRunning, settingsOpen } from './state.js';
    import { activeUpdate, handleUpdate, handleUpdateAll } from './update.js';
    import { openCard } from './card.js';
    import { releaseDate } from './text.js';

    // The plugin table as one component: rows, the batch bar over them and the
    // placeholder that stands in when there are none. Every value it shows comes
    // from a store, so a language switch re-labels it in place and a scan replaces
    // the rows without anyone rebuilding the tree under them.

    // A row keeps its index into the plugin list, not into the filtered view: the
    // card and both update paths look a plugin up by that index, and a background
    // refresh is free to swap the list underneath them.
    const rows = derived([plugins, searchQuery, updatesOnly], ([$plugins, $query, $updatesOnly]) => {
        return $plugins
            .map((plugin, index) => ({ plugin, index }))
            .filter(({ plugin }) => matchesQuery(plugin, $query) && (!$updatesOnly || isUpdatable(plugin)));
    });

    // Sorting is a table-local affordance: it reorders rows inside each type
    // group (the groups are the table's structure, not a side effect of the
    // scan order) and cycles ascending → descending → off, off handing the
    // rows back to the order the scan handed over.
    let sort = $state({ key: '', dir: 1 });

    const comparators = {
        plugin: (a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }),
        installed: (a, b) => compareVersions(a.installed_version, b.installed_version),
        latest: (a, b) => compareVersions(a.latest_version, b.latest_version),
    };

    function toggleSort(key) {
        if (sort.key !== key) {
            sort = { key, dir: 1 };
        } else if (sort.dir === 1) {
            sort = { key, dir: -1 };
        } else {
            sort = { key: '', dir: 1 };
        }
    }

    function ariaSort(column) {
        if (sort.key !== column) return 'none';
        return sort.dir === 1 ? 'ascending' : 'descending';
    }

    // One group per run of equal types. The scan hands rows over in ini order and
    // the sections are parsed Packer, Lister, FileSystem, Content, so a type does
    // not normally reappear after its run ended — but the key carries the first row
    // anyway, because a run is what a group is and two runs of one type must not
    // collide onto one key. The filters only shorten a run or drop it whole, and
    // the sort reorders rows inside a run, so a group stays one run.
    // A $derived rune, not a derived() store: it reads the component-local sort
    // state, which a store callback cannot track.
    let groups = $derived.by(() => {
        const out = [];
        for (const row of $rows) {
            const last = out[out.length - 1];
            if (last && last.type === row.plugin.type) last.rows.push(row);
            else out.push({ type: row.plugin.type, key: groupKey(row), rows: [row] });
        }
        if (sort.key) {
            const cmp = comparators[sort.key];
            for (const group of out) {
                group.rows.sort((a, b) => cmp(a.plugin, b.plugin) * sort.dir);
            }
        }
        return out;
    });

    function groupKey(row) {
        return `${row.plugin.type}:${row.plugin.id}`;
    }

    const offered = derived(plugins, ($plugins) => $plugins.filter(isUpdatable).length);

    // The strip that owns the filter disappears when nothing is offerable, so the
    // flag must not outlive it: otherwise the next scan that finds an update would
    // come back with the table already filtered and the control to undo it hidden
    // behind nothing.
    $effect(() => {
        if ($updatesOnly && $offered === 0) updatesOnly.set(false);
    });

    // One delegated handler, as the table had before: the update button keeps its
    // own action, a click anywhere else in the row opens the plugin card. The name
    // cell is a button, so its own handler gets there first and the guard below
    // keeps this listener from opening the card a second time.
    function onTableClick(event) {
        const updateBtn = event.target.closest('button.btn-update');
        if (updateBtn) {
            handleUpdate(updateBtn.dataset.update);
            return;
        }
        if (event.target.closest('button')) return;
        const row = event.target.closest('tr[data-idx]');
        if (!row) return;
        // Focus goes back to the button, not to a row that cannot hold it.
        openCard(Number(row.dataset.idx), row.querySelector('.row-open'));
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
                  : $searchQuery.trim()
                    ? $msg('table.noresults')
                    : $updatesOnly
                      ? $msg('table.noupdates')
                      : $msg('table.noresults'),
    );

    function badge(status) {
        const { cls, glyph, key } = badgeFor(status);
        return { cls, glyph, label: $msg(key) };
    }

    // Progress is drawn in the row that is being worked on: the phase name comes
    // from the language store, and only a download has a percentage to show.
    function rowProgress(state) {
        const percent =
            state.phase === 'download' && state.percent >= 0 ? Math.min(100, Math.round(state.percent)) : -1;
        return {
            phase: $msg('update.progress.' + (state.phase || 'download')),
            pct: percent >= 0 ? `${percent}%` : '',
            percent,
            indeterminate: percent < 0,
        };
    }

    // The header and the placeholder's colspan come from the same list, so a
    // column added here cannot quietly make a placeholder row misaligned. The type
    // is not in it: every run of rows already carries a heading naming its type.
    // Status and action share the last column: an updatable row holds its button,
    // an exception holds its badge, and an up-to-date row holds nothing.
    const columns = ['plugin', 'installed', 'latest', 'status'];
</script>

<div id="view-main" hidden={$settingsOpen}>
    {#if $offered > 0}
        <div class="update-all">
            <button
                type="button"
                class="update-filter"
                aria-pressed={$updatesOnly}
                title={$msg('update_all.filter')}
                onclick={() => updatesOnly.update((on) => !on)}
            >
                <span class="update-all-count">{$plural('update_all.count', $offered)}</span>
            </button>
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
                        <th scope="col" aria-sort={ariaSort(column)}>
                            {#if column !== 'status'}
                                <button
                                    type="button"
                                    class="th-sort"
                                    class:active={sort.key === column}
                                    title={$msg('table.sort', { column: $msg(`col.${column}`) })}
                                    onclick={() => toggleSort(column)}
                                >
                                    {$msg(`col.${column}`)}
                                    <span class="sort-arrow" aria-hidden="true">
                                        {sort.key === column ? (sort.dir === 1 ? '↑' : '↓') : ''}
                                    </span>
                                </button>
                            {:else}
                                {$msg(`col.${column}`)}
                            {/if}
                        </th>
                    {/each}
                </tr>
            </thead>
            <!-- Anywhere in a row opens the card, but as a mouse shortcut over the
                 name button in it rather than the only way in — that button is what
                 the keyboard uses. The checker is right about the row, so this
                 delegation stays ignored: one listener for every row is what keeps
                 a scan cheap. -->
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
                    {#each groups as group (group.key)}
                        <tr class="group-head">
                            <th colspan={columns.length} scope="colgroup">
                                {group.type} ·
                                <small class="group-count">{$plural('count.plugins', group.rows.length)}</small>
                            </th>
                        </tr>
                        {#each group.rows as { plugin, index } (plugin.id)}
                            {@const mark = badge(plugin.status)}
                            {@const updatable = isUpdatable(plugin)}
                            {@const released =
                                updatable && plugin.published_at ? releaseDate(plugin.published_at, $lang) : null}
                            {@const progress = $activeUpdate.id === plugin.id ? rowProgress($activeUpdate) : null}
                            <tr data-idx={index}>
                                <td>
                                    <button
                                        type="button"
                                        class="row-open"
                                        onclick={(event) => openCard(index, event.currentTarget)}
                                    >
                                        {plugin.name}
                                    </button>
                                </td>
                                <td><span class="num">{plugin.installed_version}</span></td>
                                <td>
                                    {#if plugin.latest_version}
                                        <span class="num" class:new={updatable}>{plugin.latest_version}</span>
                                        {#if released}
                                            <span class="release-date" title={released.title}>{released.text}</span>
                                        {/if}
                                    {:else}
                                        —
                                    {/if}
                                </td>
                                <td class="row-status">
                                    {#if progress}
                                        <span class="row-update" role="status">
                                            <span class="row-update-line">
                                                <span>{progress.phase}</span>
                                                <span>{progress.pct}</span>
                                            </span>
                                            <span
                                                class="row-progress-track"
                                                class:indeterminate={progress.indeterminate}
                                            >
                                                <span
                                                    class="row-progress-fill"
                                                    style:width={progress.indeterminate ? null : `${progress.percent}%`}
                                                ></span>
                                            </span>
                                        </span>
                                    {:else if updatable}
                                        <button
                                            type="button"
                                            class="btn-update"
                                            data-update={plugin.id}
                                            title={$msg('btn.update.hint')}
                                            disabled={$progressRunning}
                                        >
                                            {$msg('btn.update')}
                                        </button>
                                    {:else}
                                        <!-- Every row answers here: exceptions carry
                                             their badge, an up-to-date row a quiet
                                             one, so the column is never an
                                             unexplained void. -->
                                        <span class="badge {mark.cls}" class:quiet={plugin.status === 'up_to_date'}>
                                            {mark.glyph}
                                            {mark.label}
                                        </span>
                                    {/if}
                                </td>
                            </tr>
                        {/each}
                    {/each}
                {/if}
            </tbody>
        </table>
    </div>
</div>
