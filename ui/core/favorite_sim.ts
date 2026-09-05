import { LOCAL_STORAGE_PREFIX } from './constants/other';

// A favourited spec opens automatically instead of the landing page, in the browser and in
// the desktop app alike (the app starts at the landing page, so the same redirect covers it).
//
// Stored as a site-relative path so it survives moving between the website and the app, whose
// origins differ.

const STORAGE_KEY = `${LOCAL_STORAGE_PREFIX}_favorite_sim.v1`;

// Suppresses the redirect, so the landing page stays reachable even with a favourite set.
export const HOME_PARAM = 'home';

export const getFavoriteSim = (): string | null => {
	try {
		return localStorage.getItem(STORAGE_KEY);
	} catch {
		// Private windows and blocked site data both throw rather than returning null.
		return null;
	}
};

export const setFavoriteSim = (path: string | null) => {
	try {
		if (path) localStorage.setItem(STORAGE_KEY, path);
		else localStorage.removeItem(STORAGE_KEY);
	} catch {
		// Not being able to remember the choice is not worth breaking the page over.
	}
};

// Paths are compared with a trailing slash so "/tbc/warrior/dps" and "/tbc/warrior/dps/"
// count as the same sim.
export const simPathOf = (href: string): string => {
	const path = new URL(href, window.location.href).pathname;
	return path.endsWith('/') ? path : `${path}/`;
};

export const isFavoriteSim = (href: string): boolean => getFavoriteSim() === simPathOf(href);

// Called on the landing page. Uses replace() so the redirect does not sit in history and
// trap Back on the page it just left.
export const redirectToFavoriteSim = () => {
	if (new URLSearchParams(window.location.search).has(HOME_PARAM)) return;
	const favorite = getFavoriteSim();
	if (!favorite || favorite === simPathOf(window.location.href)) return;
	window.location.replace(favorite);
};
