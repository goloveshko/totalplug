import './style.css';
import './mock.js'; // dev-only fake backend; compiles away in production builds
import { mount } from 'svelte';
import Titlebar from './Titlebar.svelte';
import PluginTable from './PluginTable.svelte';
import Dialog from './Dialog.svelte';
import PluginCard from './PluginCard.svelte';
import HistoryDialog from './HistoryDialog.svelte';
import Settings from './Settings.svelte';
import { loadAppInfo } from './about.js';
import { initBackdrop } from './backdrop.js';
import { initCatalogEvents, runScan } from './catalog.js';
import { initLayout } from './layout.js';
import { initUpdateEvents } from './update.js';

// Composition root: every screen is a Svelte island that stays mounted and reads
// what it shows from a store, so nothing is rebuilt — a language switch re-labels
// the whole window in place, dialogs included. What is left here is the order the
// islands go on screen and the two calls that start the app.

const chrome = document.getElementById('chrome');
const tableRoot = document.getElementById('table');
const overlays = document.getElementById('overlays');
const settingsRoot = document.getElementById('settings');

initCatalogEvents();
initUpdateEvents();
initBackdrop();
initLayout();

mount(Titlebar, { target: chrome });
mount(PluginTable, { target: tableRoot });
mount(Dialog, { target: overlays });
mount(PluginCard, { target: overlays });
mount(HistoryDialog, { target: overlays });
mount(Settings, { target: settingsRoot });

// The About panel needs only local values, so ask for them once up front
// instead of when the panel opens.
loadAppInfo();

// Startup: cache-only scan, no network wait before the window is useful
runScan(false);
