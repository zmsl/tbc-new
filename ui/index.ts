/// <reference types="vite/client" />
import './shared/bootstrap_overrides';

import { getFavoriteSim, isFavoriteSim, redirectToFavoriteSim, setFavoriteSim, simPathOf } from './core/favorite_sim';

import * as Popper from '@popperjs/core';
import { Dropdown, Modal, Tab } from 'bootstrap';
import { Chart, registerables } from 'chart.js';
import tippy from 'tippy.js';

declare global {
	interface Window {
		Popper: any;
		bootstrap: any;
	}
}

Chart.register(...registerables);
Chart.defaults.color = 'white';

tippy.setDefaultProps({ arrow: false, allowHTML: true });
window.Popper = Popper;
window.bootstrap = { Dropdown, Modal, Tab };

// Force scroll to top when refreshing
if (history.scrollRestoration) {
	history.scrollRestoration = 'manual';
} else {
	window.onbeforeunload = function () {
		window.scrollTo(0, 0);
	};
}

function docReady(fn: any) {
	// see if DOM is already available
	if (document.readyState === 'complete' || document.readyState === 'interactive') {
		// call on next available tick
		setTimeout(fn, 1);
	} else {
		document.addEventListener('DOMContentLoaded', fn);
	}
}

// Star on each sim card, toggling which sim opens on startup. Added here rather than in
// index.html so the 21 static entries do not each need editing.
function installFavoriteStars() {
	document.querySelectorAll<HTMLAnchorElement>('a.sim-link[href]').forEach(link => {
		const star = document.createElement('span');
		star.className = 'sim-link-favorite';
		star.setAttribute('role', 'button');
		star.setAttribute('tabindex', '0');

		const render = () => {
			const on = isFavoriteSim(link.href);
			star.classList.toggle('sim-link-favorite--on', on);
			star.innerHTML = `<i class="${on ? 'fas' : 'far'} fa-star"></i>`;
			star.title = on ? 'Opens on startup. Click to clear.' : 'Open this sim on startup';
		};

		const toggle = (event: Event) => {
			// Inside the card's anchor, so without this the click navigates instead.
			event.preventDefault();
			event.stopPropagation();
			setFavoriteSim(isFavoriteSim(link.href) ? null : simPathOf(link.href));
			// Every card, since favouriting one clears another.
			document.querySelectorAll<HTMLElement>('.sim-link-favorite').forEach(other => other.dispatchEvent(new CustomEvent('favorite:refresh')));
		};

		star.addEventListener('click', toggle);
		star.addEventListener('keydown', event => {
			if ((event as KeyboardEvent).key === 'Enter' || (event as KeyboardEvent).key === ' ') toggle(event);
		});
		star.addEventListener('favorite:refresh', render);

		render();
		link.appendChild(star);
	});
}

// Before anything renders: with a favourite set this page is only a redirect.
redirectToFavoriteSim();

docReady(function () {
	installFavoriteStars();
	if (getFavoriteSim()) document.body.classList.add('has-favorite-sim');
	document.body.classList.add('ready');
});
