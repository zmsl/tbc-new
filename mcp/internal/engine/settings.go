package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// PresetKind is one of the three flavours of checked-in preset.
type PresetKind string

const (
	PresetGear     PresetKind = "gear"
	PresetRotation PresetKind = "rotation"
	PresetBuild    PresetKind = "build"
)

func (k PresetKind) paths() (dir, suffix string, err error) {
	switch k {
	case PresetGear:
		return "gear_sets", ".gear.json", nil
	case PresetRotation:
		return "apls", ".apl.json", nil
	case PresetBuild:
		return "builds", ".build.json", nil
	}
	return "", "", fmt.Errorf("unknown preset kind %q", k)
}

// ReadPreset returns a preset file's raw bytes. The engine's own loaders (core.GetGearSet and
// friends) call log.Fatal on a missing file, which is fine for a test binary and fatal for a
// server, so presets are read here instead.
func (c Config) ReadPreset(spec proto.Spec, kind PresetKind, name string) ([]byte, error) {
	specDir, ok := SpecDir(spec)
	if !ok {
		return nil, fmt.Errorf("no preset directory known for %v", spec)
	}
	dir, suffix, err := kind.paths()
	if err != nil {
		return nil, err
	}
	base := filepath.Join(c.PresetsRoot, specDir, dir)
	path := filepath.Join(base, name+suffix)
	// Names may be nested -- the hunter keeps its gear sets in per-phase directories -- but must
	// stay inside the preset directory.
	if !strings.HasPrefix(path, base+string(filepath.Separator)) {
		return nil, fmt.Errorf("preset name %q escapes the preset directory", name)
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("no %s preset named %q for %v", kind, name, specName(spec))
	}
	return data, err
}

// LoadGearSet reads a checked-in gear set.
func (c Config) LoadGearSet(spec proto.Spec, name string) (*proto.EquipmentSpec, error) {
	data, err := c.ReadPreset(spec, PresetGear, name)
	if err != nil {
		return nil, err
	}
	equipment := &proto.EquipmentSpec{}
	if err := protojson.Unmarshal(data, equipment); err != nil {
		return nil, fmt.Errorf("gear set %q: %w", name, err)
	}
	return equipment, nil
}

// LoadRotation reads a checked-in APL rotation.
func (c Config) LoadRotation(spec proto.Spec, name string) (*proto.APLRotation, error) {
	data, err := c.ReadPreset(spec, PresetRotation, name)
	if err != nil {
		return nil, err
	}
	rotation := &proto.APLRotation{}
	if err := protojson.Unmarshal(data, rotation); err != nil {
		return nil, fmt.Errorf("rotation %q: %w", name, err)
	}
	return rotation, nil
}

// LoadBuild reads a checked-in build: gear, talents, rotation and encounter together, already in
// the shape a share link carries.
func (c Config) LoadBuild(spec proto.Spec, name string) (*proto.IndividualSimSettings, error) {
	data, err := c.ReadPreset(spec, PresetBuild, name)
	if err != nil {
		return nil, err
	}
	settings := &proto.IndividualSimSettings{}
	if err := protojson.Unmarshal(data, settings); err != nil {
		return nil, fmt.Errorf("build %q: %w", name, err)
	}
	return settings, nil
}

// TalentPreset is a named talent build.
type TalentPreset struct {
	Name    string `json:"name" jsonschema:"the preset's name, e.g. Smite"`
	Talents string `json:"talents" jsonschema:"the talent string, e.g. 5051000130505002501-225051000320152-"`
}

// Talent strings live in the client's presets.ts and nowhere else machine-readable, so they are
// scraped out of it. That is deliberate rather than duplicated: a copy in this package would
// silently rot the first time a spec retunes its build, whereas a scrape either finds the
// current string or reports that it found nothing.
var (
	talentsStringPattern = regexp.MustCompile(`talentsString:\s*'([^']*)'`)
	talentsNamePattern   = regexp.MustCompile(`(?:name:\s*'([^']*)'|makePresetTalents\(\s*'([^']*)')`)
)

