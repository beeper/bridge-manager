package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/bridge-manager/api/hungryapi"
	"github.com/beeper/bridge-manager/log"
)

type UserError struct {
	Message string
}

func (ue UserError) Error() string {
	return ue.Message
}

var (
	Tag       string
	Commit    string
	BuildTime string

	ParsedBuildTime time.Time

	Version = "v0.12.2"
)

const BuildTimeFormat = "Jan _2 2006, 15:04:05 MST"

func init() {
	var err error
	ParsedBuildTime, err = time.Parse(time.RFC3339, BuildTime)
	if BuildTime != "" && err != nil {
		panic(fmt.Errorf("program compiled with malformed build time: %w", err))
	}
	if Tag != Version {
		if Commit == "" {
			Version = fmt.Sprintf("%s+dev.unknown", Version)
		} else {
			Version = fmt.Sprintf("%s+dev.%s", Version, Commit[:8])
		}
	}
	if BuildTime != "" {
		app.Version = fmt.Sprintf("%s (built at %s)", Version, ParsedBuildTime.Format(BuildTimeFormat))
		app.Compiled = ParsedBuildTime
	} else {
		app.Version = Version
	}
	mautrix.DefaultUserAgent = fmt.Sprintf("bbctl/%s %s", Version, mautrix.DefaultUserAgent)
}

func getDefaultConfigPath() string {
	baseConfigDir, err := os.UserConfigDir()
	if err != nil {
		panic(err)
	}
	return path.Join(baseConfigDir, "bbctl", "config.json")
}

func prepareApp(ctx *cli.Context) error {
	cfg, err := loadConfig(ctx.String("config"))
	if err != nil {
		return err
	}
	env := ctx.String("env")
	homeserver, ok := envs[env]
	if !ok {
		return fmt.Errorf("invalid environment %q", env)
	} else if err = ctx.Set("homeserver", homeserver); err != nil {
		return err
	}
	envConfig := cfg.Environments.Get(env)
	ctx.Context = context.WithValue(ctx.Context, contextKeyConfig, cfg)
	ctx.Context = context.WithValue(ctx.Context, contextKeyEnvConfig, envConfig)
	if envConfig.HasCredentials() {
		if envConfig.HungryAddress == "" || envConfig.ClusterID == "" || envConfig.Username == "" || !strings.Contains(envConfig.HungryAddress, "/_hungryserv") {
			log.Printf("Fetching whoami to fill missing env config details")
			_, err = getCachedWhoami(ctx)
			if err != nil {
				return fmt.Errorf("failed to get whoami: %w", err)
			}
		}
		homeserver := ctx.String("homeserver")
		ctx.Context = context.WithValue(ctx.Context, contextKeyMatrixClient, NewMatrixAPI(homeserver, envConfig.Username, envConfig.AccessToken))
		ctx.Context = context.WithValue(ctx.Context, contextKeyHungryClient, hungryapi.NewClient(homeserver, envConfig.HungryAddress, envConfig.Username, envConfig.AccessToken))
	}
	return nil
}

var app = &cli.App{
	Name:  "bbctl",
	Usage: "Manage self-hosted bridges for Beeper",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:   "homeserver",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:    "env",
			Aliases: []string{"e"},
			EnvVars: []string{"BEEPER_ENV"},
			Value:   "prod",
			Usage:   "The Beeper environment to connect to",
		},
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			EnvVars: []string{"BBCTL_CONFIG"},
			Usage:   "Path to the config file where access tokens are saved",
			Value:   getDefaultConfigPath(),
		},
		&cli.StringFlag{
			Name:    "color",
			EnvVars: []string{"BBCTL_COLOR"},
			Usage:   "Enable or disable all colors and hyperlinks in output (valid values: always/never/auto)",
			Value:   "auto",
			Action: func(ctx *cli.Context, val string) error {
				switch val {
				case "never":
					color.NoColor = true
				case "always":
					color.NoColor = false
				case "auto":
					// The color package auto-detects by default
				default:
					return fmt.Errorf("invalid value for --color: %q", val)
				}
				return nil
			},
		},
	},
	Before: prepareApp,
	Commands: []*cli.Command{
		loginCommand,
		loginPasswordCommand,
		logoutCommand,
		registerCommand,
		deleteCommand,
		whoamiCommand,
		configCommand,
		runCommand,
		proxyCommand,
	},
}

func main() {
	if err := app.Run(os.Args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
	}
}

const MatrixURLTemplate = "https://matrix.%s"

func NewMatrixAPI(baseDomain string, username, accessToken string) *mautrix.Client {
	homeserverURL := fmt.Sprintf(MatrixURLTemplate, baseDomain)
	var userID id.UserID
	if username != "" {
		userID = id.NewUserID(username, baseDomain)
	}
	client, err := mautrix.NewClient(homeserverURL, userID, accessToken)
	if err != nil {
		panic(err)
	}
	return client
}

func RequiresAuth(ctx *cli.Context) error {
	if !GetEnvConfig(ctx).HasCredentials() {
		return UserError{"You're not logged in"}
	}
	return nil
}
