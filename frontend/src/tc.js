import { GetTcConfig, SetTcConfig, BrowseTcIni } from '../wailsjs/go/gui/App';
import { t } from './i18n.js';
import { escapeHtml } from './text.js';
import { runScan } from './catalog.js';

// Settings > Total Commander. Which wincmd.ini a scan opens is the Go side's
// decision, so this panel shows its answer instead of a path the frontend
// invented.

// `tcError` is a rejected edit: while it is set the field keeps the user's own
// text, because replacing it with the stored value would hide what they need to
// fix.
let tcConfig = null;
let tcError = '';

export const tcMarkup = () => `
          <div class="settings-panel" id="panel-tc">
            <h3>${t('settings.nav.tc')}</h3>
            <p class="section-desc">${t('settings.tc')}</p>
            <div class="section-label">${t('settings.tc.ini')}</div>
            <div class="tc-row">
              <input type="text" id="tc-ini" aria-label="${t('settings.tc.ini')}" spellcheck="false" autocomplete="off">
              <button type="button" class="btn-secondary" id="tc-browse">${t('settings.tc.browse')}</button>
            </div>
            <div class="tc-state" id="tc-state"></div>
            <p class="editor-hint">${t('settings.tc.hint')}</p>
          </div>`;

export function renderTcPanel() {
    const pathEl = document.getElementById('tc-ini');
    const host = document.getElementById('tc-state');
    if (!pathEl || !host) return;

    if (!tcError && tcConfig) pathEl.value = tcConfig.effective;

    let line = '';
    if (tcError) {
        line = `<span class="tc-warn">${escapeHtml(tcError)}</span>`;
    } else if (tcConfig) {
        if (!tcConfig.effective) {
            line = `<span class="tc-warn">${escapeHtml(t('settings.tc.none'))}</span>`;
        } else if (!tcConfig.readable) {
            line = `<span class="tc-warn">${escapeHtml(t('settings.tc.gone'))}</span>`;
        } else {
            const key = tcConfig.configured ? 'settings.tc.mode.manual' : 'settings.tc.mode.auto';
            line = `<span class="tc-mode">${escapeHtml(t(key))}</span>`;
        }
    }
    // The way out of a manual path is offered wherever one is stored, healthy or
    // not: it is the only control that returns the panel to detection.
    const back = tcConfig?.configured
        ? `<button type="button" class="btn-secondary" id="tc-auto">${escapeHtml(t('settings.tc.back_auto'))}</button>`
        : '';
    host.innerHTML = line + back;
}

export async function loadTcConfig() {
    try {
        tcConfig = await GetTcConfig();
        tcError = '';
    } catch (err) {
        // The panel has no other use for this call, so the reason is all that
        // can be shown.
        tcError = String(err?.message ?? err);
        tcConfig = null;
    }
    renderTcPanel();
}

// Store a path and re-read the plugins, because the table the user is looking
// at describes the Total Commander that was just replaced.
async function applyTcPath(path) {
    const trimmed = String(path ?? '').trim();
    tcError = '';
    // Retyping the path that is already in force is not an edit, but it does
    // retire a rejected one: the field goes back to what is actually stored.
    if (tcConfig && trimmed === tcConfig.effective) {
        renderTcPanel();
        return;
    }
    try {
        await SetTcConfig(trimmed);
    } catch (err) {
        // The reason plus the fact that nothing was stored: the field still
        // shows the rejected text, so the panel has to say it is not in effect.
        tcError = `${String(err?.message ?? err)} — ${t('settings.tc.not_saved')}`;
        renderTcPanel();
        return;
    }
    await loadTcConfig();
    runScan(true);
}

async function browseTcIni() {
    try {
        // An empty result is the user cancelling the picker, not a failure.
        const picked = await BrowseTcIni(t('settings.tc.browse_title'));
        if (!picked) return;
        const pathEl = document.getElementById('tc-ini');
        if (pathEl) pathEl.value = picked;
        await applyTcPath(picked);
    } catch (err) {
        tcError = String(err?.message ?? err);
        renderTcPanel();
    }
}

export function bindTc() {
    // The path saves on the same trigger the rest of the settings use — leaving
    // the field — and Enter counts as leaving it, because typing an absolute
    // Windows path and pressing Enter is how everyone expects that to work.
    const pathEl = document.getElementById('tc-ini');
    pathEl.addEventListener('change', () => applyTcPath(pathEl.value));
    pathEl.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            applyTcPath(pathEl.value);
        }
    });
    document.getElementById('tc-browse').addEventListener('click', browseTcIni);
    // The state line is rebuilt as HTML, so its button is reached the way the
    // About panel reaches its own row.
    document.getElementById('tc-state').addEventListener('click', (e) => {
        if (e.target.closest('#tc-auto')) applyTcPath('');
    });
}
