'use strict';

const { app, BrowserWindow, Menu, Notification, clipboard, dialog, ipcMain, net, powerSaveBlocker, protocol, shell } = require('electron');
const { spawn } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const readline = require('node:readline');
const { autoUpdater } = require('electron-updater');

// The UI is served over a custom scheme rather than straight from http://127.0.0.1:<port>
// because localStorage — where every saved setting and gear set lives — is keyed by origin
// *including the port*. The sim server binds an OS-assigned port, so pointing the window at
// the loopback URL directly would hand the user an empty settings store on every launch.
// Proxying a fixed scheme to whatever port we got keeps the origin stable forever.
const SCHEME = 'wowsims';
const ORIGIN = `${SCHEME}://app`;
const START_URL = `${ORIGIN}/tbc/`;

// Must match readyLinePrefix in sim/web/main.go.
const READY_PREFIX = 'WOWSIMS_LISTENING port=';
const SIDECAR_START_TIMEOUT_MS = 30_000;

const RELEASES_URL = 'https://github.com/zmsl/tbc-new/releases/latest';
const REPO_URL = 'https://github.com/zmsl/tbc-new';
const DISCORD_URL = 'https://discord.gg/jJMPr9JWwx';

// Windows ties notifications to an AppUserModelID. Without this matching the installed
// app's identity they are silently dropped -- no error, they simply never appear.
const APP_ID = 'com.wowsims.tbc.desktop';

// macOS auto-update goes through Squirrel.Mac, which refuses to apply an update to an
// unsigned app bundle — there is no way around this. Until the bundle is signed and
// notarized, mac builds notify and link to the release page instead of updating in place.
// Flip this to true once an Apple Developer certificate is wired into the build.
const MAC_AUTO_UPDATE_SUPPORTED = false;

let simProcess = null;
let simPort = 0;
let mainWindow = null;
let powerBlockerId = null;

protocol.registerSchemesAsPrivileged([
	{
		scheme: SCHEME,
		privileges: { standard: true, secure: true, supportFetchAPI: true, corsEnabled: true, stream: true },
	},
]);

function sidecarPath() {
	const exe = process.platform === 'win32' ? 'wowsimtbc.exe' : 'wowsimtbc';
	// Packaged: electron-builder drops the binary in extraResources. Dev: the repo root
	// build produced by `make wowsimtbc`.
	return app.isPackaged ? path.join(process.resourcesPath, exe) : path.join(__dirname, '..', exe);
}

// Starts the Go sim server and resolves with the port it bound. We wait for the server to
// announce itself on stdout rather than polling a port, so this can never race startup.
function startSidecar() {
	const bin = sidecarPath();
	if (!fs.existsSync(bin)) {
		return Promise.reject(new Error(`Sim server not found at ${bin}\n\nRun "make wowsimtbc" from the repo root first.`));
	}

	simProcess = spawn(bin, ['--desktop'], { stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true });

	return new Promise((resolve, reject) => {
		let settled = false;
		const timer = setTimeout(() => {
			if (!settled) {
				settled = true;
				reject(new Error('The sim server did not start within 30 seconds.'));
			}
		}, SIDECAR_START_TIMEOUT_MS);

		readline.createInterface({ input: simProcess.stdout }).on('line', line => {
			if (!settled && line.startsWith(READY_PREFIX)) {
				settled = true;
				clearTimeout(timer);
				resolve(Number(line.slice(READY_PREFIX.length)));
				return;
			}
			console.log('[sim]', line);
		});
		readline.createInterface({ input: simProcess.stderr }).on('line', line => console.log('[sim]', line));

		simProcess.on('error', err => {
			if (!settled) {
				settled = true;
				clearTimeout(timer);
				reject(err);
			}
		});

		simProcess.on('exit', code => {
			simProcess = null;
			if (!settled) {
				settled = true;
				clearTimeout(timer);
				reject(new Error(`The sim server exited with code ${code} before it finished starting.`));
			} else if (!app.isQuitting) {
				// The engine died out from under a running window; there is nothing left to
				// show, so fail loudly rather than leaving a dead UI on screen.
				dialog.showErrorBox('Simulator stopped', `The sim server exited unexpectedly (code ${code}). The app will close.`);
				app.quit();
			}
		});
	});
}

