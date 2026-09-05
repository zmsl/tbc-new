import { Phase } from '../../core/constants/other';
import * as PresetUtils from '../../core/preset_utils';
import { ConsumesSpec, Debuffs, Drums, IndividualBuffs, PartyBuffs, Profession, PseudoStat, RaidBuffs, Stat, TristateEffect } from '../../core/proto/common';
import { SmitePriest_Options as Options } from '../../core/proto/priest';
import { SavedTalents } from '../../core/proto/ui';
import { Stats } from '../../core/proto_utils/stats';
import { defaultImprovedShadowBoltSettings, defaultRaidBuffMajorDamageCooldowns } from '../../core/proto_utils/utils';
import DefaultApl from './apls/default.apl.json';
import P1Gear from './gear_sets/p1.gear.json';
import P2Gear from './gear_sets/p2.gear.json';
import P3Gear from './gear_sets/p3.gear.json';
import P4Gear from './gear_sets/p4.gear.json';
import P5Gear from './gear_sets/p5.gear.json';
import PreRaidGear from './gear_sets/pre_raid.gear.json';

// Built by simming rather than copied from a guide: no smite-specific BiS list exists, and the
// shadow ones are full of +shadow damage that does nothing here. Every slot was chosen by
// running the sim over that phase's top EP candidates and keeping whatever measured best, so
// on-use and proc trinkets are valued by what they actually do. Assumes Enchanting + Tailoring.
export const PRE_RAID_PRESET = PresetUtils.makePresetGear('Pre Raid Preset', PreRaidGear, { phase: Phase.Phase1 });
export const P1_PRESET = PresetUtils.makePresetGear('P1 Preset', P1Gear, { phase: Phase.Phase1 });
export const P2_PRESET = PresetUtils.makePresetGear('P2 Preset', P2Gear, { phase: Phase.Phase2 });
export const P3_PRESET = PresetUtils.makePresetGear('P3 Preset', P3Gear, { phase: Phase.Phase3 });
export const P4_PRESET = PresetUtils.makePresetGear('P4 Preset', P4Gear, { phase: Phase.Phase4 });
export const P5_PRESET = PresetUtils.makePresetGear('P5 Preset', P5Gear, { phase: Phase.Phase5 });

export const ROTATION_PRESET_DEFAULT = PresetUtils.makePresetAPLRotation('Default', DefaultApl);

// Measured by the sim's own stat-weight run on the matching gear set, normalised to spell
// damage. Haste leads once Shadowfiend keeps the mana bar afloat; before that, hit and the
// regen stats are worth far more, which is why the two sets differ so much.
export const PRE_RAID_EP_PRESET = PresetUtils.makePresetEpWeights(
	'Pre-Raid',
	Stats.fromMap(
		{
			[Stat.StatIntellect]: 0.66,
			[Stat.StatSpirit]: 0.68,
			[Stat.StatSpellDamage]: 1.0,
			[Stat.StatHolyDamage]: 0.85,
			[Stat.StatSpellHitRating]: 0.79,
			[Stat.StatSpellCritRating]: 0.54,
			[Stat.StatSpellHasteRating]: 1.52,
			[Stat.StatMP5]: 0.42,
		},
		{
			[PseudoStat.PseudoStatSchoolHitPercentHoly]: 0.79,
		},
	),
);

export const P3_EP_PRESET = PresetUtils.makePresetEpWeights(
	'P3',
	Stats.fromMap(
		{
			[Stat.StatIntellect]: 0.05,
			[Stat.StatSpirit]: 0.34,
			[Stat.StatSpellDamage]: 1.0,
			[Stat.StatHolyDamage]: 0.87,
			[Stat.StatSpellHitRating]: 0.29,
			[Stat.StatSpellCritRating]: 0.22,
			[Stat.StatSpellHasteRating]: 1.61,
			[Stat.StatMP5]: 0.17,
		},
		{
			[PseudoStat.PseudoStatSchoolHitPercentHoly]: 0.29,
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
