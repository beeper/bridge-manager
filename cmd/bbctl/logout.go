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
	envCfg := GetEnvConfig(ctx)
	if !envCfg.UsesDesktopLogin() {
		_, err := GetMatrixClient(ctx).Logout(ctx.Context)
		if err != nil && !ctx.Bool("force") {
			return fmt.Errorf("error logging out: %w", err)
		}
	}
	cfg := GetConfig(ctx)
	delete(cfg.Environments, ctx.String("env"))
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("error saving config: %w", err)
	}
	if envCfg.UsesDesktopLogin() {
		fmt.Println("Logged out of bbctl successfully. Your Beeper Desktop session was not affected.")
		return nil
	}
	fmt.Println("Logged out successfully")
	return nil
}
