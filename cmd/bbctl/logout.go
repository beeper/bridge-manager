package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var logoutCommand = &cli.Command{
	Name:   "logout",
	Usage:  "Log out from the Beeper server",
	Before: RequiresAuth,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			EnvVars: []string{"BEEPER_FORCE_LOGOUT"},
			Usage:   "Remove access token even if logout API call fails",
		},
	},
	Action: beeperLogout,
}

func beeperLogout(ctx *cli.Context) error {
	_, err := GetMatrixClient(ctx).Logout(ctx.Context)
	if err != nil && !ctx.Bool("force") {
		return fmt.Errorf("error logging out: %w", err)
	}
	cfg := GetConfig(ctx)
	delete(cfg.Environments, ctx.String("env"))
	err = cfg.Save()
	if err != nil {
		return fmt.Errorf("error saving config: %w", err)
	}
	fmt.Println("Logged out successfully")
	return nil
}
