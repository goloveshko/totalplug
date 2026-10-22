// Dev-only fake Wails backend: running `npm run dev` in a plain browser
// renders the full GUI with sample plugins, a background refresh, update
// progress, overrides and history. Vite replaces import.meta.env.DEV with
// false in production builds, so this module compiles away to a no-op.
if (import.meta.env.DEV) {
    installMockBackend();
}

function installMockBackend() {
    const plugins = makePlugins();
    const handlers = new Map();
    // Update history, newest first, in the shape the Go side gives it. The 7-Zip
    // line has no kept copy left, so it offers no rollback.
    const history = [
        {
            plugin_id: 'wlx_CudaLister',
            plugin_name: 'CudaLister',
            version: '3.1.6',
            replaced_version: '3.1.5',
            plugin_dir: 'C:\\TC\\wlx\\CudaLister',
            backup_path: 'C:\\TC\\backups\\wlx\\CudaLister.bak_1700000000',
            applied_at: iso(-2, 9),
        },
        { plugin_id: 'wdx_7zip', plugin_name: '7-Zip', version: '24.08', applied_at: iso(-9, 14) },
        {
            plugin_id: 'wfx_DeviceLocker',
            plugin_name: 'DeviceLocker',
            version: '0.9.3',
            replaced_version: '0.9.2',
            plugin_dir: 'C:\\TC\\wfx\\DeviceLocker',
            backup_path: 'C:\\TC\\backups\\wfx\\DeviceLocker.bak_1700000000',
            applied_at: iso(-30, 11),
        },
    ];

    function emit(name, ...args) {
        const set = handlers.get(name);
        if (set) for (const cb of set) cb(...args);
    }

    const runtime = {
        EventsOn(name, cb) {
            if (!handlers.has(name)) handlers.set(name, new Set());
            handlers.get(name).add(cb);
            return cb;
        },
        EventsOnMultiple(name, cb) {
            return runtime.EventsOn(name, cb);
        },
        EventsOnce(name, cb) {
            const once = (...args) => {
                runtime.EventsOff(name);
                cb(...args);
            };
            return runtime.EventsOn(name, once);
        },
        EventsOff(name) {
            handlers.delete(name);
        },
        EventsOffAll() {
            handlers.clear();
        },
        EventsEmit: emit,
        BrowserOpenURL(url) {
            console.log('[mock] BrowserOpenURL', url);
            window.open(url, '_blank');
        },
        WindowMinimise() {
            console.log('[mock] WindowMinimise');
        },
        WindowToggleMaximise() {
            console.log('[mock] WindowToggleMaximise');
        },
        WindowSetDarkTheme() {
            console.log('[mock] WindowSetDarkTheme');
        },
        WindowSetLightTheme() {
            console.log('[mock] WindowSetLightTheme');
        },
        WindowSetSystemDefaultTheme() {
            console.log('[mock] WindowSetSystemDefaultTheme');
        },
        Quit() {
            console.log('[mock] Quit');
        },
    };

    const app = {
        // First call mimics the cache-only startup paint: stale statuses,
        // then a background refresh event replaces the list ~1.2s later.
        ScanPlugins(forceRefresh) {
            if (!forceRefresh && !app._painted) {
                app._painted = true;
                setTimeout(() => emit('plugins:refreshed', clone(plugins)), 1200);
                return Promise.resolve(clone(cachedPaint(plugins)));
            }
            return Promise.resolve(clone(plugins));
        },

        SaveOverride(id, override) {
            const p = plugins.find((x) => x.id === id);
            if (!p) return Promise.reject('mock: unknown plugin ' + id);
            // Mirror the Go bridge: the record keeps the override itself and the
            // effective source is the override, so the editor can prefill it.
            if (override.custom_version) p.custom_version = p.installed_version = override.custom_version;
            if (override.custom_source) p.custom_source = p.source = override.custom_source;
            reclassify(p);
            return Promise.resolve(null);
        },

        async UpdatePlugin(plugin) {
            const p = plugins.find((x) => x.id === plugin.id);
            if (!p) throw new Error('mock: unknown plugin ' + plugin.id);
            // A source that ends on a plaintext server is refused before a byte is
            // fetched, and the bridge reports it as a result rather than an error:
            // the frontend's next move is to ask the user, not to show a failure.
            if (/^http:\/\//i.test(p.download_url || '')) {
                return { locked: false, insecure: true, backup_path: '' };
            }
            return app.downloadAndInstall(p);
        },

        // The second attempt the consent flow makes once the user allows HTTP.
        UpdatePluginInsecure(plugin) {
            const p = plugins.find((x) => x.id === plugin.id);
            if (!p) return Promise.reject(new Error('mock: unknown plugin ' + plugin.id));
            return app.downloadAndInstall(p);
        },

        async downloadAndInstall(p) {
            const url = p.download_url || 'https://example.invalid/' + p.name + '.zip';
            for (let pct = 0; pct <= 100; pct += 20) {
                await sleep(150);
                emit('update:progress', { phase: 'download', percent: pct, url });
            }
            await sleep(300);
            emit('update:progress', { phase: 'extract', percent: -1, url });
            await sleep(400);
            emit('update:progress', { phase: 'install', percent: -1, url });
            await sleep(300);
            // Scripted failures keep the single-plugin and queue error paths demoable:
            // wcx_BrokenMirror always fails, wfx_DeviceLocker always reports a lock.
            // The queue walks the list in scan order, so everything after DeviceLocker
            // is a row that gets skipped by the lock ending the run.
            if (p.id === 'wcx_BrokenMirror') throw new Error('mock: every mirror returned 404 for ' + p.id);
            if (p.id === 'wfx_DeviceLocker') return { locked: true, backup_path: '' };
            p.installed_version = p.latest_version;
            reclassify(p);
            return { locked: false, insecure: false, backup_path: 'C:\\TC\\plugins\\' + p.id + '.bak_' + Date.now() };
        },

        GetUpdateHistory() {
            return Promise.resolve(clone(history));
        },

        // A rollback logs a line of its own and takes the offer away from the line
        // it undid: only the newest change to a plugin describes the folder, and
        // going forward again is what the update button is for.
        RollbackUpdate(entry) {
            if (!entry.backup_path) return Promise.reject('mock: ' + entry.plugin_id + ' has no kept copy');
            const last = history.findIndex((h) => h.plugin_id === entry.plugin_id);
            if (last >= 0) history[last].backup_path = '';
            history.unshift({
                plugin_id: entry.plugin_id,
                plugin_name: entry.plugin_name,
                version: entry.replaced_version,
                replaced_version: entry.version,
                plugin_dir: entry.plugin_dir,
                restored: true,
                applied_at: new Date().toISOString(),
            });
            return Promise.resolve({
                locked: false,
                backup_path: 'C:\\TC\\backups\\wlx\\' + entry.plugin_name + '.bak_' + Date.now(),
            });
        },

        // The real bridge answers from the Windows build; in the browser the
        // query string stands in, so `?mica` is how the backdrop CSS gets demoed.
        GetWindowEffect() {
            return Promise.resolve(location.search.includes('mica') ? 'mica' : 'none');
        },

        // Settings > Backups. The settings file holds these numbers in the real
        // app, so the mock just keeps whatever the panel last stored.
        _backup: { keep_newest: 3, max_age_days: 30 },

        GetBackupPolicy() {
            return Promise.resolve({ ...app._backup });
        },
        SetBackupPolicy(policy) {
            app._backup = {
                keep_newest: Number(policy?.keep_newest ?? 0),
                max_age_days: Number(policy?.max_age_days ?? 0),
            };
            console.log('[mock] SetBackupPolicy', app._backup);
            return Promise.resolve(null);
        },
        BackupLocation() {
            return Promise.resolve('C:\\Users\\demo\\AppData\\Roaming\\totalplug\\backups');
        },

        // Settings > Total Commander. A folder holding the word "gone" stands in
        // for a path whose file disappeared, which is the state the panel exists
        // to show, so it can be demoed by typing it.
        _tc: { configured: '', detected: 'C:\\Users\\demo\\AppData\\Roaming\\GHISLER\\wincmd.ini' },

        GetTcConfig() {
            const st = app._tc;
            const effective = st.configured || st.detected;
            return Promise.resolve({
                configured: st.configured,
                detected: st.detected,
                effective,
                readable: effective !== '' && !effective.includes('gone'),
            });
        },

        SetTcConfig(path) {
            const trimmed = String(path || '').trim();
            if (trimmed && !/wincmd\.ini$/i.test(trimmed)) {
                return Promise.reject(`wincmd.ini not accessible at ${trimmed}`);
            }
            app._tc.configured = trimmed;
            return Promise.resolve(null);
        },

        // No native picker exists in a plain browser, so Browse answers with the
        // path a user would most plausibly have chosen.
        BrowseTcIni(title) {
            console.log('[mock] BrowseTcIni', title);
            return Promise.resolve('D:\\TC-portable\\wincmd.ini');
        },

        AppInfo() {
            return Promise.resolve({
                version: '1.1.0',
                releases_url: 'https://github.com/goloveshko/totalplug/releases',
                data_dir: 'C:\\Users\\demo\\AppData\\Roaming\\totalplug',
                go_version: 'go1.25.0',
            });
        },

        // Pretend a newer release exists, so the About panel's available state
        // and its release link are demoable without publishing anything.
        async CheckForAppUpdate() {
            await sleep(900);
            return {
                current: '1.1.0',
                latest: '1.2.0',
                available: true,
                comparable: true,
                release_url: 'https://github.com/goloveshko/totalplug/releases/tag/v1.2.0',
                checked_at: new Date().toISOString(),
            };
        },
    };

    window.go = { gui: { App: app } };
    window.runtime = runtime;
    console.log('[mock] TotalPlug demo backend installed — no Go/Wails needed');
}

