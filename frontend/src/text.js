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

// Placeholder row for the plugin table (scanning / empty / no search hits).
export const emptyRow = (message) => `<tr class="table-empty"><td colspan="6">${message}</td></tr>`;

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

// A timestamp this browser cannot parse has to cost one cell, not the whole view:
// Intl.DateTimeFormat throws RangeError on an Invalid Date, and in the history
// dialog that meant one bad entry took every other row down with it. The raw
// value is shown instead, which is the thing the user can report.
export const formatStamp = (fmt, value) => {
    const date = new Date(value);
    return Number.isFinite(date.getTime()) ? fmt.format(date) : String(value || '—');
};

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
