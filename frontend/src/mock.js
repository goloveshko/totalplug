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
        EventsOnMultiple(name, cb) { return runtime.EventsOn(name, cb); },
        EventsOnce(name, cb) {
            const once = (...args) => { runtime.EventsOff(name); cb(...args); };
            return runtime.EventsOn(name, once);
        },
        EventsOff(name) { handlers.delete(name); },
        EventsOffAll() { handlers.clear(); },
        EventsEmit: emit,
        BrowserOpenURL(url) { console.log('[mock] BrowserOpenURL', url); window.open(url, '_blank'); },
        WindowMinimise() { console.log('[mock] WindowMinimise'); },
        WindowToggleMaximise() { console.log('[mock] WindowToggleMaximise'); },
        Quit() { console.log('[mock] Quit'); },
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
            // Locked plugin keeps the lock dialog demoable.
            if (p.id.includes('Locked')) return { locked: true, backup_path: '' };
            p.installed_version = p.latest_version;
            reclassify(p);
            return { locked: false, backup_path: 'C:\\TC\\plugins\\' + p.id + '.bak_' + Date.now() };
        },

        GetUpdateHistory() {
            return Promise.resolve([
                { plugin_id: 'wlx_CudaLister', plugin_name: 'CudaLister', version: '3.1.6', applied_at: iso(-2, 9) },
                { plugin_id: 'wdx_7zip', plugin_name: '7-Zip', version: '24.08', applied_at: iso(-9, 14) },
                { plugin_id: 'wfx_DeviceLocker', plugin_name: 'DeviceLocker', version: '0.9.3', applied_at: iso(-30, 11) },
            ]);
        },

        GetWindowEffect() { return Promise.resolve('none'); },
        SetWindowEffect(effect) { console.log('[mock] SetWindowEffect', effect); return Promise.resolve(null); },
    };

    window.go = { main: { App: app } };
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
    const pa = String(a).split('.').map((n) => parseInt(n, 10) || 0);
    const pb = String(b).split('.').map((n) => parseInt(n, 10) || 0);
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
        type: 'wlx', path: `C:\\TC\\plugins\\${p.id}\\`, confidence: 'PE Binary',
        status: 'up_to_date', last_checked: checked, arch: 'x64', ...p,
    });
    return [
        base({
            id: 'ltx_CudaLister', name: 'CudaLister', type: 'ltx',
            installed_version: '3.1.6', latest_version: '3.1.6',
            source: 'github:Alexey-T/CudaLister',
            web_url: 'https://github.com/Alexey-T/CudaLister',
            description: 'Lister plugin showing CUDA source files with syntax highlighting.',
        }),
        base({
            id: 'wdx_7zip', name: '7-Zip', type: 'wdx',
            installed_version: '24.08', latest_version: '24.09', status: 'update_avail',
            source: 'totalcmd', download_url: 'https://example.invalid/7zip-24.09.zip',
            download_urls: ['https://example.invalid/7zip-24.09.zip', 'https://mirror.example.invalid/7zip-24.09.zip'],
            web_url: 'https://sevenzip.osdn.jp/',
            description: 'Content plugin: lists archives, computes hashes, reads embedded comments.',
        }),
        base({
            id: 'wlx_EdgeView', name: 'WLX EdgeViewer', type: 'wlx',
            installed_version: '1.2.5', latest_version: '1.2.6', status: 'update_avail',
            source: 'github:rg-software/wlx-edge-viewer',
            download_url: 'https://example.invalid/edgeview-1.2.6.zip',
            web_url: 'https://github.com/rg-software/wlx-edge-viewer',
            description: 'Lister plugin: opens files in a Chromium/Edge-based viewer.',
        }),
        base({
            id: 'dsx_UninstallLister', name: 'Uninstall List', type: 'dsx',
            installed_version: '1.4.0', latest_version: '1.5.2', status: 'update_avail',
            source: 'wincmd_ru', download_url: 'https://example.invalid/uninstall-1.5.2.zip',
            web_url: 'https://totalcmd.net/plugring/uninstall_list.html',
            description: 'Диск-плагин: список установленных программ с быстрым удалением.',
        }),
        base({
            id: 'wfx_DeviceLocker', name: 'DeviceLocker', type: 'wfx',
            installed_version: '0.9.3', latest_version: '1.0.0', status: 'update_avail',
            source: 'totalcmd', download_url: 'https://example.invalid/devicelocker-1.0.0.zip',
            description: 'File packer stub: every UpdatePlugin call on it returns Locked, to demo the lock dialog.',
        }),
        base({
            id: 'tbr_ToolbarPlus', name: 'ToolbarPlus Extended Collection', type: 'tbr',
            installed_version: '2.0', latest_version: '2.0',
            confidence: 'Readme/INF', description: '',
        }),
        base({
            id: 'wlx_HexPreviewVeryLongPluginName', name: 'Hex Preview (Very Long Plugin Name)', type: 'wlx',
            installed_version: '0.7.1', latest_version: '0.7.1',
            confidence: 'Readme/INF', status: 'uncertain',
            description: 'Hex viewer/editor preview inside the Lister window.',
        }),
        base({
            id: 'wfx_AllwaySync', name: 'Allway Sync', type: 'wfx',
            installed_version: '15.2', latest_version: '15.2',
            web_url: 'https://allwaysync.com/',
            description: 'Folder synchronization packer with scheduling and delta transfer.',
        }),
        base({
            id: 'wdx_SysInfo', name: 'SysInfo', type: 'wdx',
            installed_version: '2.3', latest_version: '2.3',
            confidence: 'Readme/INF', status: 'uncertain', source: 'totalcmd',
            description: 'Content plugin: system and file properties, CPU/RAM temperature widgets.',
        }),
    ];
}
