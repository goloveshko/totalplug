import { writable } from 'svelte/store';

// How wide the window has to be before the plugin card stops being a modal
// dialog over the table and becomes the rail beside it. The window opens at
// 1024px and may be pulled down to 880px, so the rail is the wide-screen shape
// of the same card, not the default one.
export const RAIL_MIN_WIDTH = 1100;

// True while the window is wide enough for the rail.
//
// The card has two shapes and only one of them can be on screen: a modal
// dialog makes the rest of the document inert, so on a wide window the table
// could not be clicked while the card was up. The window decides which shape
// that is, and both hosts read this flag.
export const wideLayout = writable(false);

export function initLayout() {
    const query = window.matchMedia(`(min-width: ${RAIL_MIN_WIDTH}px)`);
    const apply = (event) => wideLayout.set(event.matches);
    apply(query);
    query.addEventListener('change', apply);
}
