<script>
    import { get } from 'svelte/store';
    import { tr } from './i18n.js';
    import { closeProgress, progressState, settleProgress } from './progress.js';
    import { progressRunning } from './state.js';
    import { interceptLinks } from './text.js';

    // The updater's progress bar and its final result, in the same dialog, as an
    // island that never rebuilds: every value it shows is a store field, so the
    // phase name, the source label and the two buttons follow the language store.

    let el = $state(null);
    let ok = $state(null);

    // The source line is built here rather than stored: «from» is a translation,
    // so an update still in flight re-labels it when the language changes.
    const source = $derived($progressState.url ? `${$tr('update.progress.source')}: ${$progressState.url}` : '');

    // The same pair of DOM-only calls as the other dialog: `open` puts the
    // progress on screen, and closing it is how the caller learns the result was read.
    $effect(() => {
        if (!el) return;
        if ($progressState.open) {
            if (!el.open) el.showModal();
        } else if (el.open) {
            el.close();
        }
    });

    // finishProgress() swaps the bar for the result, and the acknowledgement is
    // the only control left: a keyboard user has to land on it rather than on the
    // one that just disappeared. Plain variable on purpose — a store here would
    // make this effect depend on its own write.
    let wasRunning = get(progressRunning);
    $effect(() => {
        const running = $progressRunning;
        if (wasRunning && !running && $progressState.open) ok?.focus();
        wasRunning = running;
    });

    // Mid-update there is nothing to acknowledge yet, so Esc is refused.
    function onCancel(event) {
        if (get(progressRunning)) event.preventDefault();
    }
</script>

<dialog id="update-progress" bind:this={el} oncancel={onCancel} onclose={settleProgress} use:interceptLinks>
    <button
        type="button"
        class="dlg-close"
        id="progress-close"
        title={$tr('dialog.close')}
        aria-label={$tr('dialog.close')}
        hidden={$progressRunning}
        onclick={closeProgress}
    >
        <svg viewBox="0 0 24 24">
            <line x1="18" y1="6" x2="6" y2="18" />
            <line x1="6" y1="6" x2="18" y2="18" />
        </svg>
    </button>
    <h2 id="progress-title">{$progressState.title}</h2>
    <div id="progress-live" hidden={$progressRunning}>
        <div class="progress-track" id="progress-track" class:indeterminate={$progressState.indeterminate}>
            <div class="progress-fill" id="progress-fill" style:width="{$progressState.width}%"></div>
        </div>
        <!-- The percent is its own aria-hidden span: a live region that announced
             every download tick would talk over itself, so only the phase name is
             worth announcing. The space between the two spans belongs to this
             markup, which is why no stored value starts with one. -->
        <div class="progress-phase">
            <span id="progress-phase" role="status">{$progressState.phase}</span>
            <span id="progress-pct" aria-hidden="true">{$progressState.pct}</span>
        </div>
        <div class="progress-url" id="progress-url" title={$progressState.url}>{source}</div>
    </div>
    <div class="dlg-message" role="status" hidden={!$progressRunning}>{@html $progressState.resultHtml}</div>
    <div class="editor-actions">
        <button
            type="button"
            class="btn-save"
            id="progress-ok"
            bind:this={ok}
            hidden={$progressRunning}
            onclick={closeProgress}
        >
            {$tr('dialog.ok')}
        </button>
    </div>
</dialog>
