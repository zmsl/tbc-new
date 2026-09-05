// Bridge exposed by the Electron desktop shell's preload script (desktop/preload.js).
//
// The same bundle ships to the website, where none of this exists, so every caller must
// feature detect with getDesktop() rather than assuming the bridge is there.

export interface DesktopUpdateInfo {
	version: string;
	// False on platforms that cannot replace themselves in place — notably an unsigned
	// macOS bundle, which Squirrel.Mac refuses to update. Those builds open the release
	// page instead of updating.
	canSelfUpdate: boolean;
	releaseUrl: string;
}

export interface DesktopStartUpdateResult {
	downloading?: boolean;
	openedExternally?: boolean;
}

export interface DesktopBridge {
	isDesktop: true;
	platform: string;
	onUpdateAvailable: (callback: (info: DesktopUpdateInfo) => void) => () => void;
	onUpdateProgress: (callback: (percent: number) => void) => () => void;
	onUpdateReady: (callback: () => void) => () => void;
	onUpdateError: (callback: (message: string) => void) => () => void;
	// Chromium's GPU feature status, as chrome://gpu reports it.
	getGpuStatus: () => Promise<{ featureStatus: Record<string, string>; hardwareAccelerationDisabled: boolean }>;
	onMenuCommand: (callback: (command: 'run-sim' | 'import' | 'export') => void) => () => void;
	reportSimProgress: (fraction: number) => void;
	reportSimDone: (summary: { notify: boolean; body?: string }) => void;
	copyText: (text: string) => Promise<boolean>;
	saveFile: (fileName: string, contents: string) => Promise<{ saved: boolean; filePath?: string }>;
	startUpdate: () => Promise<DesktopStartUpdateResult>;
	installUpdate: () => Promise<void>;
}

declare global {
	interface Window {
		wowsimsDesktop?: DesktopBridge;
	}
}

export const getDesktop = (): DesktopBridge | undefined => window.wowsimsDesktop;

export const isDesktop = (): boolean => !!window.wowsimsDesktop;

// One duration for every transition in the desktop app, so a tooltip, a modal and a
// collapsing panel all take the same wall-clock time to happen.
//
// 150ms is not an invented number: it is already this project's house value, used by
// $btn-transition, --link-transition, the sim header, the icon picker and the selector
// modal. Aligning on it means only the slow outliers need touching, and everything else
// already matches.
//
// Measured as shipped, those outliers were tippy tooltips at 300ms, Bootstrap modal
// dialogs at 300ms, and collapses at 350ms. On a web page that reads as polish; in an
// installed app, where every other window responds immediately, it reads as lag. Building
// and inserting a tooltip costs ~1ms, so the wait was very nearly all animation.
//
// Deliberately desktop only. The website keeps the durations it was built with.
const DESKTOP_MOTION_MS = 150;

const DESKTOP_TOOLTIP_DURATION: [number, number] = [DESKTOP_MOTION_MS, DESKTOP_MOTION_MS];

// Only the outliers. Bootstrap's .fade is already 150ms, and .dropdown-menu has no
// transition at all (measured "all 0s"), so neither appears here. Nor does offcanvas --
// this project never imports bootstrap/scss/offcanvas.
//
// Overriding just the duration leaves Bootstrap's easing and properties alone. These match
// Bootstrap's own specificity and are injected afterwards, so they win without !important,
// and the prefers-reduced-motion block that sets transition: none still overrides them.
const DESKTOP_MOTION_CSS = `
.modal.fade .modal-dialog { transition-duration: ${DESKTOP_MOTION_MS}ms; }
.collapsing, .collapsing.collapse-horizontal { transition-duration: ${DESKTOP_MOTION_MS}ms; }
`;

let tweaksApplied = false;

