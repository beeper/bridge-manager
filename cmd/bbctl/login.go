package main

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/urfave/cli/v2"
	"maunium.net/go/mautrix"

	"github.com/beeper/bridge-manager/beeperapi"
	"github.com/beeper/bridge-manager/interactive"
)

var loginCommand = &cli.Command{
	Name:   "login",
	Usage:  "Log into the Beeper server",
	Before: interactive.Ask,
	Action: beeperLogin,
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

func beeperLogin(ctx *cli.Context) error {
	homeserver := ctx.String("homeserver")
	username := ctx.String("username")
	password := ctx.String("password")
	cfg := GetConfig(ctx)

	api := NewMatrixAPI(homeserver, "", "")
	resp, err := api.Login(&mautrix.ReqLogin{
		Type: mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{
			Type: mautrix.IdentifierTypeUser,
			User: username,
		},
		Password:                 password,
		DeviceID:                 cfg.DeviceID,
		InitialDeviceDisplayName: "github.com/beeper/bridge-manager",
	})
	if err != nil {
		return fmt.Errorf("failed to log in: %w", err)
	}
	fmt.Printf("Successfully logged in as %s\n", resp.UserID)
	username, homeserver, _ = resp.UserID.Parse()
	beeperAPI := beeperapi.NewClient(homeserver, username, resp.AccessToken)
	whoami, err := beeperAPI.Whoami()
	if err != nil {
		_, _ = api.Logout()
		return fmt.Errorf("failed to get user details: %w", err)
	}
	if !whoami.UserInfo.UseHungryserv {
		_, _ = api.Logout()
		return UserError{"This tool is only available on the new infra (hungryserv)\nPlease upgrade from the Beeper desktop app first"}
	}
	fmt.Printf("Found own bridge cluster ID: %s\n", whoami.UserInfo.BridgeClusterID)
	envCfg := GetEnvConfig(ctx)
	envCfg.ClusterID = whoami.UserInfo.BridgeClusterID
	envCfg.Username = whoami.UserInfo.Username
	envCfg.AccessToken = resp.AccessToken
	err = cfg.Save()
	if err != nil {
		_, _ = api.Logout()
		return fmt.Errorf("failed to save config: %w", err)
	}
	return nil
}
