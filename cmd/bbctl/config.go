package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/maps"

	"github.com/beeper/bridge-manager/bridgeconfig"
	"github.com/beeper/bridge-manager/cli/hyper"
)

var configCommand = &cli.Command{
	Name:      "config",
	Aliases:   []string{"c"},
	Usage:     "Generate a config for an official Beeper bridge",
	ArgsUsage: "BRIDGE",
	Before:    RequiresAuth,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "type",
			Aliases: []string{"t"},
			EnvVars: []string{"BEEPER_BRIDGE_TYPE"},
			Usage:   "The type of bridge being registered.",
		},
		&cli.StringSliceFlag{
			Name:    "param",
			Aliases: []string{"p"},
			Usage:   "Set a bridge-specific config generation option. Can be specified multiple times for different keys. Format: key=value",
		},
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Value:   "-",
			EnvVars: []string{"BEEPER_BRIDGE_CONFIG_FILE"},
			Usage:   "Path to save generated config file to.",
		},
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "Force register a bridge without the sh- prefix (dangerous).",
			Hidden:  true,
		},
	},
	Action: generateBridgeConfig,
}

func simpleDescriptions(descs map[string]string) func(string, int) string {
	return func(s string, i int) string {
		return descs[s]
	}
}

var askParams = map[string]func(map[string]string) error{
	"imessage": func(extraParams map[string]string) error {
		platform := extraParams["imessage_platform"]
		barcelonaPath := extraParams["barcelona_path"]
		if platform == "" {
			err := survey.AskOne(&survey.Select{
				Message: "Select iMessage connector:",
				Options: []string{"mac", "mac-nosip"},
				Description: simpleDescriptions(map[string]string{
					"mac":       "Use AppleScript to send messages and read chat.db for incoming data - only requires Full Disk Access (from system settings)",
					"mac-nosip": "Use Barcelona to interact with private APIs - requires disabling SIP and AMFI",
				}),
				Default: "mac",
			}, &platform)
			if err != nil {
				return err
			}
			extraParams["imessage_platform"] = platform
		}
		if platform == "mac-nosip" && barcelonaPath == "" {
			err := survey.AskOne(&survey.Input{
				Message: "Enter Barcelona executable path:",
				Default: "darwin-barcelona-mautrix",
			}, &barcelonaPath)
			if err != nil {
				return err
			}
			extraParams["barcelona_path"] = barcelonaPath
		}
		return nil
	},
}

type generatedBridgeConfig struct {
	BridgeType string
	Config     string
	*RegisterJSON
}

func doGenerateBridgeConfig(ctx *cli.Context, bridge string) (*generatedBridgeConfig, error) {
	if err := validateBridgeName(ctx, bridge); err != nil {
		return nil, err
	}

	whoami, err := getCachedWhoami(ctx)
	if err != nil {
		return nil, err
	}
	existingBridge, ok := whoami.User.Bridges[bridge]
	var bridgeType string
	if ok && existingBridge.BridgeState.BridgeType != "" {
		bridgeType = existingBridge.BridgeState.BridgeType
	} else {
		bridgeType, err = guessOrAskBridgeType(bridge, ctx.String("type"))
		if err != nil {
			return nil, err
		}
	}
	extraParamAsker := askParams[bridgeType]
	extraParams := make(map[string]string)
	for _, item := range ctx.StringSlice("param") {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			return nil, UserError{fmt.Sprintf("Invalid param %q", item)}
		}
		extraParams[strings.ToLower(parts[0])] = parts[1]
	}
	cliParams := maps.Clone(extraParams)
	if extraParamAsker != nil {
		err = extraParamAsker(extraParams)
		if err != nil {
			return nil, err
		}
		if len(extraParams) != len(cliParams) {
			formattedParams := make([]string, 0, len(extraParams))
			for key, value := range extraParams {
				_, isCli := cliParams[key]
				if !isCli {
					formattedParams = append(formattedParams, fmt.Sprintf("--param '%s=%s'", key, value))
				}
			}
			_, _ = fmt.Fprintf(os.Stderr, color.YellowString("To run without specifying parameters interactively, add `%s` next time\n"), strings.Join(formattedParams, " "))
		}
	}
	reg, err := doRegisterBridge(ctx, bridge, bridgeType, false)
	if err != nil {
		return nil, err
	}

	cfg, err := bridgeconfig.Generate(bridgeType, bridgeconfig.Params{
		HungryAddress: reg.HomeserverURL,
		BeeperDomain:  ctx.String("homeserver"),
		Websocket:     true,
		AppserviceID:  reg.Registration.ID,
		ASToken:       reg.Registration.AppToken,
		HSToken:       reg.Registration.ServerToken,
		BridgeName:    bridge,
		UserID:        reg.YourUserID,
		Params:        extraParams,
	})
	return &generatedBridgeConfig{
		BridgeType:   bridgeType,
		Config:       cfg,
		RegisterJSON: reg,
	}, err
}

func generateBridgeConfig(ctx *cli.Context) error {
	if ctx.NArg() == 0 {
		return UserError{"You must specify a bridge to generate a config for"}
	} else if ctx.NArg() > 1 {
		return UserError{"Too many arguments specified (flags must come before arguments)"}
	}
	bridge := ctx.Args().Get(0)
	cfg, err := doGenerateBridgeConfig(ctx, bridge)
	if err != nil {
		return err
	}

	err = doOutputFile(ctx, "Config", cfg.Config)
	if err != nil {
		return err
	}
	outputPath := ctx.String("output")
	if outputPath == "-" || outputPath == "" {
		outputPath = "<config file>"
	}
	var startupCommand, installInstructions string
	switch cfg.BridgeType {
	case "imessage", "whatsapp", "discord", "slack", "gmessages":
		startupCommand = fmt.Sprintf("mautrix-%s", cfg.BridgeType)
		if outputPath != "config.yaml" && outputPath != "<config file>" {
			startupCommand += " -c " + outputPath
		}
		installInstructions = fmt.Sprintf("https://docs.mau.fi/bridges/go/setup.html?bridge=%s#installation", cfg.BridgeType)
	case "heisenbridge":
		heisenHomeserverURL := strings.Replace(cfg.HomeserverURL, "https://", "wss://", 1)
		startupCommand = fmt.Sprintf("python -m heisenbridge -c %s -o %s %s", outputPath, cfg.YourUserID, heisenHomeserverURL)
		installInstructions = "https://github.com/beeper/bridge-manager/wiki/Heisenbridge"
	}
	_, _ = fmt.Fprintf(os.Stderr, "\n%s: %s\n", color.YellowString("Startup command"), color.CyanString(startupCommand))
	_, _ = fmt.Fprintf(os.Stderr, "See %s for bridge installation instructions\n", hyper.Link(installInstructions, installInstructions, false))
	return nil
}
