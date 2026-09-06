package shadow

import (
	"github.com/wowsims/tbc/sim/core"
	"github.com/wowsims/tbc/sim/core/proto"
	"github.com/wowsims/tbc/sim/priest"
)

func RegisterShadowPriest() {
	core.RegisterAgentFactory(
		proto.Player_Priest{},
		proto.Spec_SpecPriest,
		func(character *core.Character, options *proto.Player, _ *proto.Raid) core.Agent {
			return NewShadowPriest(character, options)
		},
		func(player *proto.Player, spec interface{}) {
			playerSpec, ok := spec.(*proto.Player_Priest)
			if !ok {
				panic("Invalid spec value for Shadow Priest!")
			}
			player.Spec = playerSpec
		},
	)
}

type ShadowPriest struct {
	*priest.Priest
}

func NewShadowPriest(character *core.Character, options *proto.Player) *ShadowPriest {
	classOptions := options.GetPriest().GetOptions().GetClassOptions()
	selfBuffs := priest.SelfBuffs{
		UseShadowfiend: true,
		PreShadowform:  classOptions.GetPreShadowform(),
	}

	basePriest := priest.New(character, selfBuffs, options.TalentsString)
	basePriest.Latency = float64(basePriest.ChannelClipDelay.Milliseconds())

	return &ShadowPriest{Priest: basePriest}
}

func (spriest *ShadowPriest) GetPriest() *priest.Priest {
	return spriest.Priest
}

func (spriest *ShadowPriest) Initialize() {
	spriest.Priest.Initialize()
}

func (spriest *ShadowPriest) ApplyTalents() {
	spriest.Priest.ApplyTalents()
}

func (spriest *ShadowPriest) Reset(sim *core.Simulation) {
	spriest.Priest.Reset(sim)
}

func (spriest *ShadowPriest) OnEncounterStart(sim *core.Simulation) {
	spriest.Priest.OnEncounterStart(sim)
}