// Stale first paint: the two plugins whose versions come from the network
// look unresolved until the background refresh lands.
function cachedPaint(plugins) {
    return plugins.map((p) => {
        if (p.source && p.source.startsWith('github:')) {
            return { ...p, latest_version: '', status: 'up_to_date' };
        }
        if (p.status === 'update_avail' && p.source === 'wincmd_ru') {
            return { ...p, latest_version: p.installed_version, status: 'up_to_date' };
        }
        return p;
    });
}

function reclassify(p) {
    if (!p.latest_version || p.latest_version.startsWith('N/A')) return;
    p.status = compareVersions(p.installed_version, p.latest_version) < 0 ? 'update_avail' : 'up_to_date';
}

function compareVersions(a, b) {
    const pa = String(a)
        .split('.')
        .map((n) => parseInt(n, 10) || 0);
    const pb = String(b)
        .split('.')
        .map((n) => parseInt(n, 10) || 0);
    for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
        const d = (pa[i] || 0) - (pb[i] || 0);
        if (d) return d < 0 ? -1 : 1;
    }
    return 0;
}

function iso(daysAgo, hour) {
    const d = new Date();
    d.setDate(d.getDate() + daysAgo);
    d.setHours(hour, 17, 0, 0);
    return d.toISOString();
}

