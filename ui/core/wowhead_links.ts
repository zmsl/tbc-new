import { ActionId } from './proto_utils/action_id';

// Ctrl+click (Cmd on macOS) on any item, gem or enchant opens its Wowhead page in a browser.
//
// These elements are not ordinary links: a plain click opens the item picker or the gem
// picker, so there is otherwise no way to reach the Wowhead page from the sim at all. In the
// desktop app that matters more, since there is no address bar to fall back on.

const WOWHEAD_HOSTS = ['wowhead.com', 'wotlkdb.com'];

const isWowheadHref = (href: string): boolean => href.startsWith('http') && WOWHEAD_HOSTS.some(host => href.includes(host));

// data-wowhead is the tooltip parameter string built by buildWowheadTooltipDataset, e.g.
// "domain=tbc&dataEnv=5&lvl=70&item=30975". Rebuilding the canonical URL from the id keeps
// this consistent with the links used everywhere else.
const urlFromTooltipData = (data: string): string | null => {
	const params = new URLSearchParams(data);
	const itemId = Number(params.get('item'));
	if (itemId) return ActionId.makeItemUrl(itemId, Number(params.get('rand')) || undefined);
	const spellId = Number(params.get('spell'));
	if (spellId) return ActionId.makeSpellUrl(spellId);
	return null;
};

// Walks up taking whichever source appears first, rather than checking one kind and then the
// other. A gem socket is an anchor with a Wowhead href sitting inside an item element that
// carries data-wowhead for the whole item -- searching for the dataset first would open the
// item's page when the gem was clicked.
const wowheadUrlFor = (start: HTMLElement): string | null => {
	for (let node: HTMLElement | null = start; node && node !== document.body; node = node.parentElement) {
		if (node instanceof HTMLAnchorElement && isWowheadHref(node.href)) return node.href;
		const data = node.dataset?.wowhead;
		if (data) {
			const url = urlFromTooltipData(data);
			if (url) return url;
		}
	}
	return null;
};

let installed = false;

// The tab rendered under every tooltip by scss/shared/_wowhead_tooltip.scss reads its text
// from here, because the modifier differs by platform: ctrl+click is a right-click on macOS,
// so Cmd is the equivalent there.
const setHintLabel = () => {
	const isMac = /mac/i.test(navigator.platform) || /Mac OS X/.test(navigator.userAgent);
	const label = isMac ? '\u2318+Click to open in Wowhead' : 'Ctrl+Click to open in Wowhead';
	// JSON.stringify supplies the quotes that a CSS content value needs.
	document.documentElement.style.setProperty('--wowhead-hint-label', JSON.stringify(label));
};

export const installWowheadCtrlClick = () => {
	if (installed) return;
	installed = true;
	setHintLabel();

	document.addEventListener(
		'click',
		event => {
			if (!(event.ctrlKey || event.metaKey) || event.button !== 0) return;
			const target = event.target as HTMLElement | null;
			if (!target) return;
			const url = wowheadUrlFor(target);
			if (!url) return;

			// Capture phase and stopping propagation: the pickers listen for plain clicks and
			// would open their modal on top of this otherwise.
			event.preventDefault();
			event.stopPropagation();
			// A new tab on the website; the desktop shell's window-open handler turns this
			// into the user's real browser.
			window.open(url, '_blank', 'noopener');
		},
		{ capture: true },
	);
};
