import { BrowserOpenURL } from '../wailsjs/runtime';

// String and URL helpers for the screens that build HTML by hand. Every value
// interpolated into markup comes from outside the app — wincmd.ini, the catalog,
// GitHub release metadata — so the escaping rules live in one place next to the
// one clickable-message rule every dialog has to obey.

// Every cell of the table is built from wincmd.ini / catalog data: a name with
// "<" or "&" breaks the markup unless it is escaped.
export const escapeHtml = (s) =>
    String(s)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');

// Turn http(s) URLs in already-escaped text into clickable links. Plain anchors
// would navigate the webview, so clicks are intercepted and routed to the
// system browser (see interceptLinks).
export const linkify = (escaped) => escaped.replace(/(https?:\/\/[^\s<]+)/g, '<a href="$1" class="dlg-link">$1</a>');

// escapeHtml keeps a quote from leaving the attribute and says nothing about the
// scheme, while these addresses arrive from the catalog and from GitHub release
// metadata: a `javascript:` value in a plugin's homepage field would run inside
// this window, where the Go bindings are reachable. Anything that is not http(s)
// is not a link here.
export const safeHttpUrl = (url) => (/^https?:\/\//i.test(url || '') ? url : '');

// "https://" is noise in a narrow dialog and costs a line wrap; the href keeps it.
export const shortUrl = (url) => url.replace(/^https?:\/\//i, '').replace(/\/+$/, '');

// The first address inside a raw Go error is the one worth naming: it says where
// the failing request went. Go quotes it — Get "https://host/x": EOF — so the
// closing quote and any trailing punctuation are cut off, they are not the address.
export const firstHttpUrl = (text) => {
    const match = String(text || '').match(/https?:\/\/[^\s]+/);
    return match ? match[0].replace(/[.,;:!?)\]»"']+$/, '') : '';
};

// A timestamp this browser cannot parse has to cost one cell, not the whole view:
// Intl.DateTimeFormat throws RangeError on an Invalid Date, and in the history
// dialog that meant one bad entry took every other row down with it. The raw
// value is shown instead, which is the thing the user can report.
export const formatStamp = (fmt, value) => {
    const date = new Date(value);
    return Number.isFinite(date.getTime()) ? fmt.format(date) : String(value || '—');
};

// A date the backend could not resolve crosses the bridge as the zero time
// ("0001-01-01T00:00:00Z"), which parses as a date and would be drawn as one.
// No plugin release and no plugin file is that old, so a date before the year
// 2000 is read as "no date" — one place, so every date on screen agrees.
const YEAR_ZERO = Date.UTC(2000, 0, 1);

export const hasDate = (value) => {
    const time = new Date(value).getTime();
    return Number.isFinite(time) && time >= YEAR_ZERO;
};

// A release date is useful as "3 days ago" at a glance and as the exact date on
// hover. Days are the granularity that matters for a version: anything older
// than a month reads better in months, and past a year in years. `Intl` also
// localizes the plural form («2 дня назад»), which a hand-built string would not.
export function releaseDate(value, lang) {
    const date = new Date(value);
    if (!hasDate(value)) return null;

    const days = Math.round((date.getTime() - Date.now()) / 86400000);
    const months = days / 30;
    let amount = days;
    let unit = 'day';
    if (Math.abs(months) >= 12) {
        amount = Math.round(days / 365);
        unit = 'year';
    } else if (Math.abs(months) >= 1) {
        amount = Math.round(months);
        unit = 'month';
    }

    const locale = lang === 'ru' ? 'ru-RU' : 'en-US';
    return {
        text: new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(amount, unit),
        title: new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(date),
    };
}

// The day a file on disk was written, in the user's language. The warning about
// an unreadable version names the exact day on purpose: "3 months ago" says
// nothing about which build is newer.
export function fileDate(value, lang) {
    if (!hasDate(value)) return '';
    const locale = lang === 'ru' ? 'ru-RU' : 'en-US';
    const date = new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(new Date(value));
    // The Russian medium format ends in «г.», and the sentence brings its own
    // dot: «20 авг. 2026 г..» would be a typo the reader sees.
    return date.replace(/\.$/, '');
}

// A dialog link must not navigate the webview. A click hands the address to the
// system browser, and a middle click has to be caught too: it fires `auxclick`,
// which the browser otherwise acts on by itself, replacing the GUI with a
// plugin's homepage.
export function interceptLinks(el) {
    const onLink = (e) => {
        const a = e.target.closest('a.dlg-link');
        if (!a) return;
        e.preventDefault();
        BrowserOpenURL(a.href);
    };
    el.addEventListener('click', onLink);
    el.addEventListener('auxclick', onLink);
}
