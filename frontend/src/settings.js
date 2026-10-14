import { t, currentLang } from './i18n.js';
import { dom } from './dom.js';
import { state } from './state.js';
import { currentTheme, setTheme, currentScale, setScale } from './theme.js';
import { renderPluginCount } from './catalog.js';
import { tcMarkup, renderTcPanel, loadTcConfig } from './tc.js';
import { aboutMarkup } from './about.js';

// Settings is a full-window view, not a dialog: it replaces the plugin list and
// borrows the titlebar. Its own state survives the DOM rebuild that a language
// switch triggers, so the user stays where they were.

let settingsSection = 'appearance';

export const settingsMarkup = () => `
      <div id="view-settings" hidden>
        <div class="settings-top">
          <button type="button" id="settings-back" class="btn-back">${t('settings.back')}</button>
          <h2>${t('settings.title')}</h2>
          <span class="settings-save-hint">${t('settings.save_hint')}</span>
        </div>
        <div class="settings-body">
          <div class="settings-nav">
            <button type="button" data-panel="appearance">${t('settings.nav.appearance')}</button>
            <button type="button" data-panel="tc">${t('settings.nav.tc')}</button>
            <button type="button" data-panel="language">${t('settings.nav.language')}</button>
            <button type="button" data-panel="about">${t('settings.nav.about')}</button>
          </div>
          <div class="settings-panel" id="panel-appearance">
            <h3>${t('settings.nav.appearance')}</h3>
            <p class="section-desc">${t('settings.appearance')}</p>
            <div class="section-label">${t('settings.theme')}</div>
            <div class="theme-cards">
              <button type="button" class="theme-card" data-theme-choice="light" aria-pressed="false">
                <span class="theme-preview preview-light"></span>
                <span class="name">${t('settings.theme.light')}</span>
                <span class="desc">${t('settings.theme.light.desc')}</span>
              </button>
              <button type="button" class="theme-card" data-theme-choice="dark" aria-pressed="false">
                <span class="theme-preview preview-dark"></span>
                <span class="name">${t('settings.theme.dark')}</span>
                <span class="desc">${t('settings.theme.dark.desc')}</span>
              </button>
              <button type="button" class="theme-card" data-theme-choice="system" aria-pressed="false">
                <span class="theme-preview preview-system"></span>
                <span class="name">${t('settings.theme.system')}</span>
                <span class="desc">${t('settings.theme.system.desc')}</span>
              </button>
            </div>
            <div class="section-label">${t('settings.font')}</div>
            <div class="font-row">
              <input type="range" id="font-scale" min="80" max="140" step="5">
              <span class="font-value" id="font-scale-value">100%</span>
            </div>
          </div>
          ${tcMarkup()}
          <div class="settings-panel" id="panel-language">
            <h3>${t('settings.nav.language')}</h3>
            <p class="section-desc">${t('settings.language')}</p>
            <select id="lang-select">
              <option value="en">English</option>
              <option value="ru">Русский</option>
            </select>
          </div>
          ${aboutMarkup()}
        </div>
      </div>`;

export function refreshSettingsState() {
    dom.settingsView.querySelectorAll('[data-theme-choice]').forEach((el) => {
        const selected = el.dataset.themeChoice === currentTheme();
        el.classList.toggle('selected', selected);
        el.setAttribute('aria-pressed', String(selected));
    });
    dom.langSelect.value = currentLang();
    dom.fontScale.value = String(currentScale());
    dom.fontScaleValue.textContent = currentScale() + '%';
    dom.settingsView.querySelectorAll('.settings-nav button').forEach((btn) => {
        btn.classList.toggle('active', btn.dataset.panel === settingsSection);
    });
    dom.settingsView.querySelectorAll('.settings-panel').forEach((panel) => {
        panel.classList.toggle('active', panel.id === 'panel-' + settingsSection);
    });
    // The DOM rebuilds on a language switch, which empties the path field; this
    // is what puts the stored answer back into it.
    renderTcPanel();
}

// The titlebar search box and action icons belong to the plugin view only;
// they hide while the full-window settings view is open.
export function setMainChromeVisible(visible) {
    dom.search.hidden = !visible;
    dom.titlebarActions.hidden = !visible;
    renderPluginCount();
}

export function openSettings() {
    state.settingsOpen = true;
    dom.mainView.hidden = true;
    dom.settingsView.hidden = false;
    setMainChromeVisible(false);
    refreshSettingsState();
    if (settingsSection === 'tc') loadTcConfig();
}

export function closeSettings() {
    state.settingsOpen = false;
    dom.settingsView.hidden = true;
    dom.mainView.hidden = false;
    setMainChromeVisible(true);
}

export function bindSettings() {
    document.getElementById('btn-settings').addEventListener('click', openSettings);
    document.getElementById('settings-back').addEventListener('click', closeSettings);
    dom.settingsView.querySelectorAll('.settings-nav button').forEach((btn) => {
        btn.addEventListener('click', () => {
            settingsSection = btn.dataset.panel;
            // Detection can move while the app runs (a portable TC mounted, an
            // install uninstalled), and asking is two stat calls, so the panel
            // is refreshed on entry rather than trusted from the last visit.
            if (settingsSection === 'tc') loadTcConfig();
            refreshSettingsState();
        });
    });
    // Real <button>s, so Enter/Space reach the handler through the native click.
    dom.settingsView.querySelectorAll('[data-theme-choice]').forEach((card) => {
        card.addEventListener('click', () => {
            setTheme(card.dataset.themeChoice);
            refreshSettingsState();
        });
    });
    dom.fontScale.addEventListener('input', () => {
        setScale(Number(dom.fontScale.value));
        dom.fontScaleValue.textContent = currentScale() + '%';
    });
}
