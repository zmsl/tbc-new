import * as InputHelpers from '../core/components/input_helpers';
import { Player } from '../core/player';
import { WarriorTalents } from '../core/proto/warrior';
import { TypedEvent } from '../core/typed_event';
import { WarriorShout, WarriorStance } from '../core/proto/warrior';
import { ActionId } from '../core/proto_utils/action_id';
import { WarriorSpecs } from '../core/proto_utils/utils';
import i18n from '../i18n/config.js';

// Configuration for class-specific UI elements on the settings tab.
// These don't need to be in a separate file but it keeps things cleaner.
export const ShoutPicker = <SpecType extends WarriorSpecs>() =>
	InputHelpers.makeClassOptionsEnumIconInput<SpecType, WarriorShout>({
		fieldName: 'defaultShout',
		label: i18n.t('settings_tab.other.default_shout.label'),
		labelTooltip: i18n.t('settings_tab.other.default_shout.label'),
		values: [
			{ actionId: ActionId.fromSpellId(2048), value: WarriorShout.WarriorShoutBattle },
			{ actionId: ActionId.fromSpellId(469), value: WarriorShout.WarriorShoutCommanding },
		],
	});
export const StancePicker = <SpecType extends WarriorSpecs>() =>
	InputHelpers.makeClassOptionsEnumIconInput<SpecType, WarriorStance>({
		fieldName: 'defaultStance',
		label: i18n.t('settings_tab.other.default_stance.label'),
		labelTooltip: i18n.t('settings_tab.other.default_stance.label'),
		values: [
			{ actionId: ActionId.fromSpellId(2457), value: WarriorStance.WarriorStanceBattle },
			{ actionId: ActionId.fromSpellId(2458), value: WarriorStance.WarriorStanceBerserker },
			{ actionId: ActionId.fromSpellId(71), value: WarriorStance.WarriorStanceDefensive },
		],
	});

export const StartingRage = <SpecType extends WarriorSpecs>() =>
	InputHelpers.makeClassOptionsNumberInput<SpecType>({
		fieldName: 'startingRage',
		label: i18n.t('settings_tab.other.starting_rage.label'),
		labelTooltip: i18n.t('settings_tab.other.starting_rage.tooltip'),
	});

export const StanceSnapshot = <SpecType extends WarriorSpecs>() =>
	InputHelpers.makeClassOptionsBooleanInput<SpecType>({
		fieldName: 'stanceSnapshot',
		label: i18n.t('settings_tab.other.stance_snapshot.label'),
		labelTooltip: i18n.t('settings_tab.other.stance_snapshot.tooltip'),
	});

export const QueueDelay = <SpecType extends WarriorSpecs>() =>
	InputHelpers.makeClassOptionsNumberInput<SpecType>({
		fieldName: 'queueDelay',
		label: i18n.t('settings_tab.other.queue_delay.label'),
		labelTooltip: i18n.t('settings_tab.other.queue_delay.tooltip'),
	});

// Mirrors warrior.DefaultHSRageThreshold in the sim. Both the sim and the rotation
// substitute this when the option is unset, so settings saved before the option existed
// keep behaving as they did. Shown in the picker so the number matches what is used.
// Mirrors sim/warrior: base rage costs, and the talents that discount them. Improved
// Heroic Strike only affects Heroic Strike; Focused Rage affects both.
const HEROIC_STRIKE_RAGE_COST = 15;
const CLEAVE_RAGE_COST = 20;
export const DEFAULT_HS_RAGE_THRESHOLD = 40;

const heroicStrikeRageCost = <SpecType extends WarriorSpecs>(player: Player<SpecType>) => {
	const talents = player.getTalents() as WarriorTalents;
	return Math.max(0, HEROIC_STRIKE_RAGE_COST - (talents.improvedHeroicStrike || 0) - (talents.focusedRage || 0));
};

export const cleaveRageCost = <SpecType extends WarriorSpecs>(player: Player<SpecType>) => {
	const talents = player.getTalents() as WarriorTalents;
	return Math.max(0, CLEAVE_RAGE_COST - (talents.focusedRage || 0));
};

export const hsRageThreshold = <SpecType extends WarriorSpecs>(player: Player<SpecType>) =>
	player.getClassOptions().hsRageThreshold || DEFAULT_HS_RAGE_THRESHOLD;

// A threshold is the ability's own cost plus the rage kept back for Bloodthirst and
// Whirlwind. Cleave's default is derived from Cleave's cost, not from the Heroic Strike
// threshold, so both keep the same reserve whatever the talents discount.
export const cleaveRageThreshold = <SpecType extends WarriorSpecs>(player: Player<SpecType>) =>
	player.getClassOptions().cleaveRageThreshold || cleaveRageCost(player) + (hsRageThreshold(player) - heroicStrikeRageCost(player));

export const HsRageThreshold = <SpecType extends WarriorSpecs>() =>
	InputHelpers.makeClassOptionsNumberInput<SpecType>({
		fieldName: 'hsRageThreshold',
		label: i18n.t('settings_tab.other.hs_rage_threshold.label'),
		labelTooltip: i18n.t('settings_tab.other.hs_rage_threshold.tooltip'),
		positive: true,
		getValue: player => hsRageThreshold(player),
		changeEmitter: player => TypedEvent.onAny([player.specOptionsChangeEmitter, player.talentsChangeEmitter]),
	});

export const CleaveRageThreshold = <SpecType extends WarriorSpecs>() =>
	InputHelpers.makeClassOptionsNumberInput<SpecType>({
		fieldName: 'cleaveRageThreshold',
		label: i18n.t('settings_tab.other.cleave_rage_threshold.label'),
		labelTooltip: i18n.t('settings_tab.other.cleave_rage_threshold.tooltip'),
		positive: true,
		getValue: player => cleaveRageThreshold(player),
		changeEmitter: player => TypedEvent.onAny([player.specOptionsChangeEmitter, player.talentsChangeEmitter]),
	});

export const BattleShoutSolarianSapphire = <SpecType extends WarriorSpecs>() =>
	InputHelpers.makeClassOptionsBooleanIconInput<SpecType>({
		fieldName: 'hasBsSolarianSapphire',
		label: i18n.t('settings_tab.other.has_bs_solarian_sapphire.label'),
		labelTooltip: i18n.t('settings_tab.other.has_bs_solarian_sapphire.tooltip'),
		actionId: () => ActionId.fromItemId(30446),
	});

export const BattleShoutT2 = <SpecType extends WarriorSpecs>() =>
	InputHelpers.makeClassOptionsBooleanIconInput<SpecType>({
		fieldName: 'hasBsT2',
		label: i18n.t('settings_tab.other.has_bs_tier_2.label'),
		labelTooltip: i18n.t('settings_tab.other.has_bs_tier_2.tooltip'),
		actionId: () => ActionId.fromSpellId(23563),
	});
