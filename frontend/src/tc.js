import { GetTcConfig, SetTcConfig, BrowseTcIni } from '../wailsjs/go/gui/App';
import { get, writable } from 'svelte/store';
import { t } from './i18n.js';
import { runScan } from './catalog.js';

// Settings > Total Commander. Which wincmd.ini a scan opens is the Go side's
// decision, so the store holds its answer instead of a path the frontend invented.

// `error` is a rejected edit: while it is set, `path` keeps the user's own text,
// because replacing it with the stored value would hide what they need to fix.
export const tcState = writable({ config: null, error: '', path: '' });

const patchTc = (patch) => tcState.update((state) => ({ ...state, ...patch }));

// The field follows the stored answer whenever nothing was rejected.
const syncPath = (config) => patchTc({ config, error: '', path: config?.effective ?? '' });

export async function loadTcConfig() {
    try {
        syncPath(await GetTcConfig());
    } catch (err) {
        // The panel has no other use for this call, so the reason is all that
        // can be shown. The typed path stays put: it is what the message is about.
        patchTc({ config: null, error: String(err?.message ?? err) });
    }
}

// Store a path and re-read the plugins, because the table the user is looking
// at describes the Total Commander that was just replaced. `path` is what the
// field holds, or the picked file the picker just returned.
export async function applyTcPath(path) {
    const { config } = get(tcState);
    const trimmed = String(path ?? '').trim();
    // Retyping the path that is already in force is not an edit, but it does
    // retire a rejected one: this puts the field back on the stored answer.
    if (config && trimmed === config.effective) {
        patchTc({ error: '', path: config.effective });
        return;
    }
    patchTc({ error: '', path });
    try {
        await SetTcConfig(trimmed);
    } catch (err) {
        // The reason plus the fact that nothing was stored: the field still
        // shows the rejected text, so the panel has to say it is not in effect.
        patchTc({ error: `${String(err?.message ?? err)} — ${t('settings.tc.not_saved')}` });
        return;
    }
    await loadTcConfig();
    runScan(true);
}

export async function browseTcIni() {
    try {
        // An empty result is the user cancelling the picker, not a failure.
        const picked = await BrowseTcIni(t('settings.tc.browse_title'));
        if (picked) await applyTcPath(picked);
    } catch (err) {
        patchTc({ error: String(err?.message ?? err) });
    }
}
