import { t } from './i18n.js';
import { escapeHtml } from './text.js';

// How a plugin record is presented and judged, shared by the table, the plugin
// card and both update paths. No DOM here: these are pure functions of a record.

// A row offers the update button only when a download source is resolved, and
// the same rule defines the queue: "blind" updates (installed version unknown,
// hence StatusUncertain) stay a per-plugin decision, exactly like --update-all
// in the CLI.
export const isUpdatable = (p) => p.status === 'update_avail' && !!p.download_url;

export function getStatusBadge(status) {
    // Monochrome glyphs only: color emoji render differently in WebView2, do not
    // follow the theme and get read out by screen readers.
    switch (status) {
        case 'up_to_date':
            return `<span class="badge badge-up">✓ ${t('badge.up')}</span>`;
        case 'update_avail':
            return `<span class="badge badge-update">↑ ${t('badge.avail')}</span>`;
        case 'not_found':
            return `<span class="badge badge-err">✗ ${t('badge.missing')}</span>`;
        default:
            return `<span class="badge badge-muted">? ${t('badge.unknown')}</span>`;
    }
}

// Which provider resolved the latest version, shown as a glyph beside it. It
// replaces a whole "Update source" row: for a GitHub plugin that row duplicated
// the homepage, and the raw keys ("totalcmd", "github:Owner/Repo") are only
// interesting to a user debugging something, so they moved to the details block.
// Each glyph paints itself: a CSS `fill` would override the markup attribute.
export function sourceIcon(source) {
    if (!source) return '';
    const github = /^github:/i.test(source);
    const label = github ? 'GitHub' : t('card.source.catalog');
    const glyph = github
        ? // octicons/mark-github-16 (MIT, (c) GitHub Inc.)
          '<svg viewBox="0 0 16 16"><path fill="currentColor" d="M6.766 11.328c-2.063-.25-3.516-1.734-3.516-3.656 0-.781.281-1.625.75-2.188-.203-.515-.172-1.609.063-2.062.625-.078 1.468.25 1.968.703.594-.187 1.219-.281 1.985-.281.765 0 1.39.094 1.953.265.484-.437 1.344-.765 1.969-.687.218.422.25 1.515.046 2.047.5.593.766 1.39.766 2.203 0 1.922-1.453 3.375-3.547 3.64.531.344.89 1.094.89 1.954v1.625c0 .468.391.734.86.547C13.781 14.359 16 11.53 16 8.03 16 3.61 12.406 0 7.984 0 3.563 0 0 3.61 0 8.031a7.88 7.88 0 0 0 5.172 7.422c.422.156.828-.125.828-.547v-1.25c-.219.094-.5.156-.75.156-1.031 0-1.64-.562-2.078-1.609-.172-.422-.36-.672-.719-.719-.187-.015-.25-.093-.25-.187 0-.188.313-.328.625-.328.453 0 .844.281 1.25.86.313.452.64.655 1.031.655s.641-.14 1-.5c.266-.265.47-.5.657-.656"/></svg>'
        : // Stand-in until the catalog ships its own mark.
          '<svg viewBox="0 0 24 24"><g fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="7" height="7" rx="1.5"/><rect x="13" y="4" width="7" height="7" rx="1.5"/><rect x="4" y="13" width="7" height="7" rx="1.5"/><rect x="13" y="13" width="7" height="7" rx="1.5"/></g></svg>';
    return `<span class="src-icon" role="img" title="${escapeHtml(label)}" aria-label="${escapeHtml(label)}">${glyph}</span>`;
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