// Call once, before any UI builds tooltips. setDefaultProps only affects instances created
// after it runs.
export const applyDesktopTweaks = (setDefaultProps: (props: { duration: [number, number] }) => void) => {
	if (tweaksApplied || !isDesktop()) return;
	tweaksApplied = true;
	setDefaultProps({ duration: DESKTOP_TOOLTIP_DURATION });

	const style = document.createElement('style');
	style.dataset.wowsimsDesktop = 'motion';
	style.textContent = DESKTOP_MOTION_CSS;
	document.head.appendChild(style);

	installMenuCommands();
	installFileDrop();
	installTitleBar();
};

// The window runs without its native title strip so the app can own that space. The system
// still draws min/max/close over the right-hand end on Windows and Linux, and keeps the
// traffic lights at the left on macOS, so the usable region is the middle.
//
// env(titlebar-area-*) is what Chromium exposes for the part NOT covered by the window
// controls. macOS does not support it, hence the platform-specific left padding.
const TITLEBAR_HEIGHT_PX = 32;
const MAC_TRAFFIC_LIGHT_WIDTH_PX = 78;

// Resize bands live at the very edge of the window. A draggable region sitting flush against
// that edge swallows them, so the drag area is inset and the outer bar is not draggable.
const RESIZE_EDGE_PX = 4;

// Gap between the title strip and the page content. Painted in the page background, which
// matches the strip, so it reads as a slightly taller bar rather than a visible band.
const TITLEBAR_GAP_PX = 6;

const TITLEBAR_CSS = `
.wowsims-titlebar {
	position: fixed;
	top: 0;
	left: 0;
	/* Window controls sit at the right on Windows and Linux, so the usable strip stops short
	   of the window edge; titlebar-area-width is that span. macOS reports neither. */
	width: env(titlebar-area-width, 100%);
	height: env(titlebar-area-height, ${TITLEBAR_HEIGHT_PX}px);
	display: flex;
	align-items: center;
	padding: ${RESIZE_EDGE_PX}px ${RESIZE_EDGE_PX}px 0 ${RESIZE_EDGE_PX}px;
	background: #1d2021;
	color: #d8d4cf;
	font-size: 0.75rem;
	z-index: 1080;
	user-select: none;
}
.wowsims-titlebar--mac { padding-left: ${MAC_TRAFFIC_LIGHT_WIDTH_PX}px; }

/* Only this inner strip drags the window, so the outer padding above stays available to the
   window's own resize handles. */
.wowsims-titlebar__drag {
	flex: 1 1 auto;
	display: flex;
	align-items: center;
	gap: 0.5rem;
	min-width: 0;
	height: 100%;
	-webkit-app-region: drag;
}
.wowsims-titlebar__icon { width: 16px; height: 16px; flex: none; }
.wowsims-titlebar__app { font-weight: 600; white-space: nowrap; flex: none; }
.wowsims-titlebar__location,
.wowsims-titlebar__dps { opacity: 0.65; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.wowsims-titlebar__gap { flex: none; width: 1.25rem; }
.wowsims-titlebar__share {
	-webkit-app-region: no-drag;
	flex: none;
	display: inline-flex;
	align-items: center;
	gap: 0.35rem;
	height: 22px;
	padding: 0 0.6rem;
	border: 1px solid rgba(255, 255, 255, 0.18);
	border-radius: 4px;
	background: transparent;
	color: inherit;
	font-size: 0.75rem;
	cursor: pointer;
	transition-duration: ${DESKTOP_MOTION_MS}ms;
}
.wowsims-titlebar__share:hover { background: rgba(255, 255, 255, 0.1); }
.wowsims-titlebar__divider {
	flex: none;
	width: 1px;
	height: 60%;
	margin: 0 0.5rem;
	background: rgba(255, 255, 255, 0.18);
}

/* .sim-ui is already the scroller (max-height:100vh; overflow-y:auto), so page-level
   scrolling is vestigial -- and once the title strip shifts everything down it becomes a
   real 32px second scrollbar. Pin the page and let .sim-ui scroll, as it was meant to. */
html, body {
	overflow: hidden;
	/* Paint the canvas explicitly. Any strip of the window the app's own layers do not cover
	   would otherwise show the default (light) canvas as a border. */
	background-color: #1d2021;
}

/* Shifting this one element down keeps the sticky header directly beneath the title strip.
   Touching html/body heights instead would fight their height:100%. */
.sim-ui {
	margin-top: calc(env(titlebar-area-height, ${TITLEBAR_HEIGHT_PX}px) + ${TITLEBAR_GAP_PX}px);
	max-height: calc(100vh - env(titlebar-area-height, ${TITLEBAR_HEIGHT_PX}px) - ${TITLEBAR_GAP_PX}px);
}
.sim-ui .sim-root { min-height: calc(100vh - env(titlebar-area-height, ${TITLEBAR_HEIGHT_PX}px) - ${TITLEBAR_GAP_PX}px); }
`;

