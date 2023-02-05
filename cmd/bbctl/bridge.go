package main

import (
	"encoding/json"
	"fmt"

	"github.com/urfave/cli/v2"
	"maunium.net/go/mautrix/bridge/status"

	"github.com/beeper/bridge-manager/beeperapi"
	"github.com/beeper/bridge-manager/hungryapi"
)

var bridgeCommand = &cli.Command{
	Name:  "bridge",
	Usage: "Manage your bridges",
	Before: func(ctx *cli.Context) error {
		if !GetEnvConfig(ctx).HasCredentials() {
			return UserError{"You're not logged in"}
		}
		return nil
	},
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
			},
		},
		{
			Name: "whoami",
			Action: func(ctx *cli.Context) error {
				api := GetAPIClient(ctx)

				whoami, err := api.Whoami()
				if err != nil {
					return fmt.Errorf("failed to get whoami: %w", err)
				}
				data, _ := json.MarshalIndent(whoami, "", "  ")
				fmt.Println(string(data))
				return nil
			},
		},
	},
}

func registerBridge(ctx *cli.Context) error {
	bridge := ctx.Args().Get(0)
	hungryAPI := GetHungryClient(ctx)
	beeperAPI := GetAPIClient(ctx)

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
	fmt.Println(yaml)
	err = beeperAPI.PostBridgeState(bridge, beeperapi.ReqPostBridgeState{
		StateEvent: status.StateStarting,
		Reason:     "SELF_HOST_REGISTERED",
		Info: map[string]any{
			"isHungry":   true,
			"isSelfHost": true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to mark bridge as STARTING: %w", err)
	}

	return nil
}
