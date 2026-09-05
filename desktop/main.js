'use strict';

const { app, BrowserWindow, dialog, ipcMain, net, protocol, shell } = require('electron');
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

// macOS auto-update goes through Squirrel.Mac, which refuses to apply an update to an
// unsigned app bundle — there is no way around this. Until the bundle is signed and
// notarized, mac builds notify and link to the release page instead of updating in place.
// Flip this to true once an Apple Developer certificate is wired into the build.
const MAC_AUTO_UPDATE_SUPPORTED = false;

let simProcess = null;
let simPort = 0;
let mainWindow = null;

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
		fs.writeFileSync(windowStateFile(), JSON.stringify({ ...b, maximized: win.isMaximized() }));
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
		},
	});

	if (state.maximized) mainWindow.maximize();
	mainWindow.once('ready-to-show', () => mainWindow.show());
	mainWindow.on('close', () => saveWindowState(mainWindow));
	mainWindow.on('closed', () => {
		mainWindow = null;
	});

	// Anything that is not our own scheme is a real external link (Wowhead, GitHub, Discord)
	// and belongs in the user's actual browser, not in this window.
	mainWindow.webContents.setWindowOpenHandler(({ url }) => {
		if (!url.startsWith(ORIGIN)) shell.openExternal(url);
		return { action: 'deny' };
	});
	mainWindow.webContents.on('will-navigate', (event, url) => {
		if (!url.startsWith(ORIGIN)) {
			event.preventDefault();
			shell.openExternal(url);
		}
	});

	mainWindow.loadURL(START_URL);
	return mainWindow;
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

ipcMain.handle('wowsims:start-update', async () => {
	if (process.platform === 'darwin' && !MAC_AUTO_UPDATE_SUPPORTED) {
		await shell.openExternal(RELEASES_URL);
		return { openedExternally: true };
	}
	await autoUpdater.downloadUpdate();
	return { downloading: true };
});

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
	stopSidecar();
});

// Backstops for paths that bypass before-quit.
process.on('exit', stopSidecar);