// Set by the sim UI, which is the part that knows how to build a share link and show a
// toast. Keeps this module free of any dependency on the sim UI.
let shareHandler: (() => void) | null = null;
export const setDesktopShareHandler = (handler: () => void) => {
	shareHandler = handler;
};

let dpsElem: HTMLElement | null = null;

// Last sim result, shown in the title strip. No-op on the website.
export const setTitlebarDps = (text: string) => {
	if (dpsElem) dpsElem.textContent = text;
};

const installTitleBar = () => {
	const style = document.createElement('style');
	style.dataset.wowsimsDesktop = 'titlebar';
	style.textContent = TITLEBAR_CSS;
	document.head.appendChild(style);

	const bar = document.createElement('div');
	bar.className = 'wowsims-titlebar';
	if (getDesktop()?.platform === 'darwin') bar.classList.add('wowsims-titlebar--mac');

	const drag = document.createElement('div');
	drag.className = 'wowsims-titlebar__drag';

	const icon = document.createElement('img');
	icon.className = 'wowsims-titlebar__icon';
	icon.src = '/tbc/assets/favicon_io/favicon-32x32.png';
	icon.alt = '';

	const appName = document.createElement('span');
	appName.className = 'wowsims-titlebar__app';
	appName.textContent = 'WoWSims TBC';

	const location = document.createElement('span');
	location.className = 'wowsims-titlebar__location';

	dpsElem = document.createElement('span');
	dpsElem.className = 'wowsims-titlebar__dps';

	const gap = () => {
		const g = document.createElement('span');
		g.className = 'wowsims-titlebar__gap';
		return g;
	};
	drag.append(icon, appName, gap(), location, gap(), dpsElem);

	const share = document.createElement('button');
	share.className = 'wowsims-titlebar__share';
	share.type = 'button';
	share.title = 'Copy a share link for the current setup';
	share.innerHTML = '<i class="fas fa-link"></i><span>Share</span>';
	share.addEventListener('click', () => shareHandler?.());

	const divider = document.createElement('span');
	divider.className = 'wowsims-titlebar__divider';

	bar.append(drag, share, divider);
	document.body.prepend(bar);

	installLocationTracking(location);
};

// "DPS Warrior > Gear". The spec comes from the sidebar title and the tab from whichever
// nav-link is active, so this follows the UI rather than duplicating its state.
const installLocationTracking = (target: HTMLElement) => {
	const update = () => {
		const spec = document.querySelector('.sim-link-title')?.textContent?.trim() || '';
		const tab = document.querySelector('.sim-tabs .nav-link.active')?.textContent?.trim() || '';
		target.textContent = [spec, tab].filter(Boolean).join('  \u203a  ');
	};

	// The tabs use data-bs-toggle="tab", and Bootstrap's shown.bs.tab bubbles, so a document
	// level listener catches every tab change. Binding to .sim-tabs directly does not work:
	// installTitleBar runs as the first statement of the SimUI constructor, before the header
	// exists, so the element is not there to observe yet.
	document.addEventListener('shown.bs.tab', update);

	// Same reason -- poll briefly for the sidebar title so the initial value is not blank.
	let attempts = 0;
	const seed = () => {
		update();
		if (++attempts < 40 && !document.querySelector('.sim-link-title')) setTimeout(seed, 100);
	};
	seed();
};

