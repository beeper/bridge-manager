package main

import (
	"fmt"
	"os"
	"regexp"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"maunium.net/go/mautrix/bridge/status"

	"github.com/beeper/bridge-manager/api/beeperapi"
	"github.com/beeper/bridge-manager/api/hungryapi"
)

var bridgeCommand = &cli.Command{
	Name:    "bridge",
	Aliases: []string{"b"},
	Usage:   "Manage your bridges",
	Before:  RequiresAuth,
	Subcommands: []*cli.Command{
		{
			Name:      "register",
			Usage:     "Register a new bridge and print the appservice registration file",
			ArgsUsage: "BRIDGE",
			Action:    registerBridge,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "address",
					Aliases: []string{"a"},
					EnvVars: []string{"BEEPER_BRIDGE_ADDRESS"},
					Usage:   "Optionally, a https address where the Beeper server can push events.\nWhen omitted, the server will expect the bridge to connect with a websocket to receive events.",
				},
				&cli.StringFlag{
					Name:    "output",
					Aliases: []string{"o"},
					Value:   "-",
					EnvVars: []string{"BEEPER_BRIDGE_REGISTRATION_FILE"},
					Usage:   "Path to save generated registration file to.",
				},
				&cli.BoolFlag{
					Name:    "force",
					Aliases: []string{"f"},
					Usage:   "Force register an official bridge, which is not currently supported.",
				},
			},
		},
		{
			Name:      "delete",
			Usage:     "Delete a bridge and all associated rooms on the Beeper servers",
			ArgsUsage: "BRIDGE",
			Action:    deleteBridge,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "force",
					Aliases: []string{"f"},
					Usage:   "Force delete the bridge, even if it's not self-hosted or doesn't seem to exist.",
				},
			},
		},
	},
}

func deleteBridge(ctx *cli.Context) error {
	if ctx.NArg() == 0 {
		return UserError{"You must specify a bridge to delete"}
	} else if ctx.NArg() > 1 {
		return UserError{"Too many arguments specified (flags must come before arguments)"}
	}
	bridge := ctx.Args().Get(0)
	if !allowedBridgeRegex.MatchString(bridge) {
		return UserError{"Invalid bridge name"}
	} else if bridge == "hungryserv" {
		return UserError{"You really shouldn't do that"}
	}
	homeserver := ctx.String("homeserver")
	accessToken := GetEnvConfig(ctx).AccessToken
	if !ctx.Bool("force") {
		whoami, err := beeperapi.Whoami(homeserver, accessToken)
		if err != nil {
			return fmt.Errorf("failed to get whoami: %w", err)
		}
		bridgeInfo, ok := whoami.User.Bridges[bridge]
		if !ok {
			return UserError{fmt.Sprintf("You don't have a %s bridge.", color.CyanString(bridge))}
		}
		selfHosted, _ := bridgeInfo.BridgeState.Info["isSelfHosted"].(bool)
		if !selfHosted {
			return UserError{fmt.Sprintf("Your %s bridge is not self-hosted.", color.CyanString(bridge))}
		}
	}

	var confirmation bool
	err := survey.AskOne(&survey.Confirm{Message: fmt.Sprintf("Are you sure you want to permanently delete %s?", bridge)}, &confirmation)
	if err != nil {
		return err
	} else if !confirmation {
		return fmt.Errorf("bridge delete cancelled")
	}
	err = beeperapi.DeleteBridge(homeserver, bridge, accessToken)
	if err != nil {
		return fmt.Errorf("error deleting bridge: %w", err)
	}
	fmt.Println("Started deleting bridge")
	return nil
}

var allowedBridgeRegex = regexp.MustCompile("[a-z0-9]{1,32}")
var officialBridges = map[string]struct{}{
	"discord":       {},
	"discordgo":     {},
	"facebook":      {},
	"googlechat":    {},
	"imessagecloud": {},
	"imessage":      {},
	"instagram":     {},
	"linkedin":      {},
	"signal":        {},
	"slack":         {},
	"slackgo":       {},
	"telegram":      {},
	"twitter":       {},
	"whatsapp":      {},
	"androidsms":    {},
}

