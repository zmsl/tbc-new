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
};