// The native menu has no idea what a sim is, so menu items just say what the user asked for
// and the renderer drives the existing UI -- clicking the same controls a mouse would,
// rather than duplicating their behaviour and letting the two drift apart.
const MENU_COMMAND_SELECTORS: Record<string, string> = {
	'run-sim': '.sim-sidebar-action-button.dps-action',
	import: '.import-dropdown > .import-link',
	export: '.export-dropdown > .export-link',
};

const installMenuCommands = () => {
	getDesktop()?.onMenuCommand(command => {
		const target = document.querySelector<HTMLElement>(MENU_COMMAND_SELECTORS[command]);
		// Disabled during a run, which is exactly the behaviour the menu item should inherit.
		if (target && !(target as HTMLButtonElement).disabled) target.click();
	});
};

// Dropping a settings file on the window is the desktop equivalent of Import -> JSON. The
// browser's default for a dropped file is to navigate to it, which in this window would
// mean leaving the app entirely, so both handlers are needed even to do nothing.
const installFileDrop = () => {
	document.addEventListener('dragover', event => event.preventDefault());
	document.addEventListener('drop', event => {
		event.preventDefault();
		const file = event.dataTransfer?.files?.[0];
		if (!file || !file.name.endsWith('.json')) return;
		file.text().then(contents => {
			document.dispatchEvent(new CustomEvent('wowsims:import-json', { detail: contents }));
		});
	});
};

// The public site share links must point at, since an installed app's own origin
// (wowsims://app) is meaningless to anyone else. Only used for links meant to leave this
// machine -- in-app navigation between specs keeps using the real origin.
export const PUBLIC_SITE_ORIGIN = 'https://wowsims.com';

// The installed app's own origin. A link with this origin is routed to the app by the OS,
// because the shell registers itself as the handler for the scheme. It means nothing to a
// browser, and nothing at all on a machine without the app installed.
export const DESKTOP_ORIGIN = 'wowsims://app';

export type ShareLinkKind = 'web' | 'desktop';

// Which origin a share link carries. Web links open for anyone; desktop links open straight
// into the installed app, skipping the browser.
export const buildShareUrl = (kind: ShareLinkKind): URL =>
	new URL(window.location.pathname + window.location.search, kind === 'desktop' ? DESKTOP_ORIGIN : PUBLIC_SITE_ORIGIN);

// In the app, a link you make is most likely for someone else who also runs the app.
export const defaultShareLinkKind = (): ShareLinkKind => (isDesktop() ? 'desktop' : 'web');

// Sim lifecycle, forwarded to the OS: taskbar progress, keeping the machine awake, and a
// notification when a backgrounded run finishes. No-ops on the website.
export const reportSimProgress = (completed: number, total: number) => {
	getDesktop()?.reportSimProgress(total > 0 ? completed / total : 2);
};

// Must be called when a run ends for any reason -- finished, aborted, or errored -- or the
// taskbar progress bar and the sleep blocker stay behind.
export const reportSimDone = (summary?: { body?: string }) => {
	getDesktop()?.reportSimDone({ notify: !!summary, body: summary?.body });
};

// Copies through the OS clipboard in the desktop app, which -- unlike navigator.clipboard --
// works whether or not the document currently has focus. Falls back to the web API.
export const copyText = async (text: string): Promise<void> => {
	const desktop = getDesktop();
	if (desktop) {
		await desktop.copyText(text);
		return;
	}
	await navigator.clipboard.writeText(text);
};

// Saves through a native Save As dialog when running in the desktop app. Returns false when
// there is no desktop bridge, so callers fall back to the browser download path.
export const saveFileNatively = (fileName: string, contents: string): boolean => {
	const desktop = getDesktop();
	if (!desktop) return false;
	desktop.saveFile(fileName, contents);
	return true;
};
