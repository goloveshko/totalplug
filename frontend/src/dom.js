// Every element the screens reach for, re-queried as one batch after each
// (re)render. A language switch rebuilds the screens from main.js, so refs
// cached at module scope would point at detached nodes; collect() is the single
// place that has to run again, and no screen can keep a stale one by accident.
// The titlebar, the plugin table and the two dialogs are not here: they are
// Svelte islands that never rebuild.
export const dom = {};

const byId = (id) => document.getElementById(id);

export function collect() {
    // Plugin card and its version editor.
    dom.cardDialog = byId('plugin-card');
    dom.cardName = byId('card-name');
    dom.cardSub = byId('card-sub');
    dom.cardInfo = byId('card-info');
    dom.editorForm = byId('editor-form');
    dom.editorVersion = byId('editor-version');
    dom.editorSource = byId('editor-source');
    dom.editorCancel = byId('editor-cancel');
    dom.editorSave = byId('editor-save');

    // History.
    dom.historyDialog = byId('history-dialog');
    dom.historyBody = byId('history-body');
    dom.historyClose = byId('history-close');

    // Settings shell and the panels it hosts.
    dom.settingsView = byId('view-settings');
    dom.langSelect = byId('lang-select');
    dom.fontScale = byId('font-scale');
    dom.fontScaleValue = byId('font-scale-value');
}
