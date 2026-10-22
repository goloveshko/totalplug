<script>
    import { Quit, WindowMinimise, WindowToggleMaximise } from '../wailsjs/runtime';
    import { tr, trp } from './i18n.js';
    import {
        catalogStatus,
        pluginCount,
        runScan,
        scanRunning,
        searchQuery,
        setSearchQuery,
        updatesOnly,
    } from './catalog.js';
    import { settingsOpen } from './state.js';
    import { openHistory } from './history.js';
    import { openSettings } from './settings.js';

    // The frameless window's chrome, as one island that never rebuilds: labels
    // follow the language store, so switching language re-labels this row in
    // place and the search box keeps both its text and the keyboard focus.

    const catalogStateLabels = {
        cache: 'refresh.state.cache',
        fresh: 'refresh.state.fresh',
        error: 'refresh.state.error',
    };

    // The freshness dot lives in title + aria-label, not only in the tooltip:
    // a tooltip alone is mouse-only.
    let refresh = $derived.by(() => {
        const { state: key, detail } = $catalogStatus;
        const base = $tr('btn.check');
        if (!key) return { dot: undefined, title: base, aria: base };
        const label = $tr(catalogStateLabels[key]);
        return {
            dot: key,
            title: detail ? `${base} — ${label}: ${detail}` : `${base} — ${label}`,
            aria: `${base} — ${label}`,
        };
    });

    // A filtered pair is language-neutral; the plain count needs the localized
    // plural form («2 плагина» instead of a fixed «плагинов»).
    let count = $derived.by(() => {
        const { shown, total } = $pluginCount;
        const filtered = ($searchQuery.trim() !== '' || $updatesOnly) && shown !== total;
        return {
            text: filtered ? `${shown}/${total}` : $trp('count.plugins', total),
            hidden: total === 0 || $settingsOpen,
        };
    });

    // Double-clicking the search box (to select a word) or any button must not
    // maximise the window — only the empty drag area does.
    function onTitlebarDblclick(event) {
        if (event.target.closest('input, button')) return;
        WindowToggleMaximise();
    }

    let search = $state(null);

    // Ctrl+F is the one shortcut a list like this is expected to have, and an
    // empty search box leaves Esc with nothing to do: while it has text, Esc
    // clears it. Both stay out of modal dialogs, which have their own Esc.
    function onWindowKey(event) {
        if ($settingsOpen || document.querySelector('dialog[open]')) return;

        if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === 'f') {
            event.preventDefault();
            search?.focus();
            search?.select();
            return;
        }
        if (event.key === 'Escape' && (document.activeElement === search || $searchQuery !== '')) {
            event.preventDefault();
            setSearchQuery('');
        }
    }
</script>

<svelte:window onkeydown={onWindowKey} />

<div class="titlebar" id="titlebar" role="presentation" ondblclick={onTitlebarDblclick}>
    <div class="titlebar-left">
        <span class="titlebar-title">TotalPlug</span>
        <span class="titlebar-count" id="plugin-count" hidden={count.hidden}>{count.text}</span>
    </div>
    <input
        type="search"
        id="plugin-search"
        bind:this={search}
        placeholder={$tr('search.ph')}
        aria-label={$tr('search.ph')}
        autocomplete="off"
        value={$searchQuery}
        oninput={(event) => setSearchQuery(event.target.value)}
        hidden={$settingsOpen}
    />
    <div class="titlebar-actions" id="titlebar-actions" hidden={$settingsOpen}>
        <button
            type="button"
            id="btn-history"
            class="icon-btn"
            title={$tr('btn.history')}
            aria-label={$tr('btn.history')}
            onclick={openHistory}
        >
            <svg viewBox="0 0 24 24">
                <circle cx="12" cy="12" r="10" />
                <polyline points="12 6 12 12 16 14" />
            </svg>
        </button>
        <button
            type="button"
            id="btn-check"
            class="icon-btn"
            data-catalog={refresh.dot}
            title={refresh.title}
            aria-label={refresh.aria}
            disabled={$scanRunning}
            onclick={() => runScan(false)}
        >
            <svg viewBox="0 0 24 24">
                <polyline points="23 4 23 10 17 10" />
                <polyline points="1 20 1 14 7 14" />
                <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
            </svg>
        </button>
        <button
            type="button"
            id="btn-settings"
            class="icon-btn"
            title={$tr('btn.settings')}
            aria-label={$tr('btn.settings')}
            onclick={openSettings}
        >
            <svg viewBox="0 0 24 24">
                <circle cx="12" cy="12" r="3" />
                <path
                    d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"
                />
            </svg>
        </button>
    </div>
    <div class="caption-buttons">
        <button type="button" id="win-min" title={$tr('win.min')} aria-label={$tr('win.min')} onclick={WindowMinimise}>
            &#8722;
        </button>
        <button
            type="button"
            id="win-max"
            title={$tr('win.max')}
            aria-label={$tr('win.max')}
            onclick={WindowToggleMaximise}
        >
            &#9633;
        </button>
        <button
            type="button"
            id="win-close"
            class="win-close"
            title={$tr('win.close')}
            aria-label={$tr('win.close')}
            onclick={Quit}
        >
            &#10005;
        </button>
    </div>
</div>
