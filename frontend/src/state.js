import { writable } from 'svelte/store';

// Only what two or more screens truly share lives here. Everything else stays
// module-local in the screen that owns it, so a screen can be read — and later
// replaced by a component — without hunting for who mutates its state.
export const state = {
    // The plugin records the table currently describes, in row order. Written by
    // a scan and by the background refresh, read by the card and by both update
    // paths, which re-resolve a plugin by its stable id because either can swap
    // this array while a modal dialog is open.
    plugins: [],
    // True while an updater call is in flight. It disables the batch button,
    // makes a second update click a no-op, and keeps the progress dialog from
    // being dismissed.
    progressRunning: false,
};

// Which full-window view is on screen. A store, not a field, because the
// titlebar island reads it to hide the search box, the action icons and the
// plugin-count chip while settings is open — and that chrome no longer gets
// rebuilt when the view changes.
export const settingsOpen = writable(false);
