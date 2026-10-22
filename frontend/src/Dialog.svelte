<script>
    import { tr } from './i18n.js';
    import { answerDialog, dialogState, restoreOpener } from './dialog.js';
    import { interceptLinks } from './text.js';

    // The one confirmation/message dialog, as an island that never rebuilds. The
    // message itself is HTML its caller built, so nothing here has to be
    // re-labelled on a language switch except the three words the dialog owns:
    // Close, OK and Cancel.

    let el = $state(null);
    let ok = $state(null);
    let cancel = $state(null);
    let pendingCloses = 0;

    // Prevent native closure so Escape advances the queue inside the same modal.
    function onCancel(event) {
        event.preventDefault();
        answerDialog(false);
    }

    function onClose() {
        // A queued close event must not answer a newly opened request.
        if (pendingCloses > 0) {
            pendingCloses--;
            if (!el?.open) restoreOpener();
            return;
        }
        if (el?.open) return;
        answerDialog(false);
        restoreOpener();
    }

    // Each queued request resets focus to the safe answer without closing the modal.
    $effect(() => {
        if (!el) return;
        if ($dialogState.open) {
            if (!el.open) el.showModal();
            ($dialogState.isConfirm ? cancel : ok)?.focus();
        } else if (el.open) {
            pendingCloses++;
            el.close();
        }
    });
</script>

<dialog
    id="app-dialog"
    bind:this={el}
    aria-label={$tr($dialogState.isConfirm ? 'dialog.title.confirm' : 'dialog.title.message')}
    oncancel={onCancel}
    onclose={onClose}
>
    <button
        type="button"
        class="dlg-close"
        id="app-dialog-close"
        title={$tr('dialog.close')}
        aria-label={$tr('dialog.close')}
        onclick={() => answerDialog(false)}
    >
        <svg viewBox="0 0 24 24">
            <line x1="18" y1="6" x2="6" y2="18" />
            <line x1="6" y1="6" x2="18" y2="18" />
        </svg>
    </button>
    <div class="dlg-message" id="app-dialog-msg" use:interceptLinks>{@html $dialogState.html}</div>
    <div class="editor-actions">
        <button type="button" class="btn-save" id="app-dialog-ok" bind:this={ok} onclick={() => answerDialog(true)}>
            {$tr('dialog.ok')}
        </button>
        <button
            type="button"
            id="app-dialog-cancel"
            bind:this={cancel}
            hidden={!$dialogState.isConfirm}
            onclick={() => answerDialog(false)}
        >
            {$tr('dialog.cancel')}
        </button>
    </div>
</dialog>
