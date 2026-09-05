import { IndividualSimUI } from '../../../individual_sim_ui';
import { Spec } from '../../../proto/common';
import { Database } from '../../../proto_utils/database';
import { TypedEvent } from '../../../typed_event';
import { IndividualImporter } from './individual_importer';
import { IndividualLinkImporter } from './individual_link_importer';

// Opening a shared link normally means the page reading its own hash on load
// (individual_sim_ui.tsx). That cannot happen in the desktop app, which has no address bar
// to paste a link into, so this offers the same thing as an explicit paste box.
//
// Not gated to the desktop app: pasting a link is a reasonable thing to want on the website
// too, and IndividualLinkImporter.tryParseUrlLocation already accepts any URL.
export class IndividualLinkPasteImporter<SpecType extends Spec> extends IndividualImporter<SpecType> {
	constructor(parent: HTMLElement, simUI: IndividualSimUI<SpecType>) {
		super(parent, simUI, { title: 'Link', allowFileUpload: false });

		this.descriptionElem.appendChild(
			<div>
				<p>Paste a wowsims share link to load the settings it contains.</p>
				<p>Only the categories the link was exported with are replaced; everything else keeps its current value.</p>
			</div>,
		);
	}

	async onImport(data: string) {
		const trimmed = data.trim();
		if (!trimmed) throw new Error('No link provided.');

		let url: URL;
		try {
			url = new URL(trimmed);
		} catch {
			throw new Error("That doesn't look like a link. Paste the whole URL, including the part after the #.");
		}

		let parsed: ReturnType<typeof IndividualLinkImporter.tryParseUrlLocation>;
		try {
			parsed = IndividualLinkImporter.tryParseUrlLocation(url);
		} catch {
			throw new Error('The settings in that link could not be read. It may be truncated or from a different sim.');
		}
		// A link with no hash parses without throwing but carries nothing.
		if (!parsed) throw new Error('That link has no settings attached to it.');

		if (parsed.settings.player?.equipment) {
			await Database.loadLeftoversIfNecessary(parsed.settings.player.equipment);
		}
		this.simUI.fromProto(TypedEvent.nextEventID(), parsed.settings, parsed.categories);
		this.close();
	}
}
