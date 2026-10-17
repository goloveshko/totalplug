import { writable } from 'svelte/store';
import { settingsOpen } from './state.js';

// Settings is a full-window view, not a dialog: it replaces the plugin list and
// borrows the titlebar. Which panel is on screen is a store, so showing another
// one neither rebuilds the shell nor touches the panel being left behind — the
// text typed into a field that has not saved yet stays there across a visit.

export const settingsSection = writable('appearance');

export const openSettings = () => settingsOpen.set(true);

export const closeSettings = () => settingsOpen.set(false);
