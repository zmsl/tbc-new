import * as PresetUtils from '../../core/preset_utils';
import { ConsumesSpec, Debuffs, Drums, IndividualBuffs, PartyBuffs, Profession, PseudoStat, RaidBuffs, Stat, TristateEffect } from '../../core/proto/common';
import { SmitePriest_Options as Options } from '../../core/proto/priest';
import { SavedTalents } from '../../core/proto/ui';
import { Stats } from '../../core/proto_utils/stats';
import { defaultImprovedShadowBoltSettings, defaultRaidBuffMajorDamageCooldowns } from '../../core/proto_utils/utils';
import DefaultApl from './apls/default.apl.json';
import P1Gear from './gear_sets/p1.gear.json';
import P3Gear from './gear_sets/p3.gear.json';
import PreRaidGear from './gear_sets/pre_raid.gear.json';

// A smite priest wears the same spell-damage cloth as any other caster; the shadow tier
// bonuses it happens to carry simply do nothing for Holy Fire and Smite.
export const PRE_RAID_PRESET = PresetUtils.makePresetGear('Pre Raid Preset', PreRaidGear);
export const P1_PRESET = PresetUtils.makePresetGear('P1 Preset', P1Gear);
export const P3_PRESET = PresetUtils.makePresetGear('P3 Preset', P3Gear);

export const ROTATION_PRESET_DEFAULT = PresetUtils.makePresetAPLRotation('Default', DefaultApl);

// Starting weights only -- resim to refine. Spirit is unusually strong here because
// Spiritual Guidance turns 25% of it into spell damage, and crit carries extra value
// because every crit can roll a Surge of Light proc.
export const P3_EP_PRESET = PresetUtils.makePresetEpWeights(
	'P3',
	Stats.fromMap(
		{
			[Stat.StatIntellect]: 0.06,
			[Stat.StatSpirit]: 0.3,
			[Stat.StatSpellDamage]: 1.0,
			[Stat.StatHolyDamage]: 1.0,
			[Stat.StatSpellHitRating]: 1.35,
			[Stat.StatSpellCritRating]: 0.4,
			[Stat.StatSpellHasteRating]: 0.88,
			[Stat.StatMP5]: 0.05,
		},
		{
			[PseudoStat.PseudoStatSchoolHitPercentHoly]: 1.41,
		},
	),
);

// Default talents. Uses the wowhead calculator format, make the talents on
// https://www.wowhead.com/tbc/talent-calc/priest and copy the numbers in the url.
//
// 33/28/0: Discipline to Power Infusion through Force of Will and Focused Power, Holy to
// Surge of Light through Holy Specialization, Divine Fury, Searing Light and Spiritual
// Guidance. Everything else in either tree is healing throughput this sim does not model.
export const StandardTalents = {
	name: 'Smite',
	data: SavedTalents.create({
		talentsString: '5051000130505002501-225051000320152-',
	}),
};

export const DefaultOptions = Options.create({
	classOptions: {},
});

export const DefaultConsumables = ConsumesSpec.create({
	flaskId: 13512, // Flask of Supreme Power
	foodId: 27657, // Blackened Basilisk
	conjuredId: 12662, // Demonic Rune
	mhImbueId: 22522, // Superior Wizard Oil
	potId: 22839, // Destruction Potion
	explosiveId: 30217,
});

export const DefaultRaidBuffs = RaidBuffs.create({
	...defaultRaidBuffMajorDamageCooldowns(),
	arcaneBrilliance: true,
	giftOfTheWild: TristateEffect.TristateEffectImproved,
	powerWordFortitude: TristateEffect.TristateEffectImproved,
	divineSpirit: TristateEffect.TristateEffectImproved,
});

export const DefaultPartyBuffs = PartyBuffs.create({
	manaSpringTotem: TristateEffect.TristateEffectRegular,
	wrathOfAirTotem: TristateEffect.TristateEffectImproved,
	eyeOfTheNight: true,
	chainOfTheTwilightOwl: true,
	drums: Drums.LesserDrumsOfBattle,
});

export const DefaultIndividualBuffs = IndividualBuffs.create({
	blessingOfKings: true,
	blessingOfWisdom: TristateEffect.TristateEffectImproved,
	shadowPriestDps: 0,
});

export const DefaultDebuffs = Debuffs.create({
	improvedSealOfTheCrusader: TristateEffect.TristateEffectImproved,
	judgementOfWisdom: true,
	faerieFire: TristateEffect.TristateEffectImproved,
	curseOfElements: TristateEffect.TristateEffectImproved,
	exposeArmor: TristateEffect.TristateEffectImproved,
	...defaultImprovedShadowBoltSettings(),
});

export const OtherDefaults = {
	channelClipDelay: 100,
	distanceFromTarget: 28,
	profession1: Profession.Enchanting,
	profession2: Profession.Tailoring,
};
