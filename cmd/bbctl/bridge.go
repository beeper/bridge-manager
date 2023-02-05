package main

import (
	"fmt"
	"os"
	"regexp"

	"github.com/AlecAivazis/survey/v2"
	"github.com/urfave/cli/v2"
	"maunium.net/go/mautrix/bridge/status"

	"github.com/beeper/bridge-manager/beeperapi"
	"github.com/beeper/bridge-manager/hungryapi"
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
					Usage:   "Path to save generated registration file to.",
				},
			},
		},
		{
			Name:      "delete",
			Usage:     "Delete a bridge and all associated rooms on the Beeper servers",
			ArgsUsage: "BRIDGE",
			Action:    deleteBridge,
		},
	},
}

func deleteBridge(ctx *cli.Context) error {
	bridge := ctx.Args().Get(0)
	if bridge == "" {
		return UserError{"You must specify a bridge to delete"}
	} else if !allowedBridgeRegex.MatchString(bridge) {
		return UserError{"Invalid bridge name"}
	}
	var confirmation bool
	err := survey.AskOne(&survey.Confirm{Message: fmt.Sprintf("Are you sure you want to permanently delete %s?", bridge)}, &confirmation)
	if err != nil {
		return err
	} else if !confirmation {
		return fmt.Errorf("bridge delete cancelled")
	}
	err = beeperapi.DeleteBridge(ctx.String("homeserver"), bridge, GetEnvConfig(ctx).AccessToken)
	if err != nil {
		return fmt.Errorf("error deleting bridge: %w", err)
	}
	fmt.Println("Started deleting bridge")
	return nil
}

var allowedBridgeRegex = regexp.MustCompile("[a-z0-9]{1,32}")

func registerBridge(ctx *cli.Context) error {
	bridge := ctx.Args().Get(0)
	if bridge == "" {
		return UserError{"You must specify a bridge to register"}
	} else if !allowedBridgeRegex.MatchString(bridge) {
		return UserError{"Invalid bridge name. Names must consist of 1-32 lowercase ASCII letters and digits."}
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
	yaml, err := resp.YAML()
	if err != nil {
		return fmt.Errorf("failed to get yaml: %w", err)
	}
	output := ctx.String("output")
	if output == "-" {
		fmt.Println(yaml)
	} else {
		err = os.WriteFile(output, []byte(yaml), 0600)
		if err != nil {
			return fmt.Errorf("failed to write registration to %s: %w", output, err)
		}
	}
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
