import { AppInfo, CheckForAppUpdate } from '../wailsjs/go/gui/App';
import { BrowserOpenURL } from '../wailsjs/runtime';
import { t, currentLang } from './i18n.js';
import { escapeHtml, safeHttpUrl, formatStamp, interceptLinks } from './text.js';
import { openAppDialog, errorDialogHtml } from './dialog.js';

// The About panel: what build this is, where its data lives, and whether a newer
// release exists. The release check only runs when the user asks.

// The Go bridge owns the version and the data paths; the release check only runs
// when the user presses the button, so `appUpdate` stays null until then. Both
// survive the language-switch DOM rebuild.
let appInfo = null;
let appUpdate = null;
let appUpdateBusy = false;

// The build stamps a four-part version because the Windows PE version resource
// demands a number; the trailing ".0" carries no meaning for a person reading
// the About panel, so it is dropped here and nowhere else.
function formatDisplayVersion(v) {
    if (!v) return '';
    const m = v.match(/^(\d+\.\d+\.\d+)\.0$/);
    return m ? m[1] : v;
}

export const aboutMarkup = () => `
          <div class="settings-panel" id="panel-about">
            <div class="about-name">TotalPlug <span class="about-version" id="about-version"></span></div>
            <p class="about-desc">${t('about.desc')}</p>
            <div class="about-update" id="about-update"></div>
            <div class="about-links">
              <button type="button" class="btn-secondary" id="link-catalog">${t('about.catalog')}</button>
              <button type="button" class="btn-secondary" id="link-releases">${t('about.releases')}</button>
            </div>
            <p class="editor-hint about-privacy">${t('about.privacy')}</p>
            <details class="about-details">
              <summary>${t('about.details')}</summary>
              <div class="about-tech">
                <div class="k">${t('about.data_dir')}</div>
                <div class="v"><code class="path" id="about-data-dir"></code></div>
                <div class="k">${t('about.build')}</div>
                <div class="v" id="about-build"></div>
              </div>
            </details>
          </div>`;

export function renderAbout() {
    const nameEl = document.getElementById('about-version');
    if (!nameEl) return;
    nameEl.textContent = formatDisplayVersion(appInfo?.version);
    const dataDirEl = document.getElementById('about-data-dir');
    dataDirEl.textContent = appInfo?.data_dir ?? '';
    document.getElementById('about-build').textContent = appInfo?.go_version ?? '';
    renderAboutUpdate();
}

// One status line plus the button that produced it. The available case carries
// the release link too, because that is the whole point of the check.
function renderAboutUpdate() {
    const host = document.getElementById('about-update');
    if (!host) return;

    let status = '';
    if (appUpdateBusy) {
        status = `<span class="about-status">${t('about.checking')}</span>`;
    } else if (appUpdate) {
        const fmt = new Intl.DateTimeFormat(currentLang() === 'ru' ? 'ru-RU' : 'en-GB', { timeStyle: 'short' });
        const when = `<span class="about-checked">${escapeHtml(t('about.checked_at', { time: formatStamp(fmt, appUpdate.checked_at) }))}</span>`;
        if (!appUpdate.comparable) {
            status = `<span class="about-status">${escapeHtml(t('about.dev_build'))}</span>${when}`;
        } else if (appUpdate.available) {
            // GitHub answers with the URL, so it is remote data like the catalog's
            // homepage field. Without a link the row still names the new version,
            // which is the part the user came to read.
            const releaseUrl = safeHttpUrl(appUpdate.release_url);
            const link = releaseUrl
                ? `<a href="${escapeHtml(releaseUrl)}" class="dlg-link">${escapeHtml(t('about.open_release'))}</a>`
                : '';
            status = `<span class="about-status accent">${escapeHtml(t('about.available', { latest: appUpdate.latest }))}</span> ${link}${when}`;
        } else {
            status = `<span class="about-status">${escapeHtml(t('about.up_to_date'))}</span>${when}`;
        }
    }

    // The label stays put while a check runs: the status line already says
    // "checking", and a disabled button only has to stop a second request.
    const button = `<button type="button" class="btn-secondary" id="btn-app-update" ${appUpdateBusy ? 'disabled' : ''}>${escapeHtml(t('about.check'))}</button>`;
    host.innerHTML = `<div class="about-update-row">${button}</div>${status}`;
}

export async function checkForAppUpdate() {
    if (appUpdateBusy) return;
    appUpdateBusy = true;
    renderAboutUpdate();
    try {
        appUpdate = await CheckForAppUpdate();
    } catch (err) {
        // Same shape as every other failure in the app: a friendly line, with
        // the raw reason available underneath.
        openAppDialog(errorDialogHtml(err, t('about.check.error')));
    } finally {
        appUpdateBusy = false;
        renderAboutUpdate();
    }
}

// The panel needs only local values, so ask for them once up front instead of
// when it opens.
export function loadAppInfo() {
    return AppInfo().then((info) => {
        appInfo = info;
        renderAbout();
    });
}

export function bindAbout() {
    document.getElementById('link-catalog').addEventListener('click', () => {
        BrowserOpenURL('https://sergey.is-a.dev/totalplug-catalog/');
    });
    document.getElementById('link-releases').addEventListener('click', () => {
        if (appInfo) BrowserOpenURL(appInfo.releases_url);
    });
    // The update row is re-rendered as HTML, so its button and release link are
    // reached through one delegated listener instead of per-render bindings.
    const aboutUpdateEl = document.getElementById('about-update');
    interceptLinks(aboutUpdateEl);
    aboutUpdateEl.addEventListener('click', (e) => {
        if (e.target.closest('#btn-app-update')) checkForAppUpdate();
    });
}
