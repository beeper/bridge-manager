package main

import (
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"runtime"
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
		&cli.BoolFlag{
			Name:   "no-state",
			Usage:  "Don't send a bridge state update (dangerous).",
			Hidden: true,
		},
	},
	Action: generateBridgeConfig,
}

func simpleDescriptions(descs map[string]string) func(string, int) string {
	return func(s string, i int) string {
		return descs[s]
	}
}

var askParams = map[string]func(string, map[string]string) (bool, error){
	"meta": func(bridgeName string, extraParams map[string]string) (bool, error) {
		metaPlatform := extraParams["meta_platform"]
		changed := false
		if metaPlatform == "" {
			if strings.Contains(bridgeName, "facebook-tor") || strings.Contains(bridgeName, "facebooktor") {
				metaPlatform = "facebook-tor"
			} else if strings.Contains(bridgeName, "facebook") {
				metaPlatform = "facebook"
			} else if strings.Contains(bridgeName, "messenger") {
				metaPlatform = "messenger"
			} else if strings.Contains(bridgeName, "instagram") {
				metaPlatform = "instagram"
			} else {
				extraParams["meta_platform"] = ""
				return false, nil
			}
			extraParams["meta_platform"] = metaPlatform
		} else if metaPlatform != "instagram" && metaPlatform != "facebook" && metaPlatform != "facebook-tor" && metaPlatform != "messenger" {
			return false, UserError{"Invalid Meta platform specified"}
		}
		if metaPlatform == "facebook-tor" {
			proxy := extraParams["proxy"]
			if proxy == "" {
				err := survey.AskOne(&survey.Input{
					Message: "Enter Tor proxy address",
					Default: "socks5://localhost:1080",
				}, &proxy)
				if err != nil {
					return false, err
				}
				extraParams["proxy"] = proxy
				changed = true
			}
		}
		return changed, nil
	},
	"imessagego": func(bridgeName string, extraParams map[string]string) (bool, error) {
		nacToken := extraParams["nac_token"]
		var didAddParams bool
		if nacToken == "" {
			err := survey.AskOne(&survey.Input{
				Message: "Enter iMessage registration code",
			}, &nacToken)
			if err != nil {
				return didAddParams, err
			}
			extraParams["nac_token"] = nacToken
			didAddParams = true
		}
		return didAddParams, nil
	},
	"imessage": func(bridgeName string, extraParams map[string]string) (bool, error) {
		platform := extraParams["imessage_platform"]
		barcelonaPath := extraParams["barcelona_path"]
		bbURL := extraParams["bluebubbles_url"]
		bbPassword := extraParams["bluebubbles_password"]
		var didAddParams bool
		if runtime.GOOS != "darwin" && platform == "" {
			// Linux can't run the other connectors
			platform = "bluebubbles"
		}
		if platform == "" {
			err := survey.AskOne(&survey.Select{
				Message: "Select iMessage connector:",
				Options: []string{"mac", "mac-nosip", "bluebubbles"},
				Description: simpleDescriptions(map[string]string{
					"mac":         "Use AppleScript to send messages and read chat.db for incoming data - only requires Full Disk Access (from system settings)",
					"mac-nosip":   "Use Barcelona to interact with private APIs - requires disabling SIP and AMFI",
					"bluebubbles": "Connect to a BlueBubbles instance",
				}),
				Default: "mac",
			}, &platform)
			if err != nil {
				return didAddParams, err
			}
			extraParams["imessage_platform"] = platform
			didAddParams = true
		}
		if platform == "mac-nosip" && barcelonaPath == "" {
			err := survey.AskOne(&survey.Input{
				Message: "Enter Barcelona executable path:",
				Default: "darwin-barcelona-mautrix",
			}, &barcelonaPath)
			if err != nil {
				return didAddParams, err
			}
			extraParams["barcelona_path"] = barcelonaPath
			didAddParams = true
		}
		if platform == "bluebubbles" {
			if bbURL == "" {
				err := survey.AskOne(&survey.Input{
					Message: "Enter BlueBubbles API address:",
				}, &bbURL)
				if err != nil {
					return didAddParams, err
				}
				extraParams["bluebubbles_url"] = bbURL
				didAddParams = true
			}
			if bbPassword == "" {
				err := survey.AskOne(&survey.Input{
					Message: "Enter BlueBubbles password:",
				}, &bbPassword)
				if err != nil {
					return didAddParams, err
				}
				extraParams["bluebubbles_password"] = bbPassword
				didAddParams = true
			}
		}
		return didAddParams, nil
	},
	"telegram": func(bridgeName string, extraParams map[string]string) (bool, error) {
		idKey, _ := base64.RawStdEncoding.DecodeString("YXBpX2lk")
		hashKey, _ := base64.RawStdEncoding.DecodeString("YXBpX2hhc2g")
		_, hasID := extraParams[string(idKey)]
		_, hasHash := extraParams[string(hashKey)]
		if !hasID || !hasHash {
			extraParams[string(idKey)] = "26417019"
			// This is mostly here so the api key wouldn't show up in automated searches.
			// It's not really secret, and this key is only used here, cloud bridges have their own key.
			k, _ := base64.RawStdEncoding.DecodeString("qDP2pQ1LogRjxUYrFUDjDw")
			d, _ := base64.RawStdEncoding.DecodeString("B9VMuZeZlFk0pkbLcfSDDQ")
			b, _ := aes.NewCipher(k)
			b.Decrypt(d, d)
			extraParams[string(hashKey)] = hex.EncodeToString(d)
		}
		return false, nil
	},
}

