package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wowsims/tbc/sim/core"
	"google.golang.org/protobuf/encoding/protojson"
	goproto "google.golang.org/protobuf/proto"
)

var decodeLinkCmd = &cobra.Command{
	Use:   "decodelink [link]",
	Short: "decode wowsims link/url",
	Long:  "decode wowsims link/url",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return decodeLink(args[0])
	},
}

func decodeLink(link string) error {
	settings, err := core.DecodeShareLink(link)
	if err != nil {
		return err
	}

	fmt.Println(protojson.Format(goproto.Message(settings)))
	return nil
}
