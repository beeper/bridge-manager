package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/urfave/cli/v2"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/bridge-manager/hungryapi"
)

type UserError struct {
	Message string
}

func (ue UserError) Error() string {
	return ue.Message
}

type contextKey int

const (
	contextKeyConfig contextKey = iota
	contextKeyEnvConfig
	contextKeyMatrixClient
	contextKeyHungryClient
)

func GetConfig(ctx *cli.Context) *Config {
	return ctx.Context.Value(contextKeyConfig).(*Config)
}

func GetEnvConfig(ctx *cli.Context) *EnvConfig {
	return ctx.Context.Value(contextKeyEnvConfig).(*EnvConfig)
}

func GetMatrixClient(ctx *cli.Context) *mautrix.Client {
	val := ctx.Context.Value(contextKeyMatrixClient)
	if val == nil {
		return nil
	}
	return val.(*mautrix.Client)
}

func GetHungryClient(ctx *cli.Context) *hungryapi.Client {
	val := ctx.Context.Value(contextKeyHungryClient)
	if val == nil {
		return nil
	}
	return val.(*hungryapi.Client)
}

var (
	Tag       string
	Commit    string
	BuildTime string

	ParsedBuildTime time.Time

	Version = "v0.1.0"
)

func init() {
	ParsedBuildTime, _ = time.Parse("Jan _2 2006, 15:04:05 MST", BuildTime)
	if Tag != Version {
		Version = fmt.Sprintf("%s+dev.%s", Version, Commit[:8])
	}
	mautrix.DefaultUserAgent = fmt.Sprintf("bbctl/%s %s", Version, mautrix.DefaultUserAgent)
}

func getDefaultConfigPath() string {
	baseConfigDir, err := os.UserConfigDir()
	if err != nil {
		panic(err)
	}
	return path.Join(baseConfigDir, "bbctl.json")
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
		homeserver := ctx.String("homeserver")
		ctx.Context = context.WithValue(ctx.Context, contextKeyMatrixClient, NewMatrixAPI(homeserver, envConfig.Username, envConfig.AccessToken))
		ctx.Context = context.WithValue(ctx.Context, contextKeyHungryClient, hungryapi.NewClient(homeserver, envConfig.ClusterID, envConfig.Username, envConfig.AccessToken))
	}
	return nil
}

var app = &cli.App{
	Name:     "bbctl",
	Usage:    "Manage self-hosted bridges for Beeper",
	Compiled: ParsedBuildTime,
	Version:  Version,
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
			Value:   getDefaultConfigPath(),
		},
	},
	Before: prepareApp,
	Commands: []*cli.Command{
		loginCommand,
		logoutCommand,
		bridgeCommand,
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
