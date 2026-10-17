import { AppInfo, CheckForAppUpdate } from '../wailsjs/go/gui/App';
import { BrowserOpenURL } from '../wailsjs/runtime';
import { get, writable } from 'svelte/store';
import { t } from './i18n.js';
import { openAppDialog, errorDialogHtml } from './dialog.js';

// The About panel: what build this is, where its data lives, and whether a newer
// release exists. The release check only runs when the user asks.

// The Go bridge owns the version and the data paths; `update` stays null until a
// check has been asked for, and `busy` only has to keep a second request away.
export const aboutState = writable({ info: null, update: null, busy: false });

const patchAbout = (patch) => aboutState.update((state) => ({ ...state, ...patch }));

const CATALOG_URL = 'https://sergey.is-a.dev/totalplug-catalog/';

export const openCatalog = () => BrowserOpenURL(CATALOG_URL);

export function openReleases() {
    const { info } = get(aboutState);
    if (info) BrowserOpenURL(info.releases_url);
}

// The panel needs only local values, so ask for them once up front instead of
// when it opens.
export function loadAppInfo() {
    return AppInfo().then((info) => patchAbout({ info }));
}

export async function checkForAppUpdate() {
    if (get(aboutState).busy) return;
    patchAbout({ busy: true });
    try {
        patchAbout({ update: await CheckForAppUpdate() });
    } catch (err) {
        // Same shape as every other failure in the app: a friendly line, with
        // the raw reason available underneath.
        openAppDialog(errorDialogHtml(err, t('about.check.error')));
    } finally {
        patchAbout({ busy: false });
    }
}

// The build stamps a four-part version because the Windows PE version resource
// demands a number; the trailing ".0" carries no meaning for a person reading
// the About panel, so it is dropped here and nowhere else.
export function formatDisplayVersion(v) {
    if (!v) return '';
    const m = v.match(/^(\d+\.\d+\.\d+)\.0$/);
    return m ? m[1] : v;
}
