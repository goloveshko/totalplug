<script>
    import { tr } from './i18n.js';
    import { applyBackupPolicy, backupState, loadBackupSettings } from './backups.js';
    import { settingsSection } from './settings.js';
    import { settingsOpen } from './state.js';

    // The backups panel edits the same settings file the command line reads, so
    // the two numbers live in a store: the fields keep what was typed until a
    // write actually lands, and a visit to another panel cannot lose it.

    const state = $derived($backupState);

    // The reason and the fact that nothing was stored, named here so the module
    // can stay out of the wording.
    const failure = $derived(state.error ? `${state.error} — ${$tr('settings.backups.not_saved')}` : '');

    // Same entry rule as the TC panel: the settings file is also written by the
    // command line, so entering the panel asks for the numbers again.
    $effect(() => {
        if ($settingsOpen && $settingsSection === 'backups') loadBackupSettings();
    });

    function onEnter(event) {
        if (event.key !== 'Enter') return;
        event.preventDefault();
        applyBackupPolicy();
    }
</script>

<div class="settings-panel" class:active={$settingsSection === 'backups'}>
    <h3>{$tr('settings.nav.backups')}</h3>
    <p class="section-desc">{$tr('settings.backups')}</p>
    <div class="section-label">{$tr('settings.backups.keep')}</div>
    <!-- Both numbers travel together: a policy is one answer, so one field cannot
         save without the other. -->
    <input
        type="number"
        id="backup-keep"
        min="0"
        max="99"
        step="1"
        aria-label={$tr('settings.backups.keep')}
        bind:value={$backupState.keep}
        onchange={applyBackupPolicy}
        onkeydown={onEnter}
    />
    <div class="section-label">{$tr('settings.backups.days')}</div>
    <input
        type="number"
        id="backup-days"
        min="0"
        max="3650"
        step="1"
        aria-label={$tr('settings.backups.days')}
        bind:value={$backupState.days}
        onchange={applyBackupPolicy}
        onkeydown={onEnter}
    />
    <p class="editor-hint">{$tr('settings.backups.hint')}</p>
    <div class="section-label">{$tr('settings.backups.location')}</div>
    <code class="backup-path">{state.location}</code>
    <div class="tc-state">
        {#if failure}
            <span class="tc-warn">{failure}</span>
        {/if}
    </div>
</div>
