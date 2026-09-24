// Theme and font-scale persistence: choices live in localStorage and are
// applied to <html> before first paint to avoid a flash of the wrong theme.
const THEME_KEY = 'totalplug.theme';
const SCALE_KEY = 'totalplug.font_scale';

const darkQuery = window.matchMedia('(prefers-color-scheme: dark)');

let theme = localStorage.getItem(THEME_KEY);
if (theme !== 'light' && theme !== 'dark' && theme !== 'system') {
    theme = 'system';
}

function applyTheme() {
    const resolved = theme === 'system' ? (darkQuery.matches ? 'dark' : 'light') : theme;
    document.documentElement.dataset.theme = resolved;
}

darkQuery.addEventListener('change', () => {
    if (theme === 'system') applyTheme();
});

export function currentTheme() {
    return theme;
}

export function setTheme(next) {
    theme = next;
    localStorage.setItem(THEME_KEY, next);
    applyTheme();
}

let scale = parseInt(localStorage.getItem(SCALE_KEY), 10);
if (!(scale >= 80 && scale <= 140)) {
    scale = 100;
}

function applyScale() {
    document.documentElement.style.fontSize = scale + '%';
}

export function currentScale() {
    return scale;
}

export function setScale(next) {
    scale = Math.min(140, Math.max(80, next));
    localStorage.setItem(SCALE_KEY, String(scale));
    applyScale();
}

applyTheme();
applyScale();
