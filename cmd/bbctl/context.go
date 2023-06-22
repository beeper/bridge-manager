package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"maunium.net/go/mautrix"

	"github.com/beeper/bridge-manager/api/hungryapi"
)

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

func SaveHungryURL(ctx *cli.Context, newURL string) {
	env := GetEnvConfig(ctx)
	if env.HungryAddress != newURL && newURL != "" {
		env.HungryAddress = newURL
		err := GetConfig(ctx).Save()
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, color.RedString("Failed to save config: "+err.Error()))
		}
	}
}
