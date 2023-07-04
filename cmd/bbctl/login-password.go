package main

import (
	"github.com/AlecAivazis/survey/v2"
	"github.com/urfave/cli/v2"
	"maunium.net/go/mautrix"

	"github.com/beeper/bridge-manager/cli/interactive"
)

var loginPasswordCommand = &cli.Command{
	Name:    "login-password",
	Aliases: []string{"l"},
	Usage:   "Log into the Beeper server using username and password",
	Before:  interactive.Ask,
	Action:  beeperLoginPassword,
	Flags: []cli.Flag{
		interactive.Flag{Flag: &cli.StringFlag{
			Name:    "username",
			Aliases: []string{"u"},
			EnvVars: []string{"BEEPER_USERNAME"},
			Usage:   "The Beeper username to log in as",
		}, Survey: &survey.Input{
			Message: "Username:",
		}},
		interactive.Flag{Flag: &cli.StringFlag{
			Name:    "password",
			Aliases: []string{"p"},
			EnvVars: []string{"BEEPER_PASSWORD"},
			Usage:   "The Beeper password to log in with",
		}, Survey: &survey.Password{
			Message: "Password:",
		}},
	},
}

func beeperLoginPassword(ctx *cli.Context) error {
	return doMatrixLogin(ctx, &mautrix.ReqLogin{
		Type: mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{
			Type: mautrix.IdentifierTypeUser,
			User: ctx.String("username"),
		},
		Password: ctx.String("password"),
	}, nil)
}
