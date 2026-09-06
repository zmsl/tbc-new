package core

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/wowsims/tbc/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// A wowsims share link carries the entire sim setup in its URL fragment: the settings proto,
// deflated, then base64'd. The client writes them in ui/core/components/individual_sim_ui/
// exporters/individual_link_exporter.tsx and reads them back in the matching importer.
//
// Both halves of the codec already existed in this repo, in two places that could not share
// them: decoding in cmd/wowsimcli (which only prints), and encoding in sim/lib (which only
// exports to C). They live here so the CLI, the c-shared library and anything else speaking
// to the sim headlessly agree on one implementation.

// ErrInvalidShareLink is returned for anything that is not a wowsims URL with a settings
// fragment attached.
var ErrInvalidShareLink = errors.New("invalid wowsims share link")

// EncodeShareSettings compresses and encodes a settings proto into the fragment portion of a
// share link.
func EncodeShareSettings(settings googleProto.Message) (string, error) {
	data, err := googleProto.Marshal(settings)
	if err != nil {
		return "", fmt.Errorf("cannot marshal settings: %w", err)
	}

	var buffer bytes.Buffer
	writer := zlib.NewWriter(&buffer)
	if _, err := writer.Write(data); err != nil {
		return "", fmt.Errorf("cannot compress settings: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("cannot compress settings: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}

// DecodeShareSettings reads a share link fragment into settings, which must be the message
// type the fragment was written from.
func DecodeShareSettings(encoded string, settings googleProto.Message) error {
	raw, err := decodeShareBase64(encoded)
	if err != nil {
		return fmt.Errorf("cannot decode settings: %w", err)
	}

	reader, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("cannot decompress settings: %w", err)
	}
	defer reader.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		return fmt.Errorf("cannot decompress settings: %w", err)
	}

	if err := googleProto.Unmarshal(buf.Bytes(), settings); err != nil {
		return fmt.Errorf("cannot unmarshal settings: %w", err)
	}
	return nil
}

// Links get copied through chat clients and spreadsheets, which sometimes drop the '=' padding
// or wrap the text. Accept those rather than making the user work out why their link is
// "invalid"; anything genuinely malformed still fails at the zlib or proto step.
func decodeShareBase64(encoded string) ([]byte, error) {
	cleaned := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, encoded)

	if len(cleaned)%4 == 0 {
		return base64.StdEncoding.DecodeString(cleaned)
	}
	return base64.RawStdEncoding.DecodeString(cleaned)
}

// EncodeShareLink writes settings into the fragment of pageURL, replacing any fragment already
// there. pageURL is the sim page the link should open, e.g.
// "https://wowsims.com/tbc/priest/smite/".
func EncodeShareLink(pageURL string, settings googleProto.Message) (string, error) {
	encoded, err := EncodeShareSettings(settings)
	if err != nil {
		return "", err
	}

	base, _, _ := strings.Cut(pageURL, "#")
	if base == "" {
		return "", ErrInvalidShareLink
	}
	return base + "#" + encoded, nil
}

// DecodeShareLink reads a full share URL. Raid links carry RaidSimSettings and individual links
// carry IndividualSimSettings; the path is the only thing that says which, matching how the
// client routes them.
func DecodeShareLink(link string) (googleProto.Message, error) {
	path, fragment, found := strings.Cut(link, "#")
	if !found || fragment == "" {
		return nil, ErrInvalidShareLink
	}

	var settings googleProto.Message
	if strings.Contains(path, "/raid/") {
		settings = &proto.RaidSimSettings{}
	} else {
		settings = &proto.IndividualSimSettings{}
	}

	if err := DecodeShareSettings(fragment, settings); err != nil {
		return nil, err
	}
	return settings, nil
}

// DecodeIndividualShareLink is DecodeShareLink for the single-player case, which is the only
// one the sim tools support. A raid link is rejected rather than silently mis-parsed.
func DecodeIndividualShareLink(link string) (*proto.IndividualSimSettings, error) {
	settings, err := DecodeShareLink(link)
	if err != nil {
		return nil, err
	}

	individual, ok := settings.(*proto.IndividualSimSettings)
	if !ok {
		return nil, fmt.Errorf("%w: this is a raid link, not an individual sim link", ErrInvalidShareLink)
	}
	return individual, nil
}
