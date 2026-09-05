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
