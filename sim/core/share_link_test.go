package core

import (
	"strings"
	"testing"

	"github.com/wowsims/tbc/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

func testShareSettings() *proto.IndividualSimSettings {
	return &proto.IndividualSimSettings{
		Settings: &proto.SimSettings{Iterations: 3000},
		Player: &proto.Player{
			Name:          "Tester",
			Race:          proto.Race_RaceHuman,
			Class:         proto.Class_ClassPriest,
			TalentsString: "5051000130505002501-225051000320152-",
		},
		Encounter:     &proto.Encounter{Duration: 180},
		TargetDummies: 2,
	}
}

func TestShareLinkRoundTrip(t *testing.T) {
	settings := testShareSettings()

	link, err := EncodeShareLink("https://wowsims.com/tbc/priest/smite/", settings)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.HasPrefix(link, "https://wowsims.com/tbc/priest/smite/#") {
		t.Fatalf("link lost its page URL: %s", link)
	}

	decoded, err := DecodeIndividualShareLink(link)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !googleProto.Equal(settings, decoded) {
		t.Fatalf("round trip changed the settings:\nwant %v\ngot  %v", settings, decoded)
	}
}

// A link pasted through chat clients and spreadsheets loses its padding or picks up wrapping.
func TestShareLinkTolerantDecoding(t *testing.T) {
	settings := testShareSettings()
	link, err := EncodeShareLink("https://wowsims.com/tbc/priest/smite/", settings)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	_, fragment, _ := strings.Cut(link, "#")

	for name, mangled := range map[string]string{
		"unpadded": strings.TrimRight(fragment, "="),
		"wrapped":  fragment[:10] + "\n" + fragment[10:],
	} {
		t.Run(name, func(t *testing.T) {
			decoded := &proto.IndividualSimSettings{}
			if err := DecodeShareSettings(mangled, decoded); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !googleProto.Equal(settings, decoded) {
				t.Fatal("round trip changed the settings")
			}
		})
	}
}

// The path is the only thing distinguishing a raid link from an individual one.
func TestShareLinkRaidRouting(t *testing.T) {
	raidSettings := &proto.RaidSimSettings{Encounter: &proto.Encounter{Duration: 300}}

	link, err := EncodeShareLink("https://wowsims.com/tbc/raid/", raidSettings)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := DecodeShareLink(link)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := decoded.(*proto.RaidSimSettings); !ok {
		t.Fatalf("expected RaidSimSettings, got %T", decoded)
	}

	if _, err := DecodeIndividualShareLink(link); err == nil {
		t.Fatal("expected a raid link to be rejected as an individual link")
	}
}

func TestShareLinkRejectsMalformed(t *testing.T) {
	for name, link := range map[string]string{
		"no fragment":    "https://wowsims.com/tbc/priest/smite/",
		"empty fragment": "https://wowsims.com/tbc/priest/smite/#",
		"not base64":     "https://wowsims.com/tbc/priest/smite/#not-valid-base64!!",
		"not zlib":       "https://wowsims.com/tbc/priest/smite/#aGVsbG8gd29ybGQ=",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeShareLink(link); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
