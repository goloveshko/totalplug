import './style.css';
import './mock.js'; // dev-only fake backend; compiles away in production builds
import { WindowMinimise, WindowToggleMaximise, Quit } from '../wailsjs/runtime';
import { t, setLang } from './i18n.js';
import { dom, collect } from './dom.js';
import { state } from './state.js';
import { catalogMarkup, runScan, renderTable, initCatalogEvents, bindCatalog } from './catalog.js';
import { cardMarkup, openCard, bindCard } from './card.js';
import { dialogMarkup, bindDialog } from './dialog.js';
import { progressMarkup, initProgressEvents, bindProgress } from './progress.js';
import { historyMarkup, bindHistory } from './history.js';
import { settingsMarkup, setMainChromeVisible, refreshSettingsState, bindSettings } from './settings.js';
import { bindTc } from './tc.js';
import { renderAbout, loadAppInfo, bindAbout } from './about.js';
import { handleUpdate, bindUpdate } from './update.js';

// Composition root: the window chrome, the order the screens are assembled in,
// and the two wires that genuinely cross screens — a table row opening the
// plugin card, and a language switch rebuilding every label. Each screen owns
// its own markup, state and bindings in the module named after it.

const app = document.getElementById('app');

const titlebarMarkup = () => `
      <div class="titlebar" id="titlebar">
        <div class="titlebar-left">
          <span class="titlebar-title">TotalPlug</span>
          <span class="titlebar-count" id="plugin-count" hidden></span>
        </div>
        <input type="search" id="plugin-search" placeholder="${t('search.ph')}" aria-label="${t('search.ph')}" autocomplete="off">
        <div class="titlebar-actions" id="titlebar-actions">
          <button type="button" id="btn-history" class="icon-btn" title="${t('btn.history')}" aria-label="${t('btn.history')}">
            <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
          </button>
          <button type="button" id="btn-refresh" class="icon-btn" title="${t('btn.refresh')}" aria-label="${t('btn.refresh')}">
            <svg viewBox="0 0 24 24"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
          </button>
          <button type="button" id="btn-scan" class="icon-btn primary" title="${t('btn.scan')}" aria-label="${t('btn.scan')}">
            <svg viewBox="0 0 24 24"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
          </button>
          <button type="button" id="btn-settings" class="icon-btn" title="${t('btn.settings')}" aria-label="${t('btn.settings')}">
            <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
          </button>
        </div>
        <div class="caption-buttons">
          <button type="button" id="win-min" title="${t('win.min')}" aria-label="${t('win.min')}">&#8722;</button>
          <button type="button" id="win-max" title="${t('win.max')}" aria-label="${t('win.max')}">&#9633;</button>
          <button type="button" id="win-close" class="win-close" title="${t('win.close')}" aria-label="${t('win.close')}">&#10005;</button>
        </div>
      </div>`;

function buildDom() {
    app.innerHTML = `
      ${titlebarMarkup()}
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

    bindCatalog();
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
    // settings, which must stay open).
    dom.settingsView.hidden = !state.settingsOpen;
    dom.mainView.hidden = state.settingsOpen;
    setMainChromeVisible(!state.settingsOpen);
    if (state.settingsOpen) refreshSettingsState();

    // Re-render the state-backed parts of the panel after the DOM rebuild.
    renderAbout();

    if (state.plugins.length > 0) {
        renderTable();
    }
}

function bindShell() {
    document.getElementById('win-min').addEventListener('click', WindowMinimise);
    document.getElementById('win-max').addEventListener('click', WindowToggleMaximise);
    document.getElementById('win-close').addEventListener('click', Quit);
    document.getElementById('titlebar').addEventListener('dblclick', (e) => {
        // Double-clicking the search box (select a word) or any button must not
        // maximise the window — only the empty drag area does.
        if (e.target.closest('input, button')) return;
        WindowToggleMaximise();
    });

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
        // Rebuild all labels; buildDom() below keeps the settings view open.
        buildDom();
    });
}

initCatalogEvents();
initProgressEvents();

buildDom();

// The About panel needs only local values, so ask for them once up front
// instead of when the panel opens.
loadAppInfo();

// Startup: cache-only scan, no network wait before the window is useful
runScan(false);
