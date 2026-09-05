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

// Saves through a native Save As dialog when running in the desktop app. Returns false when
// there is no desktop bridge, so callers fall back to the browser download path.
export const saveFileNatively = (fileName: string, contents: string): boolean => {
	const desktop = getDesktop();
	if (!desktop) return false;
	desktop.saveFile(fileName, contents);
	return true;
};