function stopSidecar() {
	if (!simProcess) return;
	const child = simProcess;
	simProcess = null;
	// Closing stdin is the graceful path: the server watches it and exits on EOF. kill() is
	// the backstop for a server that is wedged and not reading.
	try {
		child.stdin.end();
	} catch {
		/* already gone */
	}
	child.kill();
}

// Forwards a custom-scheme request to the loopback sim server. Everything the page loads —
// HTML, assets, and the protobuf API calls the web workers make — comes through here.
function installProtocolHandler() {
	protocol.handle(SCHEME, request => {
		const url = new URL(request.url);
		const headers = new Headers(request.headers);
		// Host would be "app", and Origin would be the custom scheme; neither means anything
		// to the Go server, and leaving them set confuses its routing.
		headers.delete('host');
		headers.delete('origin');

		const init = { method: request.method, headers, redirect: 'follow' };
		if (request.method !== 'GET' && request.method !== 'HEAD') {
			init.body = request.body;
			init.duplex = 'half';
		}
		return net.fetch(`http://127.0.0.1:${simPort}${url.pathname}${url.search}`, init);
	});
}

const windowStateFile = () => path.join(app.getPath('userData'), 'window-state.json');

function readWindowState() {
	try {
		const s = JSON.parse(fs.readFileSync(windowStateFile(), 'utf8'));
		if (Number.isFinite(s.width) && Number.isFinite(s.height)) return s;
	} catch {
		/* first run, or the file was clobbered; fall through to defaults */
	}
	return { width: 1600, height: 1000 };
}

function saveWindowState(win) {
	if (!win || win.isDestroyed()) return;
	try {
		const b = win.getNormalBounds();
		fs.writeFileSync(windowStateFile(), JSON.stringify({ ...b, maximized: win.isMaximized(), zoomLevel: win.webContents.getZoomLevel() }));
	} catch (err) {
		console.error('[window] could not save state', err);
	}
}

function createWindow() {
	const state = readWindowState();
	mainWindow = new BrowserWindow({
		x: state.x,
		y: state.y,
		width: state.width,
		height: state.height,
		minWidth: 1024,
		minHeight: 700,
		backgroundColor: '#1d2021',
		show: false,
		autoHideMenuBar: true,
		webPreferences: {
			preload: path.join(__dirname, 'preload.js'),
			contextIsolation: true,
			nodeIntegration: false,
			sandbox: true,
			// Chromium throttles timers hard in hidden windows. The sim itself is unaffected
			// -- it runs in the Go process -- but ui/worker/worker_http.ts polls
			// /asyncProgress on a 500ms setTimeout, so a throttled window would sit there
			// looking stalled and only notice the run finished once you switched back.
			// Alt-tabbing away during a long sim is the normal thing to do here.
			backgroundThrottling: false,
		},
	});

	if (state.maximized) mainWindow.maximize();
	mainWindow.once('ready-to-show', () => {
		if (Number.isFinite(state.zoomLevel)) mainWindow.webContents.setZoomLevel(state.zoomLevel);
		mainWindow.show();
	});
	mainWindow.webContents.on('zoom-changed', () => saveWindowState(mainWindow));
	mainWindow.on('close', () => saveWindowState(mainWindow));
	mainWindow.on('closed', () => {
		mainWindow = null;
	});

	// Anything that is not our own scheme is a real external link (Wowhead, GitHub, Discord)
	// and belongs in the user's actual browser, not in this window.
	mainWindow.webContents.setWindowOpenHandler(({ url }) => {
		if (url.startsWith('http://') || url.startsWith('https://')) shell.openExternal(url);
		return { action: 'deny' };
	});
	mainWindow.webContents.on('will-navigate', (event, url) => {
		if (!url.startsWith(ORIGIN)) {
			event.preventDefault();
			// Only hand real web links to the OS browser. Internal schemes (devtools:,
			// chrome:) would otherwise be shoved at it too, which does nothing useful.
			if (url.startsWith('http://') || url.startsWith('https://')) shell.openExternal(url);
		}
	});

	// A dead renderer leaves a blank window that looks like a hang. The sidecar is a
	// separate process and survives, so reloading actually recovers.
	mainWindow.webContents.on('render-process-gone', (_event, details) => {
		if (details.reason === 'clean-exit') return;
		const response = dialog.showMessageBoxSync(mainWindow, {
			type: 'error',
			title: 'Display crashed',
			message: 'The sim window stopped responding.',
			detail: `Reason: ${details.reason}. Your saved settings are safe.`,
			buttons: ['Reload', 'Quit'],
			defaultId: 0,
		});
		if (response === 0) mainWindow.reload();
		else app.quit();
	});

	buildAppMenu(mainWindow);
	installContextMenu(mainWindow);

	mainWindow.loadURL(START_URL);
	return mainWindow;
}

