// How a plugin record is judged, shared by the table, the plugin card and both
// update paths. No DOM here: these are pure functions of a record, and what a
// screen shows of them is the component's business.

// A row offers the update button only when a download source is resolved, and
// the same rule defines the queue: "blind" updates (installed version unknown,
// hence StatusUncertain) stay a per-plugin decision, exactly like --update-all
// in the CLI.
export const isUpdatable = (p) => p.status === 'update_avail' && !!p.download_url;

// A plugin whose installed version could not be compared with the catalog's:
// the version was never read, or it was read out of prose and is not trusted.
// The catalog version is offered as a separate action — it is not an update,
// and it never joins the batch, the way the unverified list stays out of
// --update-all on the command line.
export const isBlindUpdatable = (p) => p.status === 'uncertain' && !!p.download_url;

// A record whose installed version was never read: the scanner writes
// "Unknown"/"N/A" or a low-confidence guess, and the checker refuses to offer an
// automatic update for it. The card uses this to explain why there is no update
// button and to open the version field right away.
export const isUnknownInstalled = (p) =>
    !!p && (p.confidence === 'Unknown' || !p.installed_version || /^(unknown|n\/a)/i.test(p.installed_version));

// Where the installed version came from, as a dictionary key. The backend
// answers with a fixed English token — the logic compares it, so it stays
// English — and the card shows it as a word in the user's language.
const confidenceKeys = {
    'PE Binary': 'confidence.exact',
    'Pluginst.inf': 'confidence.inf',
    'Readme/INF': 'confidence.text',
    'Manual DB': 'confidence.manual',
    Unknown: 'confidence.unknown',
};

export const confidenceKey = (confidence) => confidenceKeys[confidence] || 'confidence.unknown';

// What the search box matches a row by: the name, the type it is grouped under
// and the folder it lives in — typing "wdx" or a folder name is a way to find
// a plugin, not a second feature. Shared by the table and the count chip so
// the two cannot disagree about what a row matches.
export function matchesQuery(plugin, query) {
    const q = query.trim().toLowerCase();
    if (!q) return true;
    return (
        plugin.name.toLowerCase().includes(q) ||
        plugin.type.toLowerCase().includes(q) ||
        (plugin.path || '').toLowerCase().includes(q)
    );
}

// The numeric core a version string compares by: a port of the Go checker's
// parseVersionParts, so the order the table sorts by cannot disagree with the
// status the backend computed. Digits accumulate into components, .-_ separate
// them, a parenthesized tail is dropped and the first any other character ends
// the core — so "2.0 x64" sorts as 2.0 and never as 2.64.
function versionCore(v) {
    const parts = [];
    let current = null;
    const s = String(v ?? '')
        .split('(')[0]
        .trim()
        .replace(/^[vV]/, '');
    for (const ch of s) {
        if (ch >= '0' && ch <= '9') {
            current = (current ?? 0) * 10 + Number(ch);
        } else if (ch === '.' || ch === '-' || ch === '_') {
            if (current !== null) {
                parts.push(current);
                current = null;
            }
        } else {
            break;
        }
    }
    if (current !== null) parts.push(current);
    return parts;
}

// Version-aware ordering for the sortable columns. Equal cores ("1.0" vs
// "1.0.0", a beta tag) fall back to the raw strings, so the sort stays
// deterministic; strings with no numeric core at all sort by string.
export function compareVersions(a, b) {
    const pa = versionCore(a);
    const pb = versionCore(b);
    const len = Math.max(pa.length, pb.length);
    for (let i = 0; i < len; i++) {
        const d = (pa[i] ?? 0) - (pb[i] ?? 0);
        if (d !== 0) return d;
    }
    return String(a ?? '').localeCompare(String(b ?? ''), undefined, { numeric: true });
}

// What the search box offers under it: the types whose name the query matches
// (typing "w" offers WLX, WFX, WDX — the filter nobody can discover from
// outside, because typing "wlx" already works through matchesQuery), then the
// plugins whose name matches. An empty query lists every installed type: the
// box doubles as the type filter's front door, so clicking into it shows what
// it can filter by before a single letter is typed. Picking a type fills the
// box with it, which the table already answers as a filter; picking a plugin
// opens its card. Pure data: the titlebar draws it.
const TYPE_ORDER = ['WCX', 'WLX', 'WFX', 'WDX'];
const NAME_SUGGESTION_CAP = 6;

export function buildSuggestions(list, query) {
    const q = query.trim().toLowerCase();

    const types = [];
    for (const type of TYPE_ORDER) {
        if (q && !type.toLowerCase().includes(q)) continue;
        // An uninstalled type is not a filter this machine can use.
        const count = list.filter((p) => p.type === type).length;
        if (count) types.push({ type, count });
    }

    if (!q) return { types, names: [] };

    const names = [];
    for (let index = 0; index < list.length && names.length < NAME_SUGGESTION_CAP; index++) {
        if (list[index].name.toLowerCase().includes(q)) names.push({ index, plugin: list[index] });
    }
    return { types, names };
}

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
