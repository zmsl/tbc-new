import { IconSize } from '../player_class';
import { PlayerSpec } from '../player_spec';
import { Class, Spec } from '../proto/common';
import { getSpecSiteUrl } from '../proto_utils/utils';

export class Priest extends PlayerSpec<Spec.SpecPriest> {
	static specIndex = 0;
	static specID = Spec.SpecPriest as Spec.SpecPriest;
	static classID = Class.ClassPriest as Class.ClassPriest;
	static friendlyName = 'Shadow';
	static simLink = getSpecSiteUrl('priest', 'dps');

	static isTankSpec = false;
	static isHealingSpec = false;
	static isRangedDpsSpec = true;
	static isMeleeDpsSpec = false;
	static canDualWield = false;

	readonly specIndex = Priest.specIndex;
	readonly specID = Priest.specID;
	readonly classID = Priest.classID;
	readonly friendlyName = Priest.friendlyName;
	readonly simLink = Priest.simLink;

	readonly isTankSpec = Priest.isTankSpec;
	readonly isHealingSpec = Priest.isHealingSpec;
	readonly isRangedDpsSpec = Priest.isRangedDpsSpec;
	readonly isMeleeDpsSpec = Priest.isMeleeDpsSpec;

	readonly canDualWield = Priest.canDualWield;

	static getIcon = (size: IconSize): string => {
		return `https://wow.zamimg.com/images/wow/icons/${size}/spell_shadow_shadowwordpain.jpg`;
	};

	getIcon = (size: IconSize): string => {
		return Priest.getIcon(size);
	};
}

export class SmitePriest extends PlayerSpec<Spec.SpecSmitePriest> {
	static specIndex = 1;
	static specID = Spec.SpecSmitePriest as Spec.SpecSmitePriest;
	static classID = Class.ClassPriest as Class.ClassPriest;
	static friendlyName = 'Smite';
	static simLink = getSpecSiteUrl('priest', 'smite');

	static isTankSpec = false;
	static isHealingSpec = false;
	static isRangedDpsSpec = true;
	static isMeleeDpsSpec = false;
	static canDualWield = false;

	readonly specIndex = SmitePriest.specIndex;
	readonly specID = SmitePriest.specID;
	readonly classID = SmitePriest.classID;
	readonly friendlyName = SmitePriest.friendlyName;
	readonly simLink = SmitePriest.simLink;

	readonly isTankSpec = SmitePriest.isTankSpec;
	readonly isHealingSpec = SmitePriest.isHealingSpec;
	readonly isRangedDpsSpec = SmitePriest.isRangedDpsSpec;
	readonly isMeleeDpsSpec = SmitePriest.isMeleeDpsSpec;

	readonly canDualWield = SmitePriest.canDualWield;

	static getIcon = (size: IconSize): string => {
		return `https://wow.zamimg.com/images/wow/icons/${size}/spell_holy_searinglight.jpg`;
	};

	getIcon = (size: IconSize): string => {
		return SmitePriest.getIcon(size);
	};
}
