import { SaveOverride } from '../wailsjs/go/gui/App';
import { t } from './i18n.js';
import { dom } from './dom.js';
import { state } from './state.js';
import { escapeHtml, shortUrl, safeHttpUrl, interceptLinks } from './text.js';
import { getStatusBadge, sourceIcon, normalizeSource, displaySource } from './plugin.js';
import { openAppDialog, errorDialogHtml } from './dialog.js';
import { runScan } from './catalog.js';

// The plugin card: one dialog that shows everything a record says about itself
// and edits the two fields the user may override.

// The record the editor is currently editing.
let editingPlugin = null;
// The row that opened the card. A row is not focusable until it is asked to be,
// so opening the card gives it a tabindex and closing hands the keyboard back.
let cardOpener = null;

const cardRow = (label, valueHtml) => `<div class="k">${label}</div><div class="v">${valueHtml}</div>`;

export const cardMarkup = () => `
      <dialog id="plugin-card">
        <h2 id="card-name"></h2>
        <div class="card-sub" id="card-sub"></div>
        <div class="card-info" id="card-info"></div>
        <div class="card-edit-title">${t('editor.title')}</div>
        <form method="dialog" id="editor-form">
          <label>
            ${t('editor.version')}
            <input type="text" id="editor-version" placeholder="${t('editor.version.ph')}" autocomplete="off">
          </label>
          <label>
            ${t('editor.source')}
            <input type="text" id="editor-source" placeholder="${t('editor.source.ph')}" autocomplete="off">
          </label>
          <p class="editor-hint">${t('editor.hint')}</p>
          <div class="editor-actions">
            <button type="submit" class="btn-save" id="editor-save">${t('editor.save')}</button>
            <button type="button" id="editor-cancel">${t('editor.cancel')}</button>
          </div>
        </form>
      </dialog>`;

export function openCard(index, opener) {
    const plugin = state.plugins[index];
    if (!plugin) return;

    // Remembered before the dialog opens: closing must put the keyboard back on
    // the row it came from, and a row only accepts focus once it is named.
    cardOpener = opener || null;
    cardOpener?.setAttribute('tabindex', '-1');

    editingPlugin = plugin;
    dom.cardName.textContent = plugin.name;
    dom.cardSub.textContent = plugin.type + (plugin.arch ? ' · ' + plugin.arch : '');
    const latest = plugin.latest_version
        ? `<code>${escapeHtml(plugin.latest_version)}</code>${sourceIcon(plugin.source)}`
        : '—';
    const siteUrl = safeHttpUrl(plugin.web_url);
    const site = siteUrl
        ? `<a href="${escapeHtml(siteUrl)}" class="dlg-link">${escapeHtml(shortUrl(siteUrl))}</a>`
        : '';
    const tech =
        (plugin.source ? cardRow(t('card.source'), `<code>${escapeHtml(plugin.source)}</code>`) : '') +
        cardRow(t('card.path'), `<code class="path">${escapeHtml(plugin.path)}</code>`);
    dom.cardInfo.innerHTML =
        cardRow(
            t('col.installed'),
            `<code>${escapeHtml(plugin.installed_version)}</code> <small class="muted">${escapeHtml(plugin.confidence)}</small>`,
        ) +
        cardRow(t('col.latest'), latest) +
        cardRow(t('col.status'), getStatusBadge(plugin.status)) +
        (site ? cardRow(t('card.site'), site) : '') +
        `<details class="card-tech"><summary>${t('card.details')}</summary><div class="card-tech-body">${tech}</div></details>`;

    const auto = plugin.installed_version || '';
    dom.editorVersion.value = /^(unknown|n\/a)/i.test(auto) ? '' : auto;
    dom.editorSource.value = displaySource(plugin.custom_source);

    dom.cardDialog.showModal();
    // Without this the dialog picks the first focusable element it finds, which is
    // the SITE link whenever the plugin has a page - so the link opens framed and
    // Enter navigates it. Cancel is the safe place to start: the version field is
    // prefilled from auto-detection, and confirming that would pin a manual
    // override over the catalog value the user never typed.
    dom.editorCancel.focus();
}

function closeCard() {
    dom.cardDialog.close();
    returnCardFocus();
}

// The keyboard goes back where it came from. Done at the dismissal sites rather
// than only in the `close` handler: that event is what catches Esc, but a dialog
// that our own button closes should not depend on it arriving.
function returnCardFocus() {
    // A background refresh can repaint the table while the card is open, so the
    // opener row may be detached by now; focusing such an element does nothing.
    cardOpener?.focus();
    cardOpener = null;
}

async function saveOverrideFromEditor() {
    if (!editingPlugin) return;

    const override = {
        custom_version: dom.editorVersion.value.trim(),
        custom_source: normalizeSource(dom.editorSource.value),
    };

    editorSaveGuard(true);
    try {
        await SaveOverride(editingPlugin.id, override);
        closeCard();
        runScan(false);
    } catch (err) {
        openAppDialog(errorDialogHtml(err, t('editor.save_friendly')));
    } finally {
        editorSaveGuard(false);
    }
}

function editorSaveGuard(busy) {
    dom.editorSave.disabled = busy;
}

export function bindCard() {
    // Same rule as every other dialog surface: the homepage opens outside.
    interceptLinks(dom.cardInfo);
    dom.editorForm.addEventListener('submit', async (e) => {
        // method="dialog" would close before the async save finishes
        e.preventDefault();
        await saveOverrideFromEditor();
    });
    dom.editorCancel.addEventListener('click', closeCard);
    // Esc is not one of our buttons, so the close event is still what hands the
    // keyboard back for it; the paths above have already cleared the opener.
    dom.cardDialog.addEventListener('close', returnCardFocus);
}
