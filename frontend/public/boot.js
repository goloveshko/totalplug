// Pre-paint theme/language resolution. Served as a classic (parser-blocking)
// script from public/ so it runs before the module graph and before first paint;
// the localStorage keys are also read by src/theme.js and src/i18n.js.
try {
    var theme = localStorage.getItem('totalplug.theme');
    var dark = matchMedia('(prefers-color-scheme: dark)').matches;
    if (theme === 'dark' || ((theme === null || theme === 'system') && dark))
        document.documentElement.dataset.theme = 'dark';
    var lang = localStorage.getItem('totalplug.lang');
    if (lang === 'ru' || lang === 'en') document.documentElement.lang = lang;
} catch (e) {}
