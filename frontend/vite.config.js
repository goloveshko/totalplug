import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// index.html carries a strict CSP for the bundle Wails serves. In dev, Vite
// injects imported CSS as inline <style> tags, so the same policy would strip
// every style from the page — remove the tag while serving.
const stripCspInDev = {
    name: 'strip-csp-in-dev',
    apply: 'serve',
    transformIndexHtml(html) {
        return html.replace(/\s*<meta[^>]*http-equiv="Content-Security-Policy"[^>]*>/i, '');
    },
};

export default defineConfig({
    // Pinned to the port documented in README.md.
    server: { port: 5199, strictPort: true },
    plugins: [svelte(), stripCspInDev],
});