type generatedBridgeConfig struct {
	BridgeType string
	Config     string
	*RegisterJSON
}

// These should match the last 2 digits of https://mau.fi/ports
var bridgeIPSuffix = map[string]string{
	"telegram":   "17",
	"whatsapp":   "18",
	"meta":       "19",
	"googlechat": "20",
	"twitter":    "27",
	"signal":     "28",
	"discord":    "34",
	"slack":      "35",
	"gmessages":  "36",
	"imessagego": "37",
	"gvoice":     "38",
	"bluesky":    "40",
	"linkedin":   "41",
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
		var didAddParams bool
		didAddParams, err = extraParamAsker(bridge, extraParams)
		if err != nil {
			return nil, err
		}
		if didAddParams {
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

	dbPrefix := GetEnvConfig(ctx).DatabaseDir
	if dbPrefix != "" {
		dbPrefix = filepath.Join(dbPrefix, bridge+"-")
	}
	websocket := websocketBridges[bridgeType]
	var listenAddress string
	var listenPort uint16
	if !websocket {
		listenAddress, listenPort, reg.Registration.URL = getBridgeWebsocketProxyConfig(bridge, bridgeType)
	}
	cfg, err := bridgeconfig.Generate(bridgeType, bridgeconfig.Params{
		HungryAddress:  reg.HomeserverURL,
		BeeperDomain:   ctx.String("homeserver"),
		Websocket:      websocket,
		AppserviceID:   reg.Registration.ID,
		ASToken:        reg.Registration.AppToken,
		HSToken:        reg.Registration.ServerToken,
		BridgeName:     bridge,
		Username:       reg.YourUserID.Localpart(),
		UserID:         reg.YourUserID,
		Params:         extraParams,
		DatabasePrefix: dbPrefix,

		ListenAddr: listenAddress,
		ListenPort: listenPort,

		ProvisioningSecret: whoami.User.AsmuxData.LoginToken,
	})
	return &generatedBridgeConfig{
		BridgeType:   bridgeType,
		Config:       cfg,
		RegisterJSON: reg,
	}, err
}

func getBridgeWebsocketProxyConfig(bridgeName, bridgeType string) (listenAddress string, listenPort uint16, url string) {
	ipSuffix := bridgeIPSuffix[bridgeType]
	if ipSuffix == "" {
		ipSuffix = "1"
	}
	listenAddress = "127.29.3." + ipSuffix
	// macOS is weird and doesn't support loopback addresses properly,
	// it only routes 127.0.0.1/32 rather than 127.0.0.0/8
	if runtime.GOOS == "darwin" {
		listenAddress = "127.0.0.1"
	}
	listenPort = uint16(30000 + (crc32.ChecksumIEEE([]byte(bridgeName)) % 30000))
	url = fmt.Sprintf("http://%s:%d", listenAddress, listenPort)
	return
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
	case "imessage", "whatsapp", "discord", "slack", "gmessages", "gvoice", "signal", "meta", "twitter", "bluesky", "linkedin":
		startupCommand = fmt.Sprintf("mautrix-%s", cfg.BridgeType)
		if outputPath != "config.yaml" && outputPath != "<config file>" {
			startupCommand += " -c " + outputPath
		}
		installInstructions = fmt.Sprintf("https://docs.mau.fi/bridges/go/setup.html?bridge=%s#installation", cfg.BridgeType)
	case "imessagego":
		startupCommand = "beeper-imessage"
		if outputPath != "config.yaml" && outputPath != "<config file>" {
			startupCommand += " -c " + outputPath
		}
	case "heisenbridge":
		heisenHomeserverURL := strings.Replace(cfg.HomeserverURL, "https://", "wss://", 1)
		startupCommand = fmt.Sprintf("python -m heisenbridge -c %s -o %s %s", outputPath, cfg.YourUserID, heisenHomeserverURL)
		installInstructions = "https://github.com/beeper/bridge-manager/wiki/Heisenbridge"
	}
	if startupCommand != "" {
		_, _ = fmt.Fprintf(os.Stderr, "\n%s: %s\n", color.YellowString("Startup command"), color.CyanString(startupCommand))
	}
	if installInstructions != "" {
		_, _ = fmt.Fprintf(os.Stderr, "See %s for bridge installation instructions\n", hyper.Link(installInstructions, installInstructions, false))
	}
	return nil
}