// Without this Electron installs its own default menu, which carries Electron's branding
// and links to electronjs.org. Menu labels are English only: the menu lives in the main
// process, which has no access to the renderer's i18next instance.
function buildAppMenu(win) {
	const isMac = process.platform === 'darwin';
	const send = channel => () => win && !win.isDestroyed() && win.webContents.send(channel);

	const template = [
		...(isMac ? [{ role: 'appMenu' }] : []),
		{
			label: '&File',
			submenu: [
				{ label: 'Import Settings...', accelerator: 'CmdOrCtrl+O', click: send('wowsims:menu-import') },
				{ label: 'Export Settings...', accelerator: 'CmdOrCtrl+S', click: send('wowsims:menu-export') },
				{ type: 'separator' },
				{ role: isMac ? 'close' : 'quit' },
			],
		},
		{ role: 'editMenu' },
		{
			label: '&Sim',
			submenu: [
				// Ctrl+R is the muscle memory for "run" in a simulator, so it wins here and
				// the built-in reload role moves down to Ctrl+Shift+R.
				{ label: 'Run Simulation', accelerator: 'CmdOrCtrl+R', click: send('wowsims:menu-run-sim') },
			],
		},
		{
			label: '&View',
			submenu: [
				{ role: 'reload', accelerator: 'CmdOrCtrl+Shift+R' },
				{ type: 'separator' },
				{ role: 'resetZoom' },
				{ role: 'zoomIn' },
				// The role only binds Ctrl+= / Ctrl+Shift+=; people press Ctrl+- and also
				// expect the numpad keys to work.
				{ role: 'zoomIn', accelerator: 'CmdOrCtrl+numadd', visible: false },
				{ role: 'zoomOut' },
				{ role: 'zoomOut', accelerator: 'CmdOrCtrl+numsub', visible: false },
				{ type: 'separator' },
				{ role: 'togglefullscreen' },
				{ role: 'toggleDevTools' },
			],
		},
		{ role: 'windowMenu' },
		{
			label: '&Help',
			submenu: [
				{ label: 'Discord', click: () => shell.openExternal(DISCORD_URL) },
				{ label: 'Source Code', click: () => shell.openExternal(REPO_URL) },
				{ label: 'Releases', click: () => shell.openExternal(RELEASES_URL) },
				{ type: 'separator' },
				{
					label: 'About',
					click: () =>
						dialog.showMessageBox(win, {
							type: 'info',
							title: 'WoWSims TBC',
							message: 'WoWSims TBC',
							detail: `Version ${app.getVersion()}\nElectron ${process.versions.electron}\nChromium ${process.versions.chrome}`,
							buttons: ['OK'],
						}),
				},
			],
		},
	];

	Menu.setApplicationMenu(Menu.buildFromTemplate(template));
}

