import { SaveOverride } from '../wailsjs/go/gui/App';
import { get, writable } from 'svelte/store';
import { t } from './i18n.js';
import { plugins } from './state.js';
import { normalizeSource, displaySource, isUnknownInstalled, pinnedEntryId, sourceChoices } from './plugin.js';
import { openAppDialog, errorDialogHtml } from './dialog.js';
import { runScan } from './catalog.js';

// The plugin card: one dialog that shows everything a record says about itself
// and edits the two fields the user may override. PluginCard.svelte draws it;
// this module owns the record it points at, the drafts being edited, the element
// that opened it and the save, because none of those belong to a component.

// `version` and `source` are the drafts of the two fields, seeded from the record
// when the card opens. They live here rather than in the component so a language
// switch — which re-renders it — cannot lose what the user typed.
//
// `sourceChoice` is what the source picker shows: '' for the automatic match, a
// 'catalog:<id>' pin, or 'custom' for an address of the user's own. It is kept
// apart from the source itself, because the picker and the address field are
// two ways to write one value, and the field must survive switching between
// them. `customSource` is the text in the address field.
export const cardState = writable({
    open: false,
    plugin: null,
    version: '',
    source: '',
    sourceChoice: '',
    customSource: '',
    saving: false,
});

// Whether the override editor is unfolded. Separate from cardState on purpose:
// typing a draft writes cardState, and an editing flag living there would make
// the component's focus effect re-run on every keystroke and drag the caret back
// into the version field.
export const cardEditing = writable(false);

// The control that opened the card. A row is not focusable, so the table hands
// over the name button inside it; closing puts the keyboard back on it.
let cardOpener = null;

export function openCard(index, opener) {
    const plugin = get(plugins)[index];
    if (!plugin) return;

    // Remembered before the dialog opens: while a modal is up the rest of the
    // document is inert, so focus can only be restored once it is gone.
    cardOpener = opener || null;
    // An unreadable installed version is the one case where the version field is
    // where the user has to start, so the editor opens by itself.
    cardEditing.set(isUnknownInstalled(plugin));
    // A source override may name a catalogue entry ("catalog:<id>"): the picker
    // shows that entry, and an entry the catalogue no longer holds is shown as
    // it is rather than dropped.
    const source = displaySource(plugin.custom_source);
    const pinned = pinnedEntryId(source);
    cardState.set({
        open: true,
        plugin,
        version: autoVersion(plugin),
        source,
        sourceChoice: pinned ? source : source ? 'custom' : '',
        customSource: pinned ? '' : source,
        saving: false,
    });
}

export const requestCardClose = () => cardState.update((state) => ({ ...state, open: false }));

export function restoreCardOpener() {
    // Resetting the editor before the open flag can reopen a natively closed card.
    requestCardClose();
    cardEditing.set(false);
    // A background refresh can repaint the table while the card is open, so the
    // opener may be detached by now; focusing such an element does nothing.
    const opener = cardOpener;
    cardOpener = null;
    opener?.focus();
}

// What to prefill the version field with. A row whose installed version was only
// guessed at reads "Unknown" or "n/a", and saving that would pin a manual override
// over the catalog value the user never typed, so such a field starts empty.
function autoVersion(plugin) {
    const auto = plugin.installed_version || '';
    return /^(unknown|n\/a)/i.test(auto) ? '' : auto;
}

// The picker writes into the same draft the address field writes into: one
// value, so saving has nothing to reconcile.
export function chooseSource(choice) {
    cardState.update((state) => {
        if (choice === 'custom') {
            return { ...state, sourceChoice: 'custom', source: state.customSource };
        }
        // A catalogue entry or the automatic match. The address the user had
        // typed is kept, so switching back to it does not lose the text.
        return {
            ...state,
            sourceChoice: choice,
            customSource: state.sourceChoice === 'custom' ? state.source : state.customSource,
            source: choice,
        };
    });
}

// The address field: what is typed there is the source, and the picker follows
// it — a typed entry id names that entry.
export function setCustomSource(text) {
    cardState.update((state) => {
        const raw = text || '';
        const value = raw.trim();
        const pinned = pinnedEntryId(value);
        const known = pinned && sourceChoices(state.plugin).some((c) => c.id === pinned);
        return {
            ...state,
            source: raw,
            customSource: raw,
            // An empty field resets the manual override, so the picker returns
            // to the automatic match with it.
            sourceChoice: value === '' ? '' : known ? value : 'custom',
        };
    });
}

// A failed save leaves the card open with the drafts intact: the point of an
// override is that the user knows what they meant to write.
export async function saveCardOverride() {
    const { plugin, version, source } = get(cardState);
    if (!plugin) return;

    cardState.update((state) => ({ ...state, saving: true }));
    try {
        await SaveOverride(plugin.id, {
            custom_version: version.trim(),
            custom_source: normalizeSource(source),
        });
        requestCardClose();
        runScan(false);
    } catch (err) {
        openAppDialog(errorDialogHtml(err, t('editor.save_friendly')));
    } finally {
        cardState.update((state) => ({ ...state, saving: false }));
    }
}
