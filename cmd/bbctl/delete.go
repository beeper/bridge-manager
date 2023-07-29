package main

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/beeper/bridge-manager/api/beeperapi"
)

var deleteCommand = &cli.Command{
	Name:      "delete",
	Aliases:   []string{"d"},
	Usage:     "Delete a bridge and all associated rooms on the Beeper servers",
	ArgsUsage: "BRIDGE",
	Action:    deleteBridge,
	Before:    RequiresAuth,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "Force delete the bridge, even if it's not self-hosted or doesn't seem to exist.",
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
		whoami, err := getCachedWhoami(ctx)
		if err != nil {
			return fmt.Errorf("failed to get whoami: %w", err)
		}
		SaveHungryURL(ctx, whoami.UserInfo.HungryURL)
		bridgeInfo, ok := whoami.User.Bridges[bridge]
		if !ok {
			return UserError{fmt.Sprintf("You don't have a %s bridge.", color.CyanString(bridge))}
		}
		if !bridgeInfo.BridgeState.IsSelfHosted {
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
