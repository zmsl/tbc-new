import { default as pako } from 'pako';

import { SIM_CATEGORY_KEYS, SimSettingCategories } from '../../../constants/sim_settings';
import { IndividualSimUI } from '../../../individual_sim_ui';
import { Spec } from '../../../proto/common';
import { EventID } from '../../../typed_event';
import { IndividualSimSettings } from '../../../proto/ui';
import { ShareLinkKind, buildShareUrl, defaultShareLinkKind } from '../../../desktop';
import { arrayEquals, getEnumValues } from '../../../utils';
import { IndividualImporter } from '../importers/individual_importer';
import { EnumPicker } from '../../pickers/enum_picker';
import { IndividualExporter } from './individual_exporter';
import i18n from '../../../../i18n/config';

export class IndividualLinkExporter<SpecType extends Spec> extends IndividualExporter<SpecType> {
	private linkKind: ShareLinkKind = defaultShareLinkKind();

	constructor(parent: HTMLElement, simUI: IndividualSimUI<SpecType>) {
		super(parent, simUI, { title: i18n.t('export.link.title'), selectCategories: true });

		const kindContainer = (<div className="exporter-link-kind mb-2" />) as HTMLElement;
		this.body.prepend(kindContainer);

		new EnumPicker<IndividualLinkExporter<SpecType>>(kindContainer, this, {
			id: 'link-exporter-kind',
			label: 'Link type',
			// Offered on the website too, so a link can be made for someone who runs the app;
			// only the default differs by where you are.
			labelTooltip:
				'Web opens in a browser and works for anyone. Desktop app opens straight into the installed sim, and does nothing for someone who does not have it installed.',
			values: [
				{ name: 'Web (wowsims.com)', value: 0 },
				{ name: 'Desktop app', value: 1 },
			],
			// EnumPicker is numeric, so the two map onto 0/1 rather than the string kind.
			getValue: () => (this.linkKind === 'desktop' ? 1 : 0),
			setValue: (eventID: EventID, _modObj: IndividualLinkExporter<SpecType>, newValue: number) => {
				this.linkKind = newValue === 1 ? 'desktop' : 'web';
				this.changedEvent.emit(eventID);
			},
			changedEvent: () => this.changedEvent,
		});
	}

	getData(): string {
		return IndividualLinkExporter.createLink(
			this.simUI,
			(getEnumValues(SimSettingCategories) as Array<SimSettingCategories>).filter(c => this.exportCategories[c]),
			this.linkKind,
		);
	}

	static createLink(simUI: IndividualSimUI<any>, exportCategories?: Array<SimSettingCategories>, kind: ShareLinkKind = defaultShareLinkKind()): string {
		if (!exportCategories) {
			exportCategories = IndividualImporter.DEFAULT_CATEGORIES;
		}

		const proto = simUI.toProto(exportCategories);

		const protoBytes = IndividualSimSettings.toBinary(proto);
		// @ts-ignore Pako did some weird stuff between versions and the @types package doesn't correctly support this syntax for version 2.0.4 but it's completely valid
		// The syntax was removed in 2.1.0 and there were several complaints but the project seems to be largely abandoned now
		const deflated = pako.deflate(protoBytes, { to: 'string' });
		const encoded = btoa(String.fromCharCode(...deflated));

		// Not window.location: in the desktop app that origin is wowsims://app, so an
		// unqualified link would only ever open on the machine that made it. The path comes
		// from the current page either way; only the origin differs.
		const linkUrl = buildShareUrl(kind);
		linkUrl.hash = encoded;
		if (arrayEquals(exportCategories, IndividualImporter.DEFAULT_CATEGORIES)) {
			linkUrl.searchParams.delete(IndividualImporter.CATEGORY_PARAM);
		} else {
			const categoryCharString = exportCategories.map(c => SIM_CATEGORY_KEYS.get(c)).join('');
			linkUrl.searchParams.set(IndividualImporter.CATEGORY_PARAM, categoryCharString);
		}
		return linkUrl.toString();
	}
}
