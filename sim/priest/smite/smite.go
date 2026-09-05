package smite

import (
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
	"github.com/wowsims/tbc/sim/priest"
)

func RegisterSmitePriest() {
	core.RegisterAgentFactory(
		proto.Player_SmitePriest{},
		proto.Spec_SpecSmitePriest,
		func(character *core.Character, options *proto.Player, _ *proto.Raid) core.Agent {
			return NewSmitePriest(character, options)
		},
		func(player *proto.Player, spec interface{}) {
			playerSpec, ok := spec.(*proto.Player_SmitePriest)
			if !ok {
				panic("Invalid spec value for Smite Priest!")
			}
			player.Spec = playerSpec
		},
	)
}

type SmitePriest struct {
	*priest.Priest
}

func NewSmitePriest(character *core.Character, options *proto.Player) *SmitePriest {
	classOptions := options.GetSmitePriest().GetOptions().GetClassOptions()
	selfBuffs := priest.SelfBuffs{
		UseShadowfiend: true,
		// Shadowform is a shadow talent and locks out the Holy school, which is the
		// entire kit here.
		PreShadowform: classOptions.GetPreShadowform(),
	}

	basePriest := priest.New(character, selfBuffs, options.TalentsString)
	basePriest.Latency = float64(basePriest.ChannelClipDelay.Milliseconds())

	return &SmitePriest{Priest: basePriest}
}

func (spriest *SmitePriest) GetPriest() *priest.Priest {
	return spriest.Priest
}

func (spriest *SmitePriest) Initialize() {
	spriest.Priest.Initialize()
	spriest.Priest.RegisterHolyFireSpells()
}

func (spriest *SmitePriest) ApplyTalents() {
	spriest.Priest.ApplyTalents()
}

func (spriest *SmitePriest) Reset(sim *core.Simulation) {
	spriest.Priest.Reset(sim)
}

func (spriest *SmitePriest) OnEncounterStart(sim *core.Simulation) {
	spriest.Priest.OnEncounterStart(sim)
}
