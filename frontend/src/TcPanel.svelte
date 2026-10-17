<script>
    import { tr } from './i18n.js';
    import { settingsSection } from './settings.js';
    import { settingsOpen } from './state.js';
    import { applyTcPath, browseTcIni, loadTcConfig, tcState } from './tc.js';

    // The Total Commander panel as part of the settings island: the answer the Go
    // side gives and the path being edited both live in a store, so the field
    // survives a visit to another panel and the state line re-labels on its own.

    const state = $derived($tcState);

    // One line describing what the path in force is, chosen from the same fields
    // the old render function read. `warn` marks the cases a scan would fail on.
    const note = $derived.by(() => {
        if (state.error) return { cls: 'tc-warn', text: state.error };
        const config = state.config;
        if (!config) return null;
        if (!config.effective) return { cls: 'tc-warn', text: $tr('settings.tc.none') };
        if (!config.readable) return { cls: 'tc-warn', text: $tr('settings.tc.gone') };
        return { cls: 'tc-mode', text: $tr(config.configured ? 'settings.tc.mode.manual' : 'settings.tc.mode.auto') };
    });

    // Detection can move while the app runs (a portable TC mounted, an install
    // uninstalled), and asking is two stat calls, so the panel is refreshed on
    // entry rather than trusted from the last visit — and entering means either
    // clicking to it or opening settings on it.
    $effect(() => {
        if ($settingsOpen && $settingsSection === 'tc') loadTcConfig();
    });

    function onEnter(event) {
        // Leaving the field saves it, and Enter counts as leaving: typing an
        // absolute Windows path and pressing Enter is how everyone expects it.
        if (event.key !== 'Enter') return;
        event.preventDefault();
        applyTcPath($tcState.path);
    }
</script>

<div class="settings-panel" class:active={$settingsSection === 'tc'}>
    <h3>{$tr('settings.nav.tc')}</h3>
    <p class="section-desc">{$tr('settings.tc')}</p>
    <div class="section-label">{$tr('settings.tc.ini')}</div>
    <div class="tc-row">
        <input
            type="text"
            id="tc-ini"
            aria-label={$tr('settings.tc.ini')}
            autocomplete="off"
            spellcheck="false"
            bind:value={$tcState.path}
            onchange={() => applyTcPath($tcState.path)}
            onkeydown={onEnter}
        />
        <button type="button" class="btn-secondary" id="tc-browse" onclick={browseTcIni}>
            {$tr('settings.tc.browse')}
        </button>
    </div>
    <div class="tc-state">
        {#if note}
            <span class={note.cls}>{note.text}</span>
        {/if}
        <!-- The way out of a manual path is offered wherever one is stored, healthy
             or not: it is the only control that returns the panel to detection. -->
        {#if state.config?.configured}
            <button type="button" class="btn-secondary" onclick={() => applyTcPath('')}>
                {$tr('settings.tc.back_auto')}
            </button>
        {/if}
    </div>
    <p class="editor-hint">{$tr('settings.tc.hint')}</p>
</div>
