<script>
    import { tick } from 'svelte';
    import { lang, tr } from './i18n.js';
    import { badgeFor, isUnknownInstalled, isUpdatable } from './plugin.js';
    import { cardEditing, cardState, requestCardClose, restoreCardOpener, saveCardOverride } from './card.js';
    import { plugins, progressRunning } from './state.js';
    import { handleUpdate } from './update.js';
    import { interceptLinks, releaseDate, safeHttpUrl, shortUrl } from './text.js';

    // Live records and editing drafts must refresh independently.

    let el = $state(null);
    let changeBtn = $state(null);
    let versionInput = $state(null);
    let pendingCloses = 0;

    const cardOpen = $derived($cardState.open);
    const pluginId = $derived($cardState.plugin?.id);
    const plugin = $derived($plugins.find((p) => p.id === pluginId) ?? null);
    const sub = $derived(plugin ? plugin.type + (plugin.arch ? ' · ' + plugin.arch : '') : '');
    const site = $derived(safeHttpUrl(plugin?.web_url));
    const mark = $derived(badgeFor(plugin?.status));
    const updatable = $derived(!!plugin && isUpdatable(plugin));
    const unknown = $derived(isUnknownInstalled(plugin));
    const released = $derived(plugin?.published_at ? releaseDate(plugin.published_at, $lang) : null);

    // Which provider resolved the latest version, shown as a glyph beside it: the
    // tooltip names it, so no text row is spent on a source that for a GitHub
    // plugin only repeats the homepage — the raw key lives in the details block.
    // Each glyph paints itself: a CSS `fill` would override the markup attribute.
    const github = $derived(/^github:/i.test(plugin?.source || ''));
    const srcLabel = $derived(github ? 'GitHub' : $tr('card.source.catalog'));

    function onSubmit(event) {
        // method="dialog" would close the card before the async save finishes.
        event.preventDefault();
        saveCardOverride();
    }

    // The card closes first so the row's own progress is what the user watches;
    // the update itself is looked up by id and survives the card going away.
    function onUpdate() {
        const id = plugin?.id;
        if (!id) return;
        requestCardClose();
        handleUpdate(id);
    }

    // Change is removed together with the actions row, so the button that has to
    // take the focus back does not exist yet at the moment the flag flips.
    async function closeEditor() {
        cardEditing.set(false);
        await tick();
        if (cardOpen && el?.open) changeBtn?.focus();
    }

    function onCancel(event) {
        event.preventDefault();
        requestCardClose();
    }

    function onClose() {
        // A queued close event must not reset a newly opened card.
        if (pendingCloses > 0) {
            pendingCloses--;
            if (cardOpen || el?.open) return;
        } else if (el?.open) return;
        restoreCardOpener();
    }

    // A dialog enters the top layer only when it is asked, so the store's `open`
    // drives showModal() and close(), exactly like the other two overlays. Focus
    // starts on Change rather than on Update: an accidental Enter must not install
    // anything. The one exception is a version we could not read — the editor is
    // already open there, and the field is the only way forward.
    $effect(() => {
        if (!el) return;
        if (cardOpen) {
            if (!el.open) {
                el.showModal();
                ($cardEditing ? versionInput : changeBtn)?.focus();
            }
        } else if (el.open) {
            pendingCloses++;
            el.close();
        }
    });

    // Primitive flags keep draft writes from refocusing the version field.
    $effect(() => {
        if (cardOpen && $cardEditing) versionInput?.focus();
    });
</script>

