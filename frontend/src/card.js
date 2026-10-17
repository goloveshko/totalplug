import { SaveOverride } from '../wailsjs/go/gui/App';
import { get, writable } from 'svelte/store';
import { t } from './i18n.js';
import { plugins } from './state.js';
import { normalizeSource, displaySource } from './plugin.js';
import { openAppDialog, errorDialogHtml } from './dialog.js';
import { runScan } from './catalog.js';

// The plugin card: one dialog that shows everything a record says about itself
// and edits the two fields the user may override. PluginCard.svelte draws it;
// this module owns the record it points at, the drafts being edited, the element
// that opened it and the save, because none of those belong to a component.

// `version` and `source` are the drafts of the two fields, seeded from the record
// when the card opens. They live here rather than in the component so a language
// switch — which re-renders it — cannot lose what the user typed.
export const cardState = writable({ open: false, plugin: null, version: '', source: '', saving: false });

// The control that opened the card. A row is not focusable, so the table hands
// over the name button inside it; closing puts the keyboard back on it.
let cardOpener = null;

export function openCard(index, opener) {
    const plugin = get(plugins)[index];
    if (!plugin) return;

    // Remembered before the dialog opens: while a modal is up the rest of the
    // document is inert, so focus can only be restored once it is gone.
    cardOpener = opener || null;
    cardState.set({
        open: true,
        plugin,
        version: autoVersion(plugin),
        source: displaySource(plugin.custom_source),
        saving: false,
    });
}

export const requestCardClose = () => cardState.update((state) => ({ ...state, open: false }));

export function restoreCardOpener() {
    // A background refresh can repaint the table while the card is open, so the
    // opener may be detached by now; focusing such an element does nothing.
    cardOpener?.focus();
    cardOpener = null;
}

// What to prefill the version field with. A row whose installed version was only
// guessed at reads "Unknown" or "n/a", and saving that would pin a manual override
// over the catalog value the user never typed, so such a field starts empty.
function autoVersion(plugin) {
    const auto = plugin.installed_version || '';
    return /^(unknown|n\/a)/i.test(auto) ? '' : auto;
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
