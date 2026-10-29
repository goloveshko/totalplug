<script>
    import { cardEditing, cardState, requestCardClose, restoreCardOpener } from './card.js';
    import { tr } from './i18n.js';
    import { wideLayout } from './layout.js';
    import CardBody from './CardBody.svelte';

    // The card as a modal dialog: the shape it takes on a narrow window. A wide
    // window gets the rail instead, because a modal makes the rest of the
    // document inert and the table could not be clicked while the card was up.

    let el = $state(null);
    let body = $state(null);
    let pendingCloses = 0;

    const cardOpen = $derived($cardState.open);

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
                ($cardEditing ? body?.focusVersion : body?.focusChange)?.();
            }
        } else if (el.open) {
            pendingCloses++;
            el.close();
        }
    });
</script>

{#if !$wideLayout}
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
        <CardBody bind:this={body} />
    </dialog>
{/if}
