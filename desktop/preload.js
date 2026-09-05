'use strict';

const { contextBridge, ipcRenderer } = require('electron');

// The renderer runs the ordinary wowsims web UI, which also ships to the website. It must
// keep working with none of this present, so everything here is additive and the UI feature
// detects `window.wowsimsDesktop` before using it.
const on = (channel, callback) => {
	const listener = (_event, payload) => callback(payload);
	ipcRenderer.on(channel, listener);
	return () => ipcRenderer.removeListener(channel, listener);
};

contextBridge.exposeInMainWorld('wowsimsDesktop', {
	isDesktop: true,
	platform: process.platform,

	// Fires when a newer release exists. `canSelfUpdate` is false on unsigned macOS builds,
	// where Squirrel.Mac cannot apply an update in place — there the UI should offer a
	// download link rather than an "update and restart" button.
	onUpdateAvailable: callback => on('wowsims:update-available', callback),
	onUpdateProgress: callback => on('wowsims:update-progress', callback),
	onUpdateReady: callback => on('wowsims:update-ready', callback),
	onUpdateError: callback => on('wowsims:update-error', callback),

	// Chromium's GPU feature status, the same data chrome://gpu shows. Check this from
	// DevTools when the UI feels sluggish: "gpu_compositing" and "rasterization" should
	// say "enabled", not "disabled_software".
	getGpuStatus: () => ipcRenderer.invoke('wowsims:gpu-status'),

	// Begins the download. Resolves `{ openedExternally: true }` when the platform cannot
	// self-update and the release page was opened in the browser instead.
	startUpdate: () => ipcRenderer.invoke('wowsims:start-update'),

	// Quits and installs. Only meaningful after onUpdateReady has fired.
	installUpdate: () => ipcRenderer.invoke('wowsims:install-update'),
});