// Electron ships no context menu at all, which leaves text inputs with no copy or paste --
// the clearest sign to a user that they are not looking at a real application.
function installContextMenu(win) {
	win.webContents.on('context-menu', (_event, params) => {
		const items = [];
		const canEdit = params.isEditable;

		if (params.misspelledWord && params.dictionarySuggestions.length) {
			for (const suggestion of params.dictionarySuggestions.slice(0, 5)) {
				items.push({ label: suggestion, click: () => win.webContents.replaceMisspelling(suggestion) });
			}
			items.push({ type: 'separator' });
		}
		if (canEdit) items.push({ role: 'undo', enabled: params.editFlags.canUndo }, { role: 'redo', enabled: params.editFlags.canRedo }, { type: 'separator' });
		if (canEdit) items.push({ role: 'cut', enabled: params.editFlags.canCut });
		items.push({ role: 'copy', enabled: params.editFlags.canCopy });
		if (canEdit) items.push({ role: 'paste', enabled: params.editFlags.canPaste });
		items.push({ role: 'selectAll', enabled: params.editFlags.canSelectAll });

		if (params.linkURL && params.linkURL.startsWith('http')) {
			items.push({ type: 'separator' }, { label: 'Copy Link Address', click: () => clipboard.writeText(params.linkURL) });
		}
		if (!app.isPackaged) {
			items.push({ type: 'separator' }, { label: 'Inspect Element', click: () => win.webContents.inspectElement(params.x, params.y) });
		}

		Menu.buildFromTemplate(items).popup({ window: win });
	});
}

function setupAutoUpdate(win) {
	// An unpackaged run has no update channel to talk to, and electron-updater throws if
	// asked anyway.
	if (!app.isPackaged) return;

	const canSelfUpdate = process.platform !== 'darwin' || MAC_AUTO_UPDATE_SUPPORTED;
	autoUpdater.autoDownload = false;
	autoUpdater.autoInstallOnAppQuit = false;

	const send = (channel, payload) => {
		if (win && !win.isDestroyed()) win.webContents.send(channel, payload);
	};

	autoUpdater.on('update-available', info => send('wowsims:update-available', { version: info.version, canSelfUpdate, releaseUrl: RELEASES_URL }));
	autoUpdater.on('download-progress', p => send('wowsims:update-progress', Math.round(p.percent)));
	autoUpdater.on('update-downloaded', () => send('wowsims:update-ready'));
	autoUpdater.on('error', err => {
		console.error('[updater]', err);
		send('wowsims:update-error', String(err));
	});

	autoUpdater.checkForUpdates().catch(err => console.error('[updater] check failed', err));
}

// A long sim is exactly the kind of work you start and then alt-tab away from, so it
// should behave like any other long-running desktop task: show progress on the taskbar
// button, keep the machine awake, and say something when it is done.
ipcMain.on('wowsims:sim-progress', (_event, fraction) => {
	if (!mainWindow || mainWindow.isDestroyed()) return;
	// setProgressBar clamps, but guard NaN from a zero total before iterations are known.
	mainWindow.setProgressBar(Number.isFinite(fraction) ? Math.max(0, Math.min(1, fraction)) : 2);

	if (powerBlockerId === null) {
		powerBlockerId = powerSaveBlocker.start('prevent-app-suspension');
	}
});

const releasePowerBlocker = () => {
	if (powerBlockerId !== null && powerSaveBlocker.isStarted(powerBlockerId)) {
		powerSaveBlocker.stop(powerBlockerId);
	}
	powerBlockerId = null;
};

