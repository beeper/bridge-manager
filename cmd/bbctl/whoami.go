package main

import (
	"encoding/json"
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/beeper/bridge-manager/beeperapi"
)

var whoamiCommand = &cli.Command{
	Name:    "whoami",
	Aliases: []string{"w"},
	Usage:   "Get info about yourself",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "raw",
			Aliases: []string{"r"},
			EnvVars: []string{"BEEPER_WHOAMI_RAW"},
			Usage:   "Get raw JSON output instead of pretty-printed bridge status",
		},
	},
	Action: whoamiFunction,
}

func whoamiFunction(ctx *cli.Context) error {
	whoami, err := beeperapi.Whoami(ctx.String("homeserver"), GetEnvConfig(ctx).AccessToken)
	if err != nil {
		return fmt.Errorf("failed to get whoami: %w", err)
	}
	if ctx.Bool("raw") {
		data, err := json.MarshalIndent(whoami, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	return nil
}
