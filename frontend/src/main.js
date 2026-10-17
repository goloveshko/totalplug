import './style.css';
import './mock.js'; // dev-only fake backend; compiles away in production builds
import { mount } from 'svelte';
import { get } from 'svelte/store';
import { setLang } from './i18n.js';
import Titlebar from './Titlebar.svelte';
import PluginTable from './PluginTable.svelte';
import Dialog from './Dialog.svelte';
import ProgressOverlay from './ProgressOverlay.svelte';
import PluginCard from './PluginCard.svelte';
import { dom, collect } from './dom.js';
import { settingsOpen } from './state.js';
import { runScan, initCatalogEvents } from './catalog.js';
import { initProgressEvents } from './progress.js';
import { historyMarkup, bindHistory } from './history.js';
import { settingsMarkup, refreshSettingsState, bindSettings } from './settings.js';
import { bindTc } from './tc.js';
import { bindBackups } from './backups.js';
import { renderAbout, loadAppInfo, bindAbout } from './about.js';

// Composition root: the titlebar, the plugin table and the three overlays are
// Svelte islands that stay mounted, the screens below them are still rebuilt from
// markup functions, and the wires that cross screens are a store per shared value
// plus a language switch that re-renders the vanilla ones. Each screen owns its
// markup, state and bindings in the module named after it.

const chrome = document.getElementById('chrome');
const tableRoot = document.getElementById('table');
const overlays = document.getElementById('overlays');
const screens = document.getElementById('screens');

function buildScreens() {
    screens.innerHTML = `
      ${settingsMarkup()}
      ${historyMarkup()}
    `;

    // Every screen reaches its elements through dom.js, which has to be
    // re-filled before the first bind or the first re-render below.
    collect();

    bindHistory();
    bindAbout();
    bindTc();
    bindBackups();
    bindSettings();
    bindShell();

    // Restore the visible view after a rebuild (language switch happens inside
    // settings, which must stay open). The table and the titlebar hide parts of
    // themselves from the same store; this view still ships `hidden` in its
    // markup, so the rebuild has to lift it back.
    const open = get(settingsOpen);
    dom.settingsView.hidden = !open;
    if (open) refreshSettingsState();

    // Re-render the state-backed parts of the panel after the DOM rebuild.
    renderAbout();
}

function bindShell() {
    dom.langSelect.addEventListener('change', () => {
        setLang(dom.langSelect.value);
        // Rebuild the labels; the islands re-label themselves and buildScreens()
        // below keeps the settings view open.
        buildScreens();
    });
}

initCatalogEvents();
initProgressEvents();

// The islands before the screens: mounting them is what puts the titlebar, the
// table and the three overlays on screen, and the table reads the plugin list a
// scan writes next. A dialog that never rebuilds can also stay open across a
// language switch, which the vanilla screens could not do.
mount(Titlebar, { target: chrome });
mount(PluginTable, { target: tableRoot });
mount(Dialog, { target: overlays });
mount(ProgressOverlay, { target: overlays });
mount(PluginCard, { target: overlays });
buildScreens();

// The About panel needs only local values, so ask for them once up front
// instead of when the panel opens.
loadAppInfo();

// Startup: cache-only scan, no network wait before the window is useful
runScan(false);
