# TotalPlug GUI — running it without Go or Wails

Design work needs nothing but Node.js:

```
cd frontend
npm install
npm run dev
```

Open http://localhost:5199 — the interface runs on a demo mock (`src/mock.js`):
the plugin list, the plugin card, the update dialog with its progress, history,
settings and the counters. Hot reload: an edit shows up in the browser at once.

What to edit:

- `src/style.css` — 95% of the design: the palette and the spacing live in CSS
  custom properties (`:root` plus the dark theme), and so do the table, the
  plugin card, the dialogs and the window titlebar;
- `src/i18n.js` — every label (EN + RU);
- `src/main.js` — markup and behavior;
- `src/mock.js` — the demo data (plugins, history), for anything you want to
  look at or add to.

The mock exists only for `npm run dev`: in the built application
(`import.meta.env.DEV` is off) it is cut out of the bundle and every call goes
to the real backend.

The native parts are invisible in a plain browser — the frameless window's drag
region and caption buttons, the window shadow, a link handed to the system
browser, and the WebView2 profile the persisted settings live in. Those can only
be checked in a built `totalplug.exe`.