// ListTalents returns the talent builds a spec's presets declare, in file order.
func (c Config) ListTalents(spec proto.Spec) ([]TalentPreset, error) {
	specDir, ok := SpecDir(spec)
	if !ok {
		return nil, fmt.Errorf("no preset directory known for %v", spec)
	}

	data, err := os.ReadFile(filepath.Join(c.PresetsRoot, specDir, "presets.ts"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	source := string(data)

	var presets []TalentPreset
	for _, match := range talentsStringPattern.FindAllStringSubmatchIndex(source, -1) {
		talents := source[match[2]:match[3]]
		if talents == "" {
			continue
		}
		presets = append(presets, TalentPreset{
			Name:    nearestPrecedingName(source[:match[0]]),
			Talents: talents,
		})
	}
	return presets, nil
}

// The name belongs to whichever preset declaration most recently opened before the talent
// string, which is the last name-like match in the text preceding it.
func nearestPrecedingName(preceding string) string {
	matches := talentsNamePattern.FindAllStringSubmatch(preceding, -1)
	if len(matches) == 0 {
		return "unnamed"
	}
	last := matches[len(matches)-1]
	if last[1] != "" {
		return last[1]
	}
	return last[2]
}

// NewPlayer builds a player for a spec with its options message present and empty, which is what
// the engine expects: a nil options message panics the spec's constructor.
//
// The spec's field in Player's `spec` oneof is found by matching the message type name --
// Spec_SpecSmitePriest against the SmitePriest message -- rather than by another hand-written
// table, so a spec added to the proto needs nothing here.
func NewPlayer(spec proto.Spec) (*proto.Player, error) {
	wanted := specName(spec)

	player := &proto.Player{Class: specClasses[spec], Race: defaultRaces[specClasses[spec]]}
	message := player.ProtoReflect()
	oneof := message.Descriptor().Oneofs().ByName("spec")
	if oneof == nil {
		return nil, fmt.Errorf("Player has no spec oneof")
	}

	for i := range oneof.Fields().Len() {
		field := oneof.Fields().Get(i)
		if string(field.Message().Name()) != wanted {
			continue
		}
		options := message.NewField(field)
		fillMessageDefaults(options.Message(), map[protoreflect.FullName]bool{})
		message.Set(field, options)
		return player, nil
	}

	return nil, fmt.Errorf("no player options for %v", spec)
}

// Instantiates every singular message field, recursively. Spec options nest a few levels
// (SmitePriest -> Options -> ClassOptions) and the engine dereferences them without checking.
//
// seen holds the message types currently being filled. Some of these reference a type that
// reaches back to itself, and without the guard filling one descends forever.
func fillMessageDefaults(message protoreflect.Message, seen map[protoreflect.FullName]bool) {
	name := message.Descriptor().FullName()
	if seen[name] {
		return
	}
	seen[name] = true
	defer delete(seen, name)

	fields := message.Descriptor().Fields()
	for i := range fields.Len() {
		field := fields.Get(i)
		if field.Kind() != protoreflect.MessageKind || field.IsList() || field.IsMap() {
			continue
		}
		child := message.NewField(field)
		fillMessageDefaults(child.Message(), seen)
		message.Set(field, child)
	}
}

// Encounters, by the names the golden suite gives them. Using the same three keeps a headless
// result comparable with a checked-in one.
func Encounter(name string) (*proto.Encounter, error) {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(name))
	for _, combo := range core.MakeDefaultEncounterCombos() {
		if strings.ToLower(combo.Label) == normalized {
			return combo.Encounter, nil
		}
	}
	return nil, fmt.Errorf("unknown encounter %q (want ShortSingleTarget, LongSingleTarget or LongMultiTarget)", name)
}

// EncounterNames lists what Encounter accepts.
func EncounterNames() []string {
	combos := core.MakeDefaultEncounterCombos()
	names := make([]string, 0, len(combos))
	for _, combo := range combos {
		names = append(names, combo.Label)
	}
	return names
}

// SettingsRequest describes a setup to assemble from checked-in presets.
type SettingsRequest struct {
	Spec      proto.Spec
	Race      proto.Race
	Build     string
	GearSet   string
	Rotation  string
	Talents   string
	Encounter string
	NoBuffs   bool
}

// BuildSettings assembles settings from presets. A build preset supplies everything at once;
// otherwise a gear set and rotation are combined with the spec's default talents. The returned
// notes say which presets were actually used, since several inputs have defaults.
func (c Config) BuildSettings(request SettingsRequest) (*proto.IndividualSimSettings, []string, error) {
	var notes []string

	if request.Build != "" {
		settings, err := c.LoadBuild(request.Spec, request.Build)
		if err != nil {
			return nil, nil, err
		}
		notes = append(notes, "from build preset "+request.Build)
		if err := c.applyOverrides(settings, request, &notes); err != nil {
			return nil, nil, err
		}
		return settings, notes, nil
	}

	player, err := NewPlayer(request.Spec)
	if err != nil {
		return nil, nil, err
	}
	player.ReactionTimeMs = 100
	player.ChannelClipDelayMs = 50
	player.Buffs = core.FullIndividualBuffs
	player.Profession1 = proto.Profession_Engineering

	settings := &proto.IndividualSimSettings{
		Settings:   &proto.SimSettings{},
		Player:     player,
		RaidBuffs:  core.FullRaidBuffs,
		PartyBuffs: core.FullPartyBuffs,
		Debuffs:    core.FullDebuffs,
	}

	presets, err := c.ListPresets(request.Spec)
	if err != nil {
		return nil, nil, err
	}

	gearSet := request.GearSet
	if gearSet == "" {
		if gearSet = pickGearSet(presets.GearSets); gearSet == "" {
			return nil, nil, fmt.Errorf("%v has no checked-in gear sets; supply a share link instead", specName(request.Spec))
		}
		notes = append(notes, "gear set defaulted to "+gearSet)
	}
	if settings.Player.Equipment, err = c.LoadGearSet(request.Spec, gearSet); err != nil {
		return nil, nil, err
	}

	rotation := request.Rotation
	if rotation == "" {
		if rotation = pickRotation(presets.Rotations); rotation == "" {
			return nil, nil, fmt.Errorf("%v has no checked-in rotations; supply a share link instead", specName(request.Spec))
		}
		notes = append(notes, "rotation defaulted to "+rotation)
	}
	if settings.Player.Rotation, err = c.LoadRotation(request.Spec, rotation); err != nil {
		return nil, nil, err
	}

	if err := c.applyTalents(settings, request, &notes); err != nil {
		return nil, nil, err
	}
	if err := c.applyOverrides(settings, request, &notes); err != nil {
		return nil, nil, err
	}

	return settings, notes, nil
}

func (c Config) applyTalents(settings *proto.IndividualSimSettings, request SettingsRequest, notes *[]string) error {
	if request.Talents != "" {
		settings.Player.TalentsString = request.Talents
		return nil
	}

	presets, err := c.ListTalents(request.Spec)
	if err != nil {
		return err
	}
	if len(presets) == 0 {
		return fmt.Errorf("no default talents found for %v; pass talents explicitly", specName(request.Spec))
	}
	settings.Player.TalentsString = presets[0].Talents
	*notes = append(*notes, "talents defaulted to the "+presets[0].Name+" preset")
	return nil
}

func (c Config) applyOverrides(settings *proto.IndividualSimSettings, request SettingsRequest, notes *[]string) error {
	if request.Race != proto.Race_RaceUnknown {
		settings.Player.Race = request.Race
	}

	if request.Encounter != "" {
		encounter, err := Encounter(request.Encounter)
		if err != nil {
			return err
		}
		settings.Encounter = encounter
	}
	if settings.Encounter == nil {
		settings.Encounter, _ = Encounter("LongSingleTarget")
		*notes = append(*notes, "encounter defaulted to LongSingleTarget")
	}

	if request.NoBuffs {
		settings.RaidBuffs = &proto.RaidBuffs{}
		settings.PartyBuffs = &proto.PartyBuffs{}
		settings.Debuffs = &proto.Debuffs{}
		settings.Player.Buffs = &proto.IndividualBuffs{}
	}

	return nil
}

func specName(spec proto.Spec) string {
	name := spec.String()
	return strings.TrimPrefix(name, "Spec")
}

// A class cannot be every race, and the sim wants one chosen. These are only defaults; every
// tool that assembles settings takes a race.
var defaultRaces = map[proto.Class]proto.Race{
	proto.Class_ClassDruid:   proto.Race_RaceNightElf,
	proto.Class_ClassHunter:  proto.Race_RaceOrc,
	proto.Class_ClassMage:    proto.Race_RaceGnome,
	proto.Class_ClassPaladin: proto.Race_RaceHuman,
	proto.Class_ClassPriest:  proto.Race_RaceHuman,
	proto.Class_ClassRogue:   proto.Race_RaceHuman,
	proto.Class_ClassShaman:  proto.Race_RaceOrc,
	proto.Class_ClassWarlock: proto.Race_RaceOrc,
	proto.Class_ClassWarrior: proto.Race_RaceOrc,
}

// Gear sets are conventionally named by raid phase, so the best default is the highest phase
// rather than the last name alphabetically -- which would pick "pre_raid" over "p5".
var phaseGearSet = regexp.MustCompile(`^p(\d+)$`)

func pickGearSet(names []string) string {
	best, bestPhase := "", -1
	for _, name := range names {
		if match := phaseGearSet.FindStringSubmatch(name); match != nil {
			if phase, err := strconv.Atoi(match[1]); err == nil && phase > bestPhase {
				best, bestPhase = name, phase
			}
		}
	}
	if best != "" {
		return best
	}
	if len(names) > 0 {
		return names[0]
	}
	return ""
}

// Most specs call their main rotation "default"; the ones that do not (mage, rogue, warlock and
// dps warrior) name them per build, and any of those runs.
func pickRotation(names []string) string {
	for _, name := range names {
		if name == "default" {
			return name
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return ""
}
