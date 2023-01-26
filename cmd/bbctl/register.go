package main

import (
	"fmt"

	"github.com/beeper/bridge-manager/beeperapi"
	"github.com/urfave/cli/v2"
)

func registerCommandFactory() *cli.Command {
	return &cli.Command{
		Name:      "register",
		Usage:     "Register a new bridge and print the appservice registration file",
		UsageText: "bbctl register BRIDGE [options]",
		Subcommands: []*cli.Command{
			{
				Name:   "imessage",
				Usage:  "Register mautrix-imessage",
				Action: registerImessage,
			},
		},
	}
}

func registerImessage(c *cli.Context) error {
	homeserver := c.String("homeserver")
	username := c.String("username")
	accessToken := c.String("token")

	whoami, err := beeperapi.Whoami(homeserver, accessToken)
	if err != nil {
		return fmt.Errorf("failed to get whoami: %w", err)
	}
	hungryAPI := NewHungryAPI(homeserver, whoami.UserInfo.BridgeClusterID, username, accessToken)
	req := ReqRegisterAppService{
		Push: false,
	}

	resp, err := hungryAPI.RegisterAppService("imessage", req)
	if err != nil {
		return fmt.Errorf("failed to register appservice: %w", err)
	}
	yaml, err := resp.YAML()
	if err != nil {
		return fmt.Errorf("failed to get yaml: %w", err)
	}
	fmt.Println(yaml)

	return nil
}
