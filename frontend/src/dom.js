// Every element the screens reach for, re-queried as one batch after each
// (re)render. A language switch rebuilds the whole DOM from main.js, so refs
// cached at module scope would point at detached nodes; collect() is the single
// place that has to run again, and no screen can keep a stale one by accident.
export const dom = {};

const byId = (id) => document.getElementById(id);

export function collect() {
    // Plugin view (table, toolbar, batch bar).
    dom.tbody = byId('plugins-body');
    dom.btnScan = byId('btn-scan');
    dom.btnRefresh = byId('btn-refresh');
    dom.search = byId('plugin-search');
    dom.pluginCount = byId('plugin-count');
    dom.updateAll = byId('update-all');
    dom.updateAllCount = byId('update-all-count');
    dom.btnUpdateAll = byId('btn-update-all');
    dom.mainView = byId('view-main');
    dom.titlebarActions = byId('titlebar-actions');

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

    // Confirmation / message dialog.
    dom.appDialog = byId('app-dialog');
    dom.appDialogMsg = byId('app-dialog-msg');
    dom.appDialogOk = byId('app-dialog-ok');
    dom.appDialogCancel = byId('app-dialog-cancel');

    // Update progress.
    dom.progressDialog = byId('update-progress');
    dom.progressTitle = byId('progress-title');
    dom.progressLive = byId('progress-live');
    dom.progressTrack = byId('progress-track');
    dom.progressFill = byId('progress-fill');
    dom.progressPhase = byId('progress-phase');
    dom.progressPct = byId('progress-pct');
    dom.progressUrl = byId('progress-url');
    dom.progressResult = byId('progress-result');
    dom.progressOk = byId('progress-ok');
    dom.progressClose = byId('progress-close');

    // Settings shell and the panels it hosts.
    dom.settingsView = byId('view-settings');
    dom.langSelect = byId('lang-select');
    dom.fontScale = byId('font-scale');
    dom.fontScaleValue = byId('font-scale-value');
}
