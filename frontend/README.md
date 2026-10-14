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
- `src/*.js` — markup and behavior, one module per screen: `catalog.js` (the table
  and its search), `card.js`, `update.js`, `progress.js` and `dialog.js` (the two
  result dialogs), `history.js`, `about.js`, `tc.js`, `settings.js`, over
  `state.js`, `dom.js`, `text.js` and `plugin.js` for what they share. `main.js`
  is only the composition root: the titlebar, the order the screens go in, and the
  two wires that cross them — a row opening its card, a language switch rebuilding
  every label;
- `src/mock.js` — the demo data (plugins, history), for anything you want to
  look at or add to.

The mock exists only for `npm run dev`: in the built application
(`import.meta.env.DEV` is off) it is cut out of the bundle and every call goes
to the real backend.

The native parts are invisible in a plain browser — the frameless window's drag
region and caption buttons, the window shadow, a link handed to the system
browser, and the WebView2 profile the persisted settings live in. Those can only
be checked in a built `totalplug.exe`.

## Formatting

`npm run format` reformats the repository, `npm run format:check` only reports —
that is the check CI runs. Both step out to the repo root first: the config that
answers for the whole tree is `/.prettierrc`, and so are `/.prettierignore` and
`/.gitignore`, which keep the generated `dist/` and `wailsjs/` out of it.

The config is 4 spaces and single quotes, matching how `src/` is written by
hand, with an override back to 2 spaces and double quotes for JSON and YAML:
that is how npm writes `package.json`, how Wails writes `wails.json` and how
every CI example is pasted, so those files stay formatted by themselves instead
of being rewritten on the next tool run.
