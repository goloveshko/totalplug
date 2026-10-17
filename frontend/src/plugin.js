// How a plugin record is judged, shared by the table, the plugin card and both
// update paths. No DOM here: these are pure functions of a record, and what a
// screen shows of them is the component's business.

// A row offers the update button only when a download source is resolved, and
// the same rule defines the queue: "blind" updates (installed version unknown,
// hence StatusUncertain) stay a per-plugin decision, exactly like --update-all
// in the CLI.
export const isUpdatable = (p) => p.status === 'update_avail' && !!p.download_url;

// One source for how a status looks, so the table and the card cannot grow a
// second set of glyphs. Each screen labels the key through the language store.
export function badgeFor(status) {
    // Monochrome glyphs only: color emoji render differently in WebView2, do not
    // follow the theme and get read out by screen readers.
    switch (status) {
        case 'up_to_date':
            return { cls: 'badge-up', glyph: '✓', key: 'badge.up' };
        case 'update_avail':
            return { cls: 'badge-update', glyph: '↑', key: 'badge.avail' };
        case 'not_found':
            return { cls: 'badge-err', glyph: '✗', key: 'badge.missing' };
        default:
            return { cls: 'badge-muted', glyph: '?', key: 'badge.unknown' };
    }
}

// The only scheme the checker understands is "github:Owner/Repo", but nobody
// remembers that format — accept what users actually paste (a GitHub page URL,
// with or without .git/trailing slash, or a bare owner/repo) and canonicalize.
// Anything else is stored as typed.
export function normalizeSource(raw) {
    let s = raw
        .trim()
        .replace(/^https?:\/\/(?:www\.)?github\.com\//i, '')
        .replace(/^github:/i, '')
        .replace(/\.git$/i, '')
        .replace(/\/+$/, '');
    return /^[\w.-]+\/[\w.-]+$/.test(s) ? `github:${s}` : raw.trim();
}

// Inverse of normalizeSource for the same value: the stored key stays compact,
// the UI shows the page URL it came from (and re-folds it on save).
export function displaySource(source) {
    const match = /^github:([\w.-]+\/[\w.-]+)\/?$/i.exec(source || '');
    return match ? `https://github.com/${match[1]}` : source || '';
}