func registerBridge(ctx *cli.Context) error {
	if ctx.NArg() == 0 {
		return UserError{"You must specify a bridge to register"}
	} else if ctx.NArg() > 1 {
		return UserError{"Too many arguments specified (flags must come before arguments)"}
	}
	bridge := ctx.Args().Get(0)
	if !allowedBridgeRegex.MatchString(bridge) {
		return UserError{"Invalid bridge name. Names must consist of 1-32 lowercase ASCII letters and digits."}
	}
	if _, isOfficial := officialBridges[bridge]; isOfficial {
		_, _ = fmt.Fprintf(os.Stderr, "%s is an official bridge name.\n", color.CyanString(bridge))
		if !ctx.Bool("force") {
			return UserError{"Self-hosting the official Beeper bridges is not currently supported, as it requires configuring the bridges in a specific way. You may still run official bridges using a different bridge name."}
		}
	}
	homeserver := ctx.String("homeserver")
	accessToken := GetEnvConfig(ctx).AccessToken
	whoami, err := beeperapi.Whoami(homeserver, accessToken)
	if err != nil {
		return fmt.Errorf("failed to get whoami: %w", err)
	}
	bridgeInfo, ok := whoami.User.Bridges[bridge]
	if ok {
		selfHosted, _ := bridgeInfo.BridgeState.Info["isSelfHosted"].(bool)
		if !selfHosted {
			return UserError{fmt.Sprintf("Your %s bridge is not self-hosted.", color.CyanString(bridge))}
		}
		_, _ = fmt.Fprintf(os.Stderr, "You already have a %s bridge, returning existing registration file\n\n", color.CyanString(bridge))
	}
	hungryAPI := GetHungryClient(ctx)

	req := hungryapi.ReqRegisterAppService{
		Push: false,
	}
	if addr := ctx.String("address"); addr != "" {
		req.Push = true
		req.Address = addr
	}

	resp, err := hungryAPI.RegisterAppService(bridge, req)
	if err != nil {
		return fmt.Errorf("failed to register appservice: %w", err)
	}
	resp.EphemeralEvents = true
	resp.SoruEphemeralEvents = true
	yaml, err := resp.YAML()
	if err != nil {
		return fmt.Errorf("failed to get yaml: %w", err)
	}
	output := ctx.String("output")
	if output == "-" {
		_, _ = fmt.Fprintln(os.Stderr, color.YellowString("Registration file:"))
		fmt.Print(yaml)
	} else {
		err = os.WriteFile(output, []byte(yaml), 0600)
		if err != nil {
			return fmt.Errorf("failed to write registration to %s: %w", output, err)
		}
		_, _ = fmt.Fprintln(os.Stderr, color.YellowString("Wrote registration file to"), color.CyanString(output))
	}
	_, _ = fmt.Fprintln(os.Stderr, color.YellowString("\nAdditional bridge configuration details:"))
	_, _ = fmt.Fprintf(os.Stderr, "* Homeserver domain: %s\n", color.CyanString("beeper.local"))
	_, _ = fmt.Fprintf(os.Stderr, "* Homeserver URL: %s\n", color.CyanString(hungryAPI.HomeserverURL.String()))
	_, _ = fmt.Fprintf(os.Stderr, "* Your user ID: %s\n", color.CyanString(hungryAPI.UserID.String()))
	err = beeperapi.PostBridgeState(ctx.String("homeserver"), GetEnvConfig(ctx).Username, bridge, resp.AppToken, beeperapi.ReqPostBridgeState{
		StateEvent: status.StateRunning,
		Reason:     "SELF_HOST_REGISTERED",
		Info: map[string]any{
			"isHungry":     true,
			"isSelfHosted": true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to mark bridge as STARTING: %w", err)
	}

	return nil
}
