import { get, writable } from 'svelte/store';

// Theme and font-scale persistence: choices live in localStorage and are
// applied to <html> before first paint to avoid a flash of the wrong theme.
const THEME_KEY = 'totalplug.theme';
const SCALE_KEY = 'totalplug.font_scale';

const darkQuery = window.matchMedia('(prefers-color-scheme: dark)');

const savedTheme = localStorage.getItem(THEME_KEY);
const knownTheme = savedTheme === 'light' || savedTheme === 'dark' || savedTheme === 'system';
export const theme = writable(knownTheme ? savedTheme : 'system');

function applyTheme(value) {
    const resolved = value === 'system' ? (darkQuery.matches ? 'dark' : 'light') : value;
    document.documentElement.dataset.theme = resolved;
}

// A subscriber rather than a setter: the value read at startup has to be applied
// and stored in its normalised form too, and that is the same work a card click
// asks for.
theme.subscribe((value) => {
    localStorage.setItem(THEME_KEY, value);
    applyTheme(value);
});

// A system switch has to re-apply the choice that is already in force, which the
// store cannot do: its value did not change.
darkQuery.addEventListener('change', () => applyTheme(get(theme)));

const savedScale = parseInt(localStorage.getItem(SCALE_KEY), 10);
export const scale = writable(savedScale >= 80 && savedScale <= 140 ? savedScale : 100);

scale.subscribe((value) => {
    localStorage.setItem(SCALE_KEY, String(value));
    document.documentElement.style.fontSize = value + '%';
});