function sleep(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
}

function clone(x) {
    return JSON.parse(JSON.stringify(x));
}

function makePlugins() {
    const checked = iso(0, 9);
    const base = (p) => ({
        type: 'WLX',
        path: `C:\\TC\\plugins\\${p.id}\\`,
        confidence: 'PE Binary',
        status: 'up_to_date',
        last_checked: checked,
        arch: 'x64',
        ...p,
    });
    // Grouped by type the way a scan hands rows over: wincmd.ini is read section by
    // section — Packer, Lister, FileSystem, Content — and nothing re-sorts the list,
    // so equal types always arrive as one run. Type values are the four the Go side
    // emits, not the short lowercase forms the folder names use.
    return [
        base({
            id: 'wcx_BrokenMirror',
            name: 'Broken Mirror',
            type: 'WCX',
            installed_version: '3.1',
            latest_version: '3.2',
            status: 'update_avail',
            source: 'totalcmd',
            download_url: 'https://example.invalid/broken-mirror-3.2.zip',
            published_at: iso(-3, 8),
            description: 'Stub: every UpdatePlugin call on it throws, to demo a failed queue item.',
        }),
        base({
            id: 'wlx_CudaLister',
            name: 'CudaLister',
            installed_version: '3.1.6',
            latest_version: '3.1.6',
            source: 'github:Alexey-T/CudaLister',
            published_at: iso(-200, 14),
            web_url: 'https://github.com/Alexey-T/CudaLister',
            description: 'Lister plugin showing CUDA source files with syntax highlighting.',
        }),
        base({
            id: 'wlx_EdgeView',
            name: 'WLX EdgeViewer',
            installed_version: '1.2.5',
            latest_version: '1.2.6',
            status: 'update_avail',
            source: 'github:rg-software/wlx-edge-viewer',
            download_url: 'https://example.invalid/edgeview-1.2.6.zip',
            published_at: iso(-21, 12),
            web_url: 'https://github.com/rg-software/wlx-edge-viewer',
            description: 'Lister plugin: opens files in a Chromium/Edge-based viewer.',
        }),
        base({
            id: 'wlx_HexPreviewVeryLongPluginName',
            name: 'Hex Preview (Very Long Plugin Name)',
            installed_version: '0.7.1',
            latest_version: '0.7.1',
            confidence: 'Readme/INF',
            status: 'uncertain',
            description: 'Hex viewer/editor preview inside the Lister window.',
        }),
        base({
            id: 'wlx_PreviewPlus',
            name: 'PreviewPlus',
            installed_version: '1.0',
            latest_version: '1.1',
            status: 'update_avail',
            source: 'totalcmd',
            download_url: 'https://example.invalid/previewplus-1.1.zip',
            published_at: iso(-40, 15),
            description: 'Stub whose update is offered, to demo a batch row that succeeds.',
        }),
        base({
            id: 'wfx_DeviceLocker',
            name: 'DeviceLocker',
            type: 'WFX',
            installed_version: '0.9.3',
            latest_version: '1.0.0',
            status: 'update_avail',
            source: 'totalcmd',
            download_url: 'https://example.invalid/devicelocker-1.0.0.zip',
            published_at: iso(-64, 9),
            description: 'Stub: every UpdatePlugin call on it returns Locked, to demo the lock dialog.',
        }),
        base({
            id: 'wfx_AllwaySync',
            name: 'Allway Sync',
            type: 'WFX',
            installed_version: '15.2',
            latest_version: '15.2',
            web_url: 'https://allwaysync.com/',
            description: 'Folder synchronization packer with scheduling and delta transfer.',
        }),
        base({
            id: 'wdx_7zip',
            name: '7-Zip',
            type: 'WDX',
            installed_version: '24.08',
            latest_version: '24.09',
            status: 'update_avail',
            source: 'totalcmd',
            download_url: 'https://example.invalid/7zip-24.09.zip',
            download_urls: ['https://example.invalid/7zip-24.09.zip', 'https://mirror.example.invalid/7zip-24.09.zip'],
            published_at: iso(-95, 18),
            web_url: 'https://sevenzip.osdn.jp/',
            description: 'Content plugin: lists archives, computes hashes, reads embedded comments.',
        }),
        // The plain-HTTP case: the strict call refuses it and the consent flow asks.
        base({
            id: 'wdx_UninstallLister',
            name: 'Uninstall List',
            type: 'WDX',
            installed_version: '1.4.0',
            latest_version: '1.5.2',
            status: 'update_avail',
            source: 'wincmd_ru',
            download_url: 'http://example.invalid/uninstall-1.5.2.zip',
            published_at: iso(-400, 11),
            web_url: 'https://totalcmd.net/plugring/uninstall_list.html',
            description: 'Content plugin: список установленных программ с быстрым удалением.',
        }),
        base({
            id: 'wdx_SysInfo',
            name: 'SysInfo',
            type: 'WDX',
            installed_version: '2.3',
            latest_version: '2.3',
            confidence: 'Readme/INF',
            status: 'uncertain',
            source: 'totalcmd',
            description: 'Content plugin: system and file properties, CPU/RAM temperature widgets.',
        }),
        base({
            id: 'wdx_ToolbarPlus',
            name: 'ToolbarPlus Extended Collection',
            type: 'WDX',
            installed_version: 'Unknown',
            latest_version: '2.0',
            confidence: 'Unknown',
            status: 'uncertain',
            description: '',
        }),
    ];
}
