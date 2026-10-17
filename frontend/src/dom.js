// Every element the screens reach for, re-queried as one batch after each
// (re)render. A language switch rebuilds the screens from main.js, so refs
// cached at module scope would point at detached nodes; collect() is the single
// place that has to run again, and no screen can keep a stale one by accident.
// The titlebar, the plugin table and the overlays are not here: they are Svelte
// islands that never rebuild.
export const dom = {};

const byId = (id) => document.getElementById(id);

export function collect() {
    // Settings shell and the panels it hosts.
    dom.settingsView = byId('view-settings');
    dom.langSelect = byId('lang-select');
    dom.fontScale = byId('font-scale');
    dom.fontScaleValue = byId('font-scale-value');
}
