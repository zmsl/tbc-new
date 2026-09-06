// Replaces the cdnjs Font Awesome stylesheet with a bundled copy, in an already-built
// client tree.
//
// This runs only for the desktop app. The web deploy keeps the CDN link exactly as it is
// in ui/index.html and ui/index_template.html -- those sources are never touched, so the
// site's behaviour is unchanged. An installed desktop app is a different matter: a
// render-blocking stylesheet on a third-party CDN means the window waits on the network
// every launch, and leaves the UI with no icons at all when offline.
//
// Usage: node tools/desktop/bundle_fonts.mjs <built client dir>

import fs from 'node:fs';
import path from 'node:path';

const root = process.argv[2];
if (!root) {
	console.error('usage: bundle_fonts.mjs <built client dir>');
	process.exit(1);
}

// Lives in the desktop project, not the root one: the web build must not gain a
// dependency it does not use, and the root lockfile is what CI installs from.
const FA_PKG = 'desktop/node_modules/@fortawesome/fontawesome-free';
const LOCAL_HREF = '/tbc/fontawesome/css/all.min.css';
// The integrity hash on the source link belongs to the cdnjs build. Carrying it over to a
// different local copy would fail the check and make the browser drop the stylesheet, so
// the replacement is a bare link.
const LOCAL_LINK = `<link rel="stylesheet" href="${LOCAL_HREF}" />`;
const CDN_LINK = /<link[^>]*cdnjs\.cloudflare\.com[^>]*font-awesome[^>]*>/gis;

if (!fs.existsSync(FA_PKG)) {
	console.error(`${FA_PKG} is missing. Run "npm install" in desktop/ first.`);
	process.exit(1);
}

const dest = path.join(root, 'fontawesome');
fs.rmSync(dest, { recursive: true, force: true });
fs.mkdirSync(dest, { recursive: true });
// all.min.css references ../webfonts/, so the two must stay siblings.
for (const dir of ['css', 'webfonts']) {
	fs.cpSync(path.join(FA_PKG, dir), path.join(dest, dir), { recursive: true });
}

const htmlFiles = [];
(function walk(dir) {
	for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
		const full = path.join(dir, entry.name);
		if (entry.isDirectory()) walk(full);
		else if (entry.name.endsWith('.html')) htmlFiles.push(full);
	}
})(root);

let rewritten = 0;
let alreadyLocal = 0;
for (const file of htmlFiles) {
	const before = fs.readFileSync(file, 'utf8');
	if (before.includes(LOCAL_HREF)) {
		alreadyLocal++;
		continue;
	}
	const after = before.replace(CDN_LINK, LOCAL_LINK);
	if (after !== before) {
		fs.writeFileSync(file, after);
		rewritten++;
	}
}

if (rewritten === 0 && alreadyLocal === 0) {
	// Failing loudly matters: this operates on generated output, so a silent no-op would
	// ship a desktop app that still depends on the CDN without anyone noticing.
	console.error(`No Font Awesome CDN link found in any of the ${htmlFiles.length} HTML files under ${root}.`);
	console.error('The link markup in ui/index_template.html probably changed; update CDN_LINK here.');
	process.exit(1);
}

console.log(`Bundled Font Awesome into ${dest} and rewrote ${rewritten} of ${htmlFiles.length} HTML files.`);
