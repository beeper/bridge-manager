package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

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
			Name:    "address",
			Aliases: []string{"a"},
			EnvVars: []string{"BEEPER_BRIDGE_ADDRESS"},
			Usage:   "Optionally, a https address where the Beeper server can push events.\nWhen omitted, the server will expect the bridge to connect with a websocket to receive events.",
		},
		&cli.StringFlag{
			Name:    "listen",
			Aliases: []string{"l"},
			EnvVars: []string{"BEEPER_BRIDGE_LISTEN_ADDRESS"},
			Usage:   "IP and port where the bridge should listen. Only relevant when address is specified.",
		},
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
		},
	},
	Action: generateBridgeConfig,
}

func simpleDescriptions(descs map[string]string) func(string, int) string {
	return func(s string, i int) string {
		return descs[s]
	}
}

var askParams = map[string]func(map[string]any) error{
	"imessage": func(extraParams map[string]any) error {
		platform, _ := extraParams["imessage_platform"].(string)
		barcelonaPath, _ := extraParams["barcelona_path"].(string)
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

func generateBridgeConfig(ctx *cli.Context) error {
	if ctx.NArg() == 0 {
		return UserError{"You must specify a bridge to generate a config for"}
	} else if ctx.NArg() > 1 {
		return UserError{"Too many arguments specified (flags must come before arguments)"}
	}
	bridge := ctx.Args().Get(0)
	if err := validateBridgeName(ctx, bridge); err != nil {
		return err
	}
	bridgeType, err := guessOrAskBridgeType(bridge, ctx.String("type"))
	if err != nil {
		return err
	}
	isWebsocket := ctx.String("address") == ""
	if !isWebsocket && ctx.String("listen") == "" {
		return UserError{"Both --listen and --address must be provided when not using websocket mode"}
	} else if isWebsocket && !websocketBridges[bridgeType] {
		return UserError{fmt.Sprintf("%s doesn't support websockets yet, please provide --address and --listen", bridgeType)}
	}
	extraParamAsker := askParams[bridgeType]
	extraParams := make(map[string]any)
	for _, item := range ctx.StringSlice("param") {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			return UserError{fmt.Sprintf("Invalid param %q", item)}
		}
		extraParams[strings.ToLower(parts[0])] = parts[1]
	}
	if extraParamAsker != nil {
		err = extraParamAsker(extraParams)
		if err != nil {
			return err
		}
	}
	reg, err := doRegisterBridge(ctx, bridge, bridgeType, false)
	if err != nil {
		return err
	}
	var listenAddr string
	var listenPort uint16
	if !isWebsocket {
		_, err = fmt.Sscanf(ctx.String("listen"), "%s:%d", &listenAddr, &listenPort)
		if err != nil {
			return fmt.Errorf("failed to parse listen address: %w", err)
		}
	}

	cfg, err := bridgeconfig.Generate(bridgeType, bridgeconfig.Params{
		HungryAddress: reg.HomeserverURL,
		BeeperDomain:  ctx.String("homeserver"),
		Websocket:     reg.Registration.URL == "websocket",
		ListenAddr:    listenAddr,
		ListenPort:    listenPort,
		AppserviceID:  reg.Registration.ID,
		ASToken:       reg.Registration.AppToken,
		HSToken:       reg.Registration.ServerToken,
		BridgeName:    bridge,
		UserID:        reg.YourUserID,
		Params:        extraParams,
	})
	if err != nil {
		return err
	}
	err = doOutputFile(ctx, "Config", cfg)
	if err != nil {
		return err
	}
	outputPath := ctx.String("output")
	if outputPath == "-" || outputPath == "" {
		outputPath = "<config file>"
	}
	var startupCommand, installInstructions string
	switch bridgeType {
	case "imessage", "whatsapp", "discord", "slack", "gmessages":
		startupCommand = fmt.Sprintf("mautrix-%s", bridgeType)
		if outputPath != "config.yaml" && outputPath != "<config file>" {
			startupCommand += " -c " + outputPath
		}
		installInstructions = fmt.Sprintf("https://docs.mau.fi/bridges/go/setup.html?bridge=%s#installation", bridgeType)
	case "heisenbridge":
		heisenHomeserverURL := reg.HomeserverURL
		if reg.Registration.URL == "websocket" {
			heisenHomeserverURL = strings.Replace(heisenHomeserverURL, "https://", "wss://", 1)
		}
		startupCommand = fmt.Sprintf("python -m heisenbridge -c %s -o %s %s", outputPath, reg.YourUserID, heisenHomeserverURL)
		installInstructions = "https://github.com/beeper/bridge-manager/wiki/Heisenbridge"
	}
	_, _ = fmt.Fprintf(os.Stderr, "\n%s: %s\n", color.YellowString("Startup command"), color.CyanString(startupCommand))
	_, _ = fmt.Fprintf(os.Stderr, "See %s for bridge installation instructions\n", hyper.Link(installInstructions, installInstructions, false))
	return nil
}