// Called on completion AND on abort/error, so neither the progress bar nor the sleep
// blocker can be left behind by a sim that did not finish normally.
ipcMain.on('wowsims:sim-done', (_event, summary) => {
	if (mainWindow && !mainWindow.isDestroyed()) mainWindow.setProgressBar(-1);
	releasePowerBlocker();

	if (!summary || !summary.notify) return;
	// Only worth interrupting for if they are looking at something else.
	if (mainWindow && !mainWindow.isDestroyed() && mainWindow.isFocused()) return;
	if (!Notification.isSupported()) return;

	const notification = new Notification({ title: 'Simulation complete', body: summary.body || '' });
	notification.on('click', () => {
		if (mainWindow && !mainWindow.isDestroyed()) {
			if (mainWindow.isMinimized()) mainWindow.restore();
			mainWindow.focus();
		}
	});
	notification.show();
});

// downloadString in the web UI builds a data: URL and clicks an <a download>, which in an
// installed app drops the file into Downloads with no dialog and no way to choose a name.
ipcMain.handle('wowsims:save-file', async (_event, { fileName, contents }) => {
	if (!mainWindow || mainWindow.isDestroyed()) return { saved: false };
	const ext = path.extname(fileName || '').replace('.', '') || 'json';
	const { canceled, filePath } = await dialog.showSaveDialog(mainWindow, {
		defaultPath: fileName || `wowsims.${ext}`,
		filters: [{ name: ext.toUpperCase(), extensions: [ext] }, { name: 'All Files', extensions: ['*'] }],
	});
	if (canceled || !filePath) return { saved: false };
	await fs.promises.writeFile(filePath, contents, 'utf8');
	return { saved: true, filePath };
});

ipcMain.handle('wowsims:start-update', async () => {
	if (process.platform === 'darwin' && !MAC_AUTO_UPDATE_SUPPORTED) {
		await shell.openExternal(RELEASES_URL);
		return { openedExternally: true };
	}
	await autoUpdater.downloadUpdate();
	return { downloading: true };
});

// chrome://gpu cannot be opened in this window, so expose the same underlying data to
// DevTools instead. Whether the GPU is actually compositing is the first thing to check
// when the UI feels less smooth than the same page in a browser.
ipcMain.handle('wowsims:gpu-status', () => ({
	featureStatus: app.getGPUFeatureStatus(),
	// Electron's default is hardware accelerated; this is here to prove nothing turned it off.
	hardwareAccelerationDisabled: app.commandLine.hasSwitch('disable-gpu'),
}));

ipcMain.handle('wowsims:install-update', () => {
	app.isQuitting = true;
	stopSidecar();
	autoUpdater.quitAndInstall();
});

// Two windows would share the wowsims://app origin and therefore the same localStorage,
// so a second instance could silently overwrite the first one's saved settings. Focus the
// window that already exists instead.
if (!app.requestSingleInstanceLock()) {
	app.quit();
} else {
	app.on('second-instance', () => {
		if (mainWindow) {
			if (mainWindow.isMinimized()) mainWindow.restore();
			mainWindow.focus();
		}
	});

	app.whenReady().then(async () => {
		// Must be set before any notification is created, or Windows drops them silently.
		if (process.platform === 'win32') app.setAppUserModelId(APP_ID);
		try {
			simPort = await startSidecar();
		} catch (err) {
			dialog.showErrorBox('Could not start the simulator', err.message || String(err));
			app.quit();
			return;
		}
		installProtocolHandler();
		setupAutoUpdate(createWindow());
	});
}

// Closing the window shuts the whole thing down, including the sim server. This is
// deliberate on macOS too, where the platform convention would be to keep the app running:
// leaving a headless sim server alive with no visible window is exactly the behaviour the
// desktop build exists to avoid.
app.on('window-all-closed', () => app.quit());

app.on('before-quit', () => {
	app.isQuitting = true;
	releasePowerBlocker();
	stopSidecar();
});

// Backstops for paths that bypass before-quit.
process.on('exit', stopSidecar);
