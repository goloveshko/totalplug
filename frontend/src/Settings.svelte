<script>
    import { lang, tr } from './i18n.js';
    import { closeSettings, settingsSection } from './settings.js';
    import { settingsOpen } from './state.js';
    import { scale, theme } from './theme.js';
    import AboutPanel from './AboutPanel.svelte';
    import BackupsPanel from './BackupsPanel.svelte';
    import TcPanel from './TcPanel.svelte';

    // The settings view as an island: which panel is on screen, which theme and
    // language are in force and how large the font runs all live in stores, so a
    // language switch re-labels this screen in place instead of rebuilding it —
    // which is what lets a half-typed path or an unsaved policy survive it.

    const SECTIONS = ['appearance', 'tc', 'backups', 'language', 'about'];

    // The three preview swatches are pure CSS, so a card carries only its choice.
    const THEMES = [
        { choice: 'light', name: 'settings.theme.light', desc: 'settings.theme.light.desc' },
        { choice: 'dark', name: 'settings.theme.dark', desc: 'settings.theme.dark.desc' },
        { choice: 'system', name: 'settings.theme.system', desc: 'settings.theme.system.desc' },
    ];
</script>

<div id="view-settings" hidden={!$settingsOpen}>
    <div class="settings-top">
        <button type="button" id="settings-back" class="btn-back" onclick={closeSettings}>
            {$tr('settings.back')}
        </button>
        <h2>{$tr('settings.title')}</h2>
        <span class="settings-save-hint">{$tr('settings.save_hint')}</span>
    </div>
    <div class="settings-body">
        <div class="settings-nav">
            {#each SECTIONS as section}
                <button
                    type="button"
                    class:active={$settingsSection === section}
                    onclick={() => settingsSection.set(section)}
                >
                    {$tr(`settings.nav.${section}`)}
                </button>
            {/each}
        </div>
        <div class="settings-panel" class:active={$settingsSection === 'appearance'}>
            <h3>{$tr('settings.nav.appearance')}</h3>
            <p class="section-desc">{$tr('settings.appearance')}</p>
            <div class="section-label">{$tr('settings.theme')}</div>
            <div class="theme-cards">
                {#each THEMES as card}
                    {@const selected = $theme === card.choice}
                    <!-- Real buttons, so Enter/Space reach the handler through the native click. -->
                    <button
                        type="button"
                        class="theme-card"
                        class:selected
                        aria-pressed={String(selected)}
                        onclick={() => theme.set(card.choice)}
                    >
                        <span class="theme-preview preview-{card.choice}"></span>
                        <span class="name">{$tr(card.name)}</span>
                        <span class="desc">{$tr(card.desc)}</span>
                    </button>
                {/each}
            </div>
            <div class="section-label">{$tr('settings.font')}</div>
            <div class="font-row">
                <input
                    type="range"
                    id="font-scale"
                    min="80"
                    max="140"
                    step="5"
                    aria-label={$tr('settings.font')}
                    bind:value={$scale}
                />
                <span class="font-value" id="font-scale-value">{$scale}%</span>
            </div>
        </div>
        <TcPanel />
        <BackupsPanel />
        <div class="settings-panel" class:active={$settingsSection === 'language'}>
            <h3>{$tr('settings.nav.language')}</h3>
            <p class="section-desc">{$tr('settings.language')}</p>
            <!-- Each name is written in its own language on purpose: an English
                 interface still has to say what «Русский» is. -->
            <select id="lang-select" aria-label={$tr('settings.language')} bind:value={$lang}>
                <option value="en">English</option>
                <option value="ru">Русский</option>
            </select>
        </div>
        <AboutPanel />
    </div>
</div>
