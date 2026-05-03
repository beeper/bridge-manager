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
)

var loginCommand = &cli.Command{
	Name:    "login",
	Aliases: []string{"l"},
	Usage:   "Log into the Beeper server",
	Action:  beeperLogin,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "email",
			EnvVars: []string{"BEEPER_EMAIL"},
			Usage:   "The Beeper account email to log in with",
		},
		&cli.BoolFlag{
			Name:    "no-desktop",
			EnvVars: []string{"BBCTL_NO_DESKTOP_LOGIN"},
			Usage:   "Skip checking for an existing Beeper Desktop login",
		},
	},
}

func init() {
	loginCommand.Flags = append(loginCommand.Flags, desktopLoginFlags()...)
}

func maybeUseDesktopLogin(ctx *cli.Context) (bool, error) {
	if ctx.Bool("no-desktop") {
		return false, nil
	}
	dbPath, err := getLoginDesktopAccountDBPath(ctx)
	if err != nil {
		return false, err
	}
	account, err := readDesktopAccount(ctx.Context, dbPath)
	if err != nil {
		if ctx.IsSet("desktop-data-dir") {
			return false, err
		}
		return false, nil
	}

	useDesktop := false
	err = survey.AskOne(&survey.Confirm{
		Message: fmt.Sprintf("Use Beeper Desktop login for %s?", account.UserID),
		Default: true,
	}, &useDesktop)
	if err != nil {
		return false, err
	}
	if !useDesktop {
		return false, nil
	}

	env, homeserver, err := configureDesktopLogin(ctx, account)
	if err != nil {
		return false, err
	}
	fmt.Printf("Using Beeper Desktop login for %s in bbctl env %q (%s)\n", account.UserID, env, homeserver)
	return true, nil
}

func beeperLogin(ctx *cli.Context) error {
	didLogin, err := maybeUseDesktopLogin(ctx)
	if err != nil {
		return err
	} else if didLogin {
		return nil
	}

	homeserver := ctx.String("homeserver")
	email := ctx.String("email")
	if email == "" {
		err = survey.AskOne(&survey.Input{
			Message: "Email:",
		}, &email)
		if err != nil {
			return err
		}
	}

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
	envCfg.AccessToken = resp.AccessToken
	envCfg.DesktopDataDir = ""
	envCfg.BridgeDataDir = filepath.Join(UserDataDir, "bbctl", ctx.String("env"))
	err = cfg.Save()
	if err != nil {
		_, _ = api.Logout(ctx.Context)
		return fmt.Errorf("failed to save config: %w", err)
	}
	return nil
}
