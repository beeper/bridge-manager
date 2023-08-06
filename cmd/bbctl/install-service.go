package main

import (
	"os"
	"runtime"

	"github.com/urfave/cli/v2"
)

var installServiceCommand = &cli.Command{
	Name:      "install-service",
	Usage:     "Install a system service file to run an official Beeper bridge",
	ArgsUsage: "BRIDGE",
	Before:    RequiresAuth,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "type",
			Aliases: []string{"t"},
			EnvVars: []string{"BEEPER_BRIDGE_TYPE"},
			Usage:   "The type of bridge to run.",
		},
		&cli.StringSliceFlag{
			Name:    "param",
			Aliases: []string{"p"},
			Usage:   "Set a bridge-specific config generation option. Can be specified multiple times for different keys. Format: key=value",
		},
		&cli.BoolFlag{
			Name:    "no-update",
			Aliases: []string{"n"},
			Usage:   "Don't update the bridge even if it is out of date.",
			EnvVars: []string{"BEEPER_BRIDGE_NO_UPDATE"},
		},
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "Force register a bridge without the sh- prefix (dangerous).",
			Hidden:  true,
		},
	},
	Action: installService,
}

func isSystemd() bool {
	stat, err := os.Stat("/run/systemd/system")
	return err == nil && stat.IsDir()
}

func installService(ctx *cli.Context) error {
	if ctx.NArg() == 0 {
		return UserError{"You must specify a bridge to install"}
	} else if ctx.NArg() > 1 {
		return UserError{"Too many arguments specified (flags must come before arguments)"}
	}
	bridgeName := ctx.Args().Get(0)
	installed, err := doInstallBridge(ctx, bridgeName, true)
	if err != nil {
		return err
	}
	switch {
	case runtime.GOOS == "darwin":
	case isSystemd():
	default:
		return UserError{"No supported init systems found"}
	}
	return nil
}
