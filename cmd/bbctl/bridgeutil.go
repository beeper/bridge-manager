package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/beeper/bridge-manager/bridgeconfig"
)

var allowedBridgeRegex = regexp.MustCompile("^[a-z0-9-]{1,32}$")

type bridgeTypeToNames struct {
	typeName string
	names    []string
}

var officialBridges = []bridgeTypeToNames{
	{"discord", []string{"discord"}},
	{"meta", []string{"meta", "instagram", "facebook"}},
	{"googlechat", []string{"googlechat", "gchat"}},
	{"imessagego", []string{"imessagego"}},
	{"imessage", []string{"imessage"}},
	{"linkedin", []string{"linkedin"}},
	{"signal", []string{"signal"}},
	{"slack", []string{"slack"}},
	{"telegram", []string{"telegram"}},
	{"twitter", []string{"twitter"}},
	{"whatsapp", []string{"whatsapp"}},
	{"heisenbridge", []string{"irc", "heisenbridge"}},
	{"gmessages", []string{"gmessages", "googlemessages", "rcs", "sms"}},
	{"gvoice", []string{"gvoice", "googlevoice"}},
	{"bluesky", []string{"bluesky", "bsky"}},
}

var websocketBridges = map[string]bool{
	"discord":      true,
	"slack":        true,
	"whatsapp":     true,
	"gmessages":    true,
	"gvoice":       true,
	"heisenbridge": true,
	"imessage":     true,
	"imessagego":   true,
	"signal":       true,
	"bridgev2":     true,
	"meta":         true,
	"twitter":      true,
	"bluesky":      true,
	"linkedin":     true,
}

func doOutputFile(ctx *cli.Context, name, data string) error {
	outputPath := ctx.String("output")
	if outputPath == "-" {
		_, _ = fmt.Fprintln(os.Stderr, color.YellowString(name+" file:"))
		fmt.Println(strings.TrimRight(data, "\n"))
	} else {
		err := os.WriteFile(outputPath, []byte(data), 0600)
		if err != nil {
			return fmt.Errorf("failed to write %s to %s: %w", strings.ToLower(name), outputPath, err)
		}
		_, _ = fmt.Fprintln(os.Stderr, color.YellowString("Wrote "+strings.ToLower(name)+" file to"), color.CyanString(outputPath))
	}
	return nil
}

func validateBridgeName(ctx *cli.Context, bridge string) error {
	if !allowedBridgeRegex.MatchString(bridge) {
		return UserError{"Invalid bridge name. Names must consist of 1-32 lowercase ASCII letters, digits and -."}
	}
	if !strings.HasPrefix(bridge, "sh-") {
		if !ctx.Bool("force") {
			return UserError{"Self-hosted bridge names should start with sh-"}
		}
		_, _ = fmt.Fprintln(os.Stderr, "Self-hosted bridge names should start with sh-")
	}
	return nil
}

func guessOrAskBridgeType(bridge, bridgeType string) (string, error) {
	if bridgeType == "" {
	Outer:
		for _, br := range officialBridges {
			for _, name := range br.names {
				if strings.Contains(bridge, name) {
					bridgeType = br.typeName
					break Outer
				}
			}
		}
	}
	if !bridgeconfig.IsSupported(bridgeType) {
		_, _ = fmt.Fprintln(os.Stderr, color.YellowString("Unsupported bridge type"), color.CyanString(bridgeType))
		err := survey.AskOne(&survey.Select{
			Message: "Select bridge type:",
			Options: bridgeconfig.SupportedBridges,
		}, &bridgeType)
		if err != nil {
			return "", err
		}
	}
	return bridgeType, nil
}
