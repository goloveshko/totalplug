import { GetWindowEffect } from '../wailsjs/go/gui/App';

// The backdrop was chosen on the Go side before this page loaded: Mica on
// Windows 11 22H2 and newer, a solid window everywhere else. The page only has
// to learn which one it got, because that decides whether it may paint its own
// background. Starting opaque and going transparent a moment later costs one
// frame of solid colour instead of a flash of whatever sits behind the window.
export async function initBackdrop() {
    try {
        const effect = await GetWindowEffect();
        if (effect === 'mica') document.documentElement.dataset.backdrop = 'mica';
    } catch (err) {
        console.error(err);
    }
}
