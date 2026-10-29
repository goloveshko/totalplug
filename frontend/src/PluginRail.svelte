<script>
    import { cardEditing, cardState, requestCardClose, restoreCardOpener } from './card.js';
    import { tr } from './i18n.js';
    import { wideLayout } from './layout.js';
    import CardBody from './CardBody.svelte';

    // The card as a rail: the shape it takes on a wide window, where it stands
    // beside the table instead of over it. The rail is not a modal, so the
    // keyboard leaves it with Escape and the focus goes back to the row the
    // card was opened from.

    let body = $state(null);
    let seen = false;

    const cardOpen = $derived($cardState.open);

    function onKeydown(event) {
        if (!$wideLayout || !cardOpen) return;
        if (event.key !== 'Escape') return;
        event.preventDefault();
        requestCardClose();
    }

    // The card opens and closes while the rail stays where it is, so the effect
    // watches the flag and acts on the change, not on every store write.
    $effect(() => {
        if (cardOpen === seen) return;
        seen = cardOpen;
        if (cardOpen) {
            ($cardEditing ? body?.focusVersion : body?.focusChange)?.();
        } else {
            restoreCardOpener();
        }
    });
</script>

<!-- Escape closes the rail from anywhere in the window: the rail is not a
     modal, so it cannot lean on the browser's own Escape handling. -->
<svelte:window onkeydown={onKeydown} />

{#if $wideLayout}
    <aside class="card-rail" hidden={!cardOpen} aria-label={$tr('card.rail')}>
        <button
            type="button"
            class="dlg-close"
            id="rail-close"
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
    </aside>
{/if}
