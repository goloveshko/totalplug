import { writable } from 'svelte/store';

// Only what two or more screens truly share lives here. Everything else stays
// module-local in the screen that owns it, so a screen can be read — and later
// replaced by a component — without hunting for who mutates its state.

// The plugin records the table describes, in row order. Written by a scan and by
// the background refresh, read by the card and by both update paths, which
// re-resolve a plugin by its stable id because either can swap this list while a
// modal dialog is open.
export const plugins = writable([]);

// True while an updater call is in flight. It disables every update button, so
// a second click cannot start a parallel call.
export const progressRunning = writable(false);

// Which full-window view is on screen. The titlebar and the plugin table hide
// parts of themselves from this store, so neither has to be rebuilt to do it.
export const settingsOpen = writable(false);
