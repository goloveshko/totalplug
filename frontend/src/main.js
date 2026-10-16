import './style.css';
import './mock.js'; // dev-only fake backend; compiles away in production builds
import { mount } from 'svelte';
import { get } from 'svelte/store';
import { setLang } from './i18n.js';
import Titlebar from './Titlebar.svelte';
import { dom, collect } from './dom.js';
import { state, settingsOpen } from './state.js';
import { catalogMarkup, runScan, renderTable, initCatalogEvents } from './catalog.js';
import { cardMarkup, openCard, bindCard } from './card.js';
import { dialogMarkup, bindDialog } from './dialog.js';
import { progressMarkup, initProgressEvents, bindProgress } from './progress.js';
import { historyMarkup, bindHistory } from './history.js';
import { settingsMarkup, refreshSettingsState, bindSettings } from './settings.js';
import { bindTc } from './tc.js';
import { renderAbout, loadAppInfo, bindAbout } from './about.js';
import { bindUpdate, handleUpdate } from './update.js';

// Composition root: the window chrome is a Svelte island that stays mounted, the
// screens below it are rebuilt from markup functions, and the wires that actually
// cross screens are a table row opening the plugin card and a language switch
// re-rendering the screens. Each screen owns its markup, state and bindings in
// the module named after it.

const chrome = document.getElementById('chrome');
const screens = document.getElementById('screens');

function buildScreens() {
    screens.innerHTML = `
      ${catalogMarkup()}
      ${settingsMarkup()}
      ${cardMarkup()}
      ${historyMarkup()}
      ${dialogMarkup()}
      ${progressMarkup()}
    `;

    // Every screen reaches its elements through dom.js, which has to be
    // re-filled before the first bind or the first re-render below.
    collect();

    bindDialog();
    bindProgress();
    bindCard();
    bindUpdate();
    bindHistory();
    bindAbout();
    bindTc();
    bindSettings();
    bindShell();

    // Restore the visible view after a rebuild (language switch happens inside
    // settings, which must stay open). The titlebar hides its own search box and
    // action icons by watching the same store.
    const open = get(settingsOpen);
    dom.settingsView.hidden = !open;
    dom.mainView.hidden = open;
    if (open) refreshSettingsState();

    // Re-render the state-backed parts of the panel after the DOM rebuild.
    renderAbout();

    if (state.plugins.length > 0) {
        renderTable();
    }
}

function bindShell() {
    // One delegated handler for the table: the update button keeps its own
    // action, a click anywhere else in the row opens the plugin card.
    dom.tbody.addEventListener('click', (e) => {
        const updateBtn = e.target.closest('button.btn-update');
        if (updateBtn) {
            handleUpdate(Number(updateBtn.dataset.update));
            return;
        }
        if (e.target.closest('button')) return;
        const tr = e.target.closest('tr[data-idx]');
        if (tr) openCard(Number(tr.dataset.idx), tr);
    });

    dom.langSelect.addEventListener('change', () => {
        setLang(dom.langSelect.value);
        // Rebuild the labels; the island re-labels itself and buildScreens()
        // below keeps the settings view open.
        buildScreens();
    });
}

initCatalogEvents();
initProgressEvents();

// The chrome before the screens: mounting the island is what puts the titlebar
// on screen, and the screens have to exist before anything can call renderTable.
mount(Titlebar, { target: chrome });
buildScreens();

// The About panel needs only local values, so ask for them once up front
// instead of when the panel opens.
loadAppInfo();

// Startup: cache-only scan, no network wait before the window is useful
runScan(false);
