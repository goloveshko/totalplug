<script>
    import { lang, tr } from './i18n.js';
    import { aboutState, checkForAppUpdate, formatDisplayVersion, openCatalog, openReleases } from './about.js';
    import { settingsSection } from './settings.js';
    import { formatStamp, interceptLinks, safeHttpUrl } from './text.js';

    // The About panel as part of the settings island: what this build is, where its
    // data lives and whether a newer release exists — all read from one store, so
    // a check in flight cannot be wiped by a language switch.

    const state = $derived($aboutState);
    const info = $derived(state.info);

    // GitHub answers with the URL, so it is remote data like the catalog's homepage
    // field. Without a link the row still names the new version, which is the part
    // the user came to read.
    const release = $derived(safeHttpUrl(state.update?.release_url));

    // The stamp follows the UI language, not the OS locale.
    const fmt = $derived(new Intl.DateTimeFormat($lang === 'ru' ? 'ru-RU' : 'en-GB', { timeStyle: 'short' }));
</script>

<div class="settings-panel" class:active={$settingsSection === 'about'}>
    <div class="about-name">
        TotalPlug <span class="about-version" id="about-version">{formatDisplayVersion(info?.version)}</span>
    </div>
    <p class="about-desc">{$tr('about.desc')}</p>
    <div class="about-update" id="about-update" use:interceptLinks>
        <div class="about-update-row">
            <!-- The label stays put while a check runs: the status line already says
                 "checking", and a disabled button only has to stop a second request. -->
            <button
                type="button"
                class="btn-secondary"
                id="btn-app-update"
                disabled={state.busy}
                onclick={checkForAppUpdate}
            >
                {$tr('about.check')}
            </button>
        </div>
        {#if state.busy}
            <span class="about-status">{$tr('about.checking')}</span>
        {:else if state.update}
            {#if !state.update.comparable}
                <span class="about-status">{$tr('about.dev_build')}</span>
            {:else if state.update.available}
                <span class="about-status accent">{$tr('about.available', { latest: state.update.latest })}</span>
                {#if release}
                    <a href={release} class="dlg-link">{$tr('about.open_release')}</a>
                {/if}
            {:else}
                <span class="about-status">{$tr('about.up_to_date')}</span>
            {/if}
            <span class="about-checked">
                {$tr('about.checked_at', { time: formatStamp(fmt, state.update.checked_at) })}
            </span>
        {/if}
    </div>
    <div class="about-links">
        <button type="button" class="btn-secondary" id="link-catalog" onclick={openCatalog}>
            {$tr('about.catalog')}
        </button>
        <button type="button" class="btn-secondary" id="link-releases" onclick={openReleases}>
            {$tr('about.releases')}
        </button>
    </div>
    <p class="editor-hint about-privacy">{$tr('about.privacy')}</p>
    <details class="about-details">
        <summary>{$tr('about.details')}</summary>
        <div class="about-tech">
            <div class="k">{$tr('about.data_dir')}</div>
            <div class="v"><code class="path" id="about-data-dir">{info?.data_dir}</code></div>
            <div class="k">{$tr('about.build')}</div>
            <div class="v" id="about-build">{info?.go_version}</div>
        </div>
    </details>
</div>
