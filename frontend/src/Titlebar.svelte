<script>
    import { Quit, WindowIsMaximised, WindowMinimise, WindowToggleMaximise } from '../wailsjs/runtime';
    import { tr, trp } from './i18n.js';
    import { buildSuggestions } from './plugin.js';
    import {
        catalogStatus,
        pluginCount,
        runScan,
        scanRunning,
        searchQuery,
        setSearchQuery,
        updatesOnly,
    } from './catalog.js';
    import { plugins, settingsOpen } from './state.js';
    import { openCard } from './card.js';
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

    // The caption glyphs come from the system icon font — the same strokes the
    // OS caption buttons draw — and the maximise button reads the live state so
    // it can offer restore. The resize sync keeps that honest however the state
    // changed: the button, the drag-area double click, or Aero Snap.
    let maximised = $state(false);

    $effect(() => {
        const syncMaximised = () => Promise.resolve(WindowIsMaximised()).then((v) => (maximised = v));
        syncMaximised();
        window.addEventListener('resize', syncMaximised);
        return () => window.removeEventListener('resize', syncMaximised);
    });

    async function onMaximiseClick() {
        await WindowToggleMaximise();
        maximised = await Promise.resolve(WindowIsMaximised());
    }

    let search = $state(null);

    // The search box doubles as the type filter's front door: what it can
    // filter by is offered under it. Types come first — they are the
    // discoverable way to a whole group — and name matches follow; a type
    // fills the box (the table already filters on it), a name opens the card.
    const suggestions = $derived(buildSuggestions($plugins, $searchQuery));
    const suggested = $derived(suggestions.types.length + suggestions.names.length);
    let suggestOpen = $state(false);
    let suggestActive = $state(-1);

    function openSuggestions() {
        suggestOpen = true;
        suggestActive = -1;
    }

    function closeSuggestions() {
        suggestOpen = false;
        suggestActive = -1;
    }

    function pickSuggestion(item) {
        closeSuggestions();
        if (item.kind === 'type') {
            setSearchQuery(item.type);
            search?.focus();
        } else {
            openCard(item.index, search);
        }
    }

    function onSearchKey(event) {
        if (event.key === 'ArrowDown' && !suggestOpen && suggested > 0) {
            event.preventDefault();
            openSuggestions();
            suggestActive = 0;
            return;
        }
        if (!suggestOpen || suggested === 0) return;
        if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
            event.preventDefault();
            suggestActive =
                event.key === 'ArrowDown'
                    ? (suggestActive + 1) % suggested
                    : (suggestActive - 1 + suggested) % suggested;
        } else if (event.key === 'Enter' && suggestActive >= 0) {
            event.preventDefault();
            const flat = [
                ...suggestions.types.map((t) => ({ kind: 'type', ...t })),
                ...suggestions.names.map((n) => ({ kind: 'name', ...n })),
            ];
            // A background refresh can shrink the list under an open popup.
            const item = flat[suggestActive];
            if (item) pickSuggestion(item);
        } else if (event.key === 'Escape') {
            // The first Esc closes the list; the window handler's clear-the-box
            // answer stays one Esc behind it.
            event.preventDefault();
            event.stopPropagation();
            closeSuggestions();
        }
    }

    // Ctrl+F is the one shortcut a list like this is expected to have, and an
    // empty search box leaves Esc with nothing to do: while it has text, Esc
    // clears it. F5 is the rescan the table is expected to answer to. All stay
    // out of modal dialogs, which have their own keys.
    function onWindowKey(event) {
        if ($settingsOpen || document.querySelector('dialog[open]')) return;

        if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === 'f') {
            event.preventDefault();
            search?.focus();
            search?.select();
            return;
        }
        if (event.key === 'F5' && !$scanRunning) {
            event.preventDefault();
            runScan(false);
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
    <span class="search-wrap" hidden={$settingsOpen}>
        <input
            type="search"
            id="plugin-search"
            bind:this={search}
            placeholder={$tr('search.ph')}
            aria-label={$tr('search.ph')}
            autocomplete="off"
            role="combobox"
            aria-expanded={suggestOpen && suggested > 0}
            aria-controls="search-suggestions"
            aria-autocomplete="list"
            aria-activedescendant={suggestActive >= 0 ? `search-sugg-${suggestActive}` : undefined}
            value={$searchQuery}
            oninput={(event) => {
                setSearchQuery(event.target.value);
                openSuggestions();
            }}
            onfocus={openSuggestions}
            onfocusout={closeSuggestions}
            onkeydown={onSearchKey}
        />
        {#if suggestOpen && suggested > 0}
            <!-- pointerdown keeps focus in the box, so the click lands before
                 any focusout could close the list under the cursor. -->
            <ul
                class="search-suggest"
                id="search-suggestions"
                role="listbox"
                aria-label={$tr('search.ph')}
                onpointerdown={(event) => event.preventDefault()}
            >
                <!-- The options are keyboard-reachable through the combobox
                     itself: the input owns ArrowDown/Enter while the list is
                     open, which is what ARIA prescribes for a listbox — so the
                     click-only warning on each option is answered by design. -->
                {#if suggestions.types.length}
                    <li class="ss-head" role="presentation">{$tr('search.suggest.types')}</li>
                {/if}
                {#each suggestions.types as item, i (item.type)}
                    <!-- svelte-ignore a11y_click_events_have_key_events -->
                    <li
                        id={`search-sugg-${i}`}
                        role="option"
                        aria-selected={suggestActive === i}
                        class="ss-item"
                        class:active={suggestActive === i}
                        onclick={() => pickSuggestion({ kind: 'type', ...item })}
                    >
                        <span class="ss-type">{item.type}</span>
                        <span class="ss-count">{$trp('count.plugins', item.count)}</span>
                    </li>
                {/each}
                {#if suggestions.names.length}
                    <li class="ss-head" role="presentation">{$tr('search.suggest.plugins')}</li>
                {/if}
                {#each suggestions.names as item, j (item.plugin.id)}
                    <!-- svelte-ignore a11y_click_events_have_key_events -->
                    <li
                        id={`search-sugg-${suggestions.types.length + j}`}
                        role="option"
                        aria-selected={suggestActive === suggestions.types.length + j}
                        class="ss-item"
                        class:active={suggestActive === suggestions.types.length + j}
                        onclick={() => pickSuggestion({ kind: 'name', ...item })}
                    >
                        <span class="ss-name">{item.plugin.name}</span>
                        <span class="ss-kind">{item.plugin.type}</span>
                    </li>
                {/each}
            </ul>
        {/if}
    </span>
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
            {'\uE921'}
        </button>
        <button
            type="button"
            id="win-max"
            title={maximised ? $tr('win.restore') : $tr('win.max')}
            aria-label={maximised ? $tr('win.restore') : $tr('win.max')}
            onclick={onMaximiseClick}
        >
            {maximised ? '\uE923' : '\uE922'}
        </button>
        <button
            type="button"
            id="win-close"
            class="win-close"
            title={$tr('win.close')}
            aria-label={$tr('win.close')}
            onclick={Quit}
        >
            {'\uE8BB'}
        </button>
    </div>
</div>
