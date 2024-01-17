package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/urfave/cli/v2"
	"maunium.net/go/mautrix"

	"github.com/beeper/bridge-manager/api/beeperapi"
	"github.com/beeper/bridge-manager/cli/interactive"
)

var loginCommand = &cli.Command{
	Name:    "login",
	Aliases: []string{"l"},
	Usage:   "Log into the Beeper server",
	Before:  interactive.Ask,
	Action:  beeperLogin,
	Flags: []cli.Flag{
		interactive.Flag{Flag: &cli.StringFlag{
			Name:    "email",
			EnvVars: []string{"BEEPER_EMAIL"},
			Usage:   "The Beeper account email to log in with",
		}, Survey: &survey.Input{
			Message: "Email:",
		}},
	},
}

func beeperLogin(ctx *cli.Context) error {
	homeserver := ctx.String("homeserver")
	email := ctx.String("email")

	startLogin, err := beeperapi.StartLogin(homeserver)
	if err != nil {
		return fmt.Errorf("failed to start login: %w", err)
	}
	err = beeperapi.SendLoginEmail(homeserver, startLogin.RequestID, email)
	if err != nil {
		return fmt.Errorf("failed to send login email: %w", err)
	}
	var apiResp *beeperapi.RespSendLoginCode
	for {
		var code string
		err = survey.AskOne(&survey.Input{
			Message: "Enter login code sent to your email:",
		}, &code)
		if err != nil {
			return err
		}
		apiResp, err = beeperapi.SendLoginCode(homeserver, startLogin.RequestID, code)
		if errors.Is(err, beeperapi.ErrInvalidLoginCode) {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
			continue
		} else if err != nil {
			return fmt.Errorf("failed to send login code: %w", err)
		}
		break
	}

	return doMatrixLogin(ctx, &mautrix.ReqLogin{
		Type:  "org.matrix.login.jwt",
		Token: apiResp.LoginToken,
	}, apiResp.Whoami)
}

func doMatrixLogin(ctx *cli.Context, req *mautrix.ReqLogin, whoami *beeperapi.RespWhoami) error {
	cfg := GetConfig(ctx)
	req.DeviceID = cfg.DeviceID
	req.InitialDeviceDisplayName = "github.com/beeper/bridge-manager"

	homeserver := ctx.String("homeserver")
	api := NewMatrixAPI(homeserver, "", "")
	resp, err := api.Login(ctx.Context, req)
	if err != nil {
		return fmt.Errorf("failed to log in: %w", err)
	}
	fmt.Printf("Successfully logged in as %s\n", resp.UserID)
	if whoami == nil {
		whoami, err = beeperapi.Whoami(homeserver, resp.AccessToken)
		if err != nil {
			_, _ = api.Logout(ctx.Context)
			return fmt.Errorf("failed to get user details: %w", err)
		}
	}
	envCfg := GetEnvConfig(ctx)
	envCfg.ClusterID = whoami.UserInfo.BridgeClusterID
	envCfg.Username = whoami.UserInfo.Username
	envCfg.HungryAddress = whoami.UserInfo.HungryURL
	envCfg.AccessToken = resp.AccessToken
	envCfg.BridgeDataDir = filepath.Join(UserDataDir, "bbctl", ctx.String("env"))
	err = cfg.Save()
	if err != nil {
		_, _ = api.Logout(ctx.Context)
		return fmt.Errorf("failed to save config: %w", err)
	}
	return nil
}
