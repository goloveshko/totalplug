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

    // Dismissed any other way than by a button — Esc, or the top layer closing on
    // its own — means the question was not answered, which is a no. The answer is
    // settled before the focus moves, and a dialog a button already answered finds
    // nothing left to settle here.
    function onClose() {
        answerDialog(false);
        restoreOpener();
    }

    // A dialog only enters the top layer when it is asked, so the store's `open`
    // drives showModal() and close(). Enter must never confirm an update by
    // accident, which is why the safe button takes the focus once the dialog is up.
    $effect(() => {
        if (!el) return;
        if ($dialogState.open) {
            if (!el.open) {
                el.showModal();
                ($dialogState.isConfirm ? cancel : ok).focus();
            }
        } else if (el.open) {
            el.close();
        }
    });
</script>

<dialog
    id="app-dialog"
    bind:this={el}
    aria-label={$tr($dialogState.isConfirm ? 'dialog.title.confirm' : 'dialog.title.message')}
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