<dialog id="plugin-card" bind:this={el} aria-labelledby="card-name" oncancel={onCancel} onclose={onClose}>
    <button
        type="button"
        class="dlg-close"
        id="card-close"
        title={$tr('dialog.close')}
        aria-label={$tr('dialog.close')}
        onclick={requestCardClose}
    >
        <svg viewBox="0 0 24 24">
            <line x1="18" y1="6" x2="6" y2="18" />
            <line x1="6" y1="6" x2="18" y2="18" />
        </svg>
    </button>
    {#if plugin}
        <h2 id="card-name">{plugin.name}</h2>
        <div class="card-sub" id="card-sub">{sub}</div>
        <div class="card-info" id="card-info">
            <div class="k">{$tr('col.installed')}</div>
            <div class="v">
                <span class="num">{plugin.installed_version}</span>
                <small class="muted">{plugin.confidence}</small>
            </div>
            <div class="k">{$tr('col.latest')}</div>
            <div class="v">
                {#if plugin.latest_version}
                    <span class="num" class:new={updatable}>{plugin.latest_version}</span>
                    {#if released}
                        <span class="release-date" title={released.title}>{released.text}</span>
                    {/if}
                    <span class="src-icon" role="img" title={srcLabel} aria-label={srcLabel}>
                        {#if github}
                            <!-- octicons/mark-github-16 (MIT, (c) GitHub Inc.) -->
                            <svg viewBox="0 0 16 16">
                                <path
                                    fill="currentColor"
                                    d="M6.766 11.328c-2.063-.25-3.516-1.734-3.516-3.656 0-.781.281-1.625.75-2.188-.203-.515-.172-1.609.063-2.062.625-.078 1.468.25 1.968.703.594-.187 1.219-.281 1.985-.281.765 0 1.39.094 1.953.265.484-.437 1.344-.765 1.969-.687.218.422.25 1.515.046 2.047.5.593.766 1.39.766 2.203 0 1.922-1.453 3.375-3.547 3.64.531.344.89 1.094.89 1.954v1.625c0 .468.391.734.86.547C13.781 14.359 16 11.53 16 8.03 16 3.61 12.406 0 7.984 0 3.563 0 0 3.61 0 8.031a7.88 7.88 0 0 0 5.172 7.422c.422.156.828-.125.828-.547v-1.25c-.219.094-.5.156-.75.156-1.031 0-1.64-.562-2.078-1.609-.172-.422-.36-.672-.719-.719-.187-.015-.25-.093-.25-.187 0-.188.313-.328.625-.328.453 0 .844.281 1.25.86.313.452.64.655 1.031.655s.641-.14 1-.5c.266-.265.47-.5.657-.656"
                                />
                            </svg>
                        {:else}
                            <!-- Stand-in until the catalog ships its own mark. -->
                            <svg viewBox="0 0 24 24">
                                <g fill="none" stroke="currentColor" stroke-width="2">
                                    <rect x="4" y="4" width="7" height="7" rx="1.5" />
                                    <rect x="13" y="4" width="7" height="7" rx="1.5" />
                                    <rect x="4" y="13" width="7" height="7" rx="1.5" />
                                    <rect x="13" y="13" width="7" height="7" rx="1.5" />
                                </g>
                            </svg>
                        {/if}
                    </span>
                {:else}
                    —
                {/if}
            </div>
            <div class="k">{$tr('col.status')}</div>
            <div class="v"><span class="badge {mark.cls}">{mark.glyph} {$tr(mark.key)}</span></div>
            {#if site}
                <div class="k">{$tr('card.site')}</div>
                <div class="v"><a href={site} class="dlg-link" use:interceptLinks>{shortUrl(site)}</a></div>
            {/if}
            <!-- Raw source key and on-disk path: long, rarely read, same disclosure
                 as the error dialog. -->
            <details class="card-tech">
                <summary>{$tr('card.details')}</summary>
                <div class="card-tech-body">
                    {#if plugin.source}
                        <div class="k">{$tr('card.source')}</div>
                        <div class="v"><code>{plugin.source}</code></div>
                    {/if}
                    <div class="k">{$tr('card.path')}</div>
                    <div class="v"><code class="path">{plugin.path}</code></div>
                </div>
            </details>
        </div>
        {#if unknown}
            <p class="card-hint">{$tr('card.unknown_hint')}</p>
        {/if}
        {#if $cardEditing}
            <div class="card-edit-title">{$tr('editor.title')}</div>
            <form method="dialog" id="editor-form" onsubmit={onSubmit}>
                <label>
                    {$tr('editor.version')}
                    <input
                        type="text"
                        id="editor-version"
                        bind:this={versionInput}
                        placeholder={$tr('editor.version.ph')}
                        autocomplete="off"
                        bind:value={$cardState.version}
                    />
                </label>
                <label>
                    {$tr('editor.source')}
                    <input
                        type="text"
                        id="editor-source"
                        placeholder={$tr('editor.source.ph')}
                        autocomplete="off"
                        bind:value={$cardState.source}
                    />
                </label>
                <p class="editor-hint">{$tr('editor.hint')}</p>
                <div class="editor-actions">
                    <button type="submit" class="btn-save" id="editor-save" disabled={$cardState.saving}>
                        {$tr('editor.save')}
                    </button>
                    <button type="button" id="editor-cancel" onclick={closeEditor}>{$tr('editor.cancel')}</button>
                </div>
            </form>
        {:else}
            <div class="card-actions">
                {#if updatable}
                    <button
                        type="button"
                        class="btn-save"
                        id="card-update"
                        disabled={$progressRunning}
                        onclick={onUpdate}
                    >
                        {$tr('card.update', { version: plugin.latest_version })}
                    </button>
                {/if}
                <button
                    type="button"
                    class="btn-secondary"
                    id="card-change"
                    bind:this={changeBtn}
                    onclick={() => cardEditing.set(true)}
                >
                    {$tr('card.change')}
                </button>
            </div>
        {/if}
    {/if}
</dialog>
