import { LOCAL_STORAGE_PREFIX } from './constants/other';

// A favourited sim opens automatically instead of the landing page, in the browser and in
// the desktop app alike (a plain launch lands on the landing page, so the same redirect
// covers both). A direct link to a sim is untouched -- the redirect only ever runs on the
// landing page's own entry script.
//
// Stored as a site-relative path so it survives moving between the website and the app,
// whose origins differ.

const STORAGE_KEY = `${LOCAL_STORAGE_PREFIX}_favorite_sim.v1`;

// Suppresses the redirect, so the landing page stays reachable with a favourite set.
export const HOME_PARAM = 'home';

export const getFavoriteSim = (): string | null => {
	try {
		return localStorage.getItem(STORAGE_KEY);
	} catch {
		// Private windows and blocked site data throw rather than returning null.
		return null;
	}
};

const listeners = new Set<() => void>();

// There is only ever one favourite: setting a new one replaces the old, and setting null
// clears it entirely.
export const setFavoriteSim = (path: string | null) => {
	try {
		if (path) localStorage.setItem(STORAGE_KEY, path);
		else localStorage.removeItem(STORAGE_KEY);
	} catch {
		// Not being able to remember the choice is not worth breaking the page over.
	}
	listeners.forEach(listener => listener());
};

// Stars appear in several places at once -- the landing page, the sim's spec switcher, the
// toolbar -- and toggling one has to restyle all of them, since favouriting anything clears
// whatever was favourited before.
export const onFavoriteChange = (listener: () => void) => {
	listeners.add(listener);
	return () => listeners.delete(listener);
};

// Compared with a trailing slash so "/tbc/warrior/dps" and "/tbc/warrior/dps/" are the same.
export const simPathOf = (href: string): string => {
	const path = new URL(href, window.location.href).pathname;
	return path.endsWith('/') ? path : `${path}/`;
};

// Spec pages load the landing page's entry script too (index_template.html pulls in
// ui/index.ts alongside the spec's own), so this has to establish for itself that it is
// actually on the landing page. Without it, every sim that is not the favourite bounces to
// the favourite and becomes unreachable -- including any direct or protocol link.
const isLandingPage = (): boolean => window.location.pathname.split('/').filter(Boolean).length <= 1;

export const redirectToFavoriteSim = () => {
	if (!isLandingPage()) return;
	if (new URLSearchParams(window.location.search).has(HOME_PARAM)) return;
	const favorite = getFavoriteSim();
	if (!favorite || favorite === simPathOf(window.location.href)) return;
	window.location.replace(favorite);
};

// Adds a star to every class row under `root`.
//
// Deliberately on the class row rather than each spec: the row is what is visible without
// opening anything, and the landing page and the sim's spec switcher share this markup, so
// one pass covers both. A class that expands into a spec menu favourites the first sim in
// it; a class that is itself a single link favourites that link.
//
// Rows come in two shapes and both occur on the landing page: most classes are wrapped in a
// .sim-link-dropdown, but the single-sim ones (Rogue, Hunter, Mage, Warlock) are a bare
// anchor with no wrapper at all.
const favoriteRowsIn = (root: ParentNode): HTMLElement[] => {
	const rows = new Set<HTMLElement>();
	root.querySelectorAll<HTMLElement>('.sim-link-dropdown.dropend').forEach(row => rows.add(row));
	root.querySelectorAll<HTMLElement>('a.sim-link[href]').forEach(link => {
		// A spec inside an open menu, not a class row.
		if (link.closest('.dropdown-menu')) return;
		// Already represented by the wrapper picked up above.
		if (link.closest('.sim-link-dropdown')) return;
		rows.add(link);
	});
	return [...rows];
};

export const installFavoriteStars = (root: ParentNode) => {
	favoriteRowsIn(root).forEach(row => {
		if (row.dataset.favoriteStar) return;
		const target = row.matches('a.sim-link[href]') ? (row as HTMLAnchorElement) : row.querySelector<HTMLAnchorElement>('a.sim-link[href]');
		if (!target) return;
		row.dataset.favoriteStar = '1';

		const path = simPathOf(target.href);
		const star = document.createElement('span');
		star.className = 'sim-link-favorite';
		star.setAttribute('role', 'button');
		star.setAttribute('tabindex', '0');

		const render = () => {
			const on = getFavoriteSim() === path;
			star.classList.toggle('sim-link-favorite--on', on);
			star.innerHTML = `<i class="${on ? 'fas' : 'far'} fa-star"></i>`;
			star.title = on ? 'Opens on startup. Click to clear.' : 'Open this sim on startup';
		};

		const toggle = (event: Event) => {
			// Stops the click reaching the row behind, which would navigate or open the menu.
			event.preventDefault();
			event.stopPropagation();
			setFavoriteSim(getFavoriteSim() === path ? null : path);
		};

		star.addEventListener('click', toggle);
		star.addEventListener('keydown', event => {
			const key = (event as KeyboardEvent).key;
			if (key === 'Enter' || key === ' ') toggle(event);
		});
		onFavoriteChange(render);

		render();
		row.appendChild(star);
	});
};
