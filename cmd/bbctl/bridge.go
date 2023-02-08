package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/bridge/status"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/bridge-manager/api/beeperapi"
	"github.com/beeper/bridge-manager/api/hungryapi"
)

var bridgeCommand = &cli.Command{
	Name:    "bridge",
	Aliases: []string{"b"},
	Usage:   "Manage your bridges",
	Before:  RequiresAuth,
	Subcommands: []*cli.Command{
		{
			Name:      "register",
			Usage:     "Register a new bridge and print the appservice registration file",
			ArgsUsage: "BRIDGE",
			Action:    registerBridge,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "address",
					Aliases: []string{"a"},
					EnvVars: []string{"BEEPER_BRIDGE_ADDRESS"},
					Usage:   "Optionally, a https address where the Beeper server can push events.\nWhen omitted, the server will expect the bridge to connect with a websocket to receive events.",
				},
				&cli.StringFlag{
					Name:    "output",
					Aliases: []string{"o"},
					Value:   "-",
					EnvVars: []string{"BEEPER_BRIDGE_REGISTRATION_FILE"},
					Usage:   "Path to save generated registration file to.",
				},
				&cli.BoolFlag{
					Name:    "json",
					Aliases: []string{"j"},
					EnvVars: []string{"BEEPER_BRIDGE_REGISTRATION_JSON"},
					Usage:   "Return all data as JSON instead of registration YAML and pretty-printed metadata",
				},
				&cli.BoolFlag{
					Name:    "force",
					Aliases: []string{"f"},
					Usage:   "Force register an official bridge, which is not currently supported.",
				},
			},
		},
		{
			Name:      "get",
			Usage:     "Get the registration of an existing bridge",
			ArgsUsage: "BRIDGE",
			Action:    registerBridge,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "output",
					Aliases: []string{"o"},
					Value:   "-",
					EnvVars: []string{"BEEPER_BRIDGE_REGISTRATION_FILE"},
					Usage:   "Path to save generated registration file to.",
				},
				&cli.BoolFlag{
					Name:    "json",
					Aliases: []string{"j"},
					EnvVars: []string{"BEEPER_BRIDGE_REGISTRATION_JSON"},
					Usage:   "Return all data as JSON instead of registration YAML and pretty-printed metadata",
				},
			},
		},
		{
			Name:      "delete",
			Usage:     "Delete a bridge and all associated rooms on the Beeper servers",
			ArgsUsage: "BRIDGE",
			Action:    deleteBridge,
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "force",
					Aliases: []string{"f"},
					Usage:   "Force delete the bridge, even if it's not self-hosted or doesn't seem to exist.",
				},
			},
		},
	},
}

func deleteBridge(ctx *cli.Context) error {
	if ctx.NArg() == 0 {
		return UserError{"You must specify a bridge to delete"}
	} else if ctx.NArg() > 1 {
		return UserError{"Too many arguments specified (flags must come before arguments)"}
	}
	bridge := ctx.Args().Get(0)
	if !allowedBridgeRegex.MatchString(bridge) {
		return UserError{"Invalid bridge name"}
	} else if bridge == "hungryserv" {
		return UserError{"You really shouldn't do that"}
	}
	homeserver := ctx.String("homeserver")
	accessToken := GetEnvConfig(ctx).AccessToken
	if !ctx.Bool("force") {
		whoami, err := beeperapi.Whoami(homeserver, accessToken)
		if err != nil {
			return fmt.Errorf("failed to get whoami: %w", err)
		}
		bridgeInfo, ok := whoami.User.Bridges[bridge]
		if !ok {
			return UserError{fmt.Sprintf("You don't have a %s bridge.", color.CyanString(bridge))}
		}
		selfHosted, _ := bridgeInfo.BridgeState.Info["isSelfHosted"].(bool)
		if !selfHosted {
			return UserError{fmt.Sprintf("Your %s bridge is not self-hosted.", color.CyanString(bridge))}
		}
	}

	var confirmation bool
	err := survey.AskOne(&survey.Confirm{Message: fmt.Sprintf("Are you sure you want to permanently delete %s?", bridge)}, &confirmation)
	if err != nil {
		return err
	} else if !confirmation {
		return fmt.Errorf("bridge delete cancelled")
	}
	err = beeperapi.DeleteBridge(homeserver, bridge, accessToken)
	if err != nil {
		return fmt.Errorf("error deleting bridge: %w", err)
	}
	fmt.Println("Started deleting bridge")
	return nil
}

var allowedBridgeRegex = regexp.MustCompile("^[a-z0-9-]{1,32}$")
var officialBridges = map[string]struct{}{
	"discord":       {},
	"discordgo":     {},
	"facebook":      {},
	"googlechat":    {},
	"imessagecloud": {},
	"imessage":      {},
	"instagram":     {},
	"linkedin":      {},
	"signal":        {},
	"slack":         {},
	"slackgo":       {},
	"telegram":      {},
	"twitter":       {},
	"whatsapp":      {},
	"androidsms":    {},
}

type RegisterJSON struct {
	Registration     *appservice.Registration `json:"registration"`
	HomeserverURL    string                   `json:"homeserver_url"`
	HomeserverDomain string                   `json:"homeserver_domain"`
	YourUserID       id.UserID                `json:"your_user_id"`
}

func registerBridge(ctx *cli.Context) error {
	if ctx.NArg() == 0 {
		return UserError{"You must specify a bridge to register"}
	} else if ctx.NArg() > 1 {
		return UserError{"Too many arguments specified (flags must come before arguments)"}
	}
	bridge := ctx.Args().Get(0)
	if !allowedBridgeRegex.MatchString(bridge) {
		return UserError{"Invalid bridge name. Names must consist of 1-32 lowercase ASCII letters and digits."}
	}
	if _, isOfficial := officialBridges[bridge]; isOfficial {
		_, _ = fmt.Fprintf(os.Stderr, "%s is an official bridge name.\n", color.CyanString(bridge))
		if !ctx.Bool("force") {
			return UserError{"Self-hosting the official Beeper bridges is not currently supported, as it requires configuring the bridges in a specific way. You may still run official bridges using a different bridge name."}
		}
	}
	onlyGet := ctx.Command.Name == "get"
	homeserver := ctx.String("homeserver")
	envConfig := GetEnvConfig(ctx)
	accessToken := envConfig.AccessToken
	whoami, err := beeperapi.Whoami(homeserver, accessToken)
	if err != nil {
		return fmt.Errorf("failed to get whoami: %w", err)
	}
	bridgeInfo, ok := whoami.User.Bridges[bridge]
	if ok && !onlyGet {
		selfHosted, _ := bridgeInfo.BridgeState.Info["isSelfHosted"].(bool)
		if !selfHosted {
			return UserError{fmt.Sprintf("Your %s bridge is not self-hosted.", color.CyanString(bridge))}
		}
		_, _ = fmt.Fprintf(os.Stderr, "You already have a %s bridge, returning existing registration file\n\n", color.CyanString(bridge))
	}
	hungryAPI := GetHungryClient(ctx)

	req := hungryapi.ReqRegisterAppService{
		Push: false,
	}
	if addr := ctx.String("address"); addr != "" {
		req.Push = true
		req.Address = addr
	}

	var resp appservice.Registration
	if onlyGet {
		if req.Address != "" {
			return UserError{"You can't use --get with --address"}
		}
		resp, err = hungryAPI.GetAppService(bridge)
	} else {
		resp, err = hungryAPI.RegisterAppService(bridge, req)
	}
	if err != nil {
		return fmt.Errorf("failed to register appservice: %w", err)
	}
	// Remove the explicit bot user namespace (same as sender_localpart)
	resp.Namespaces.UserIDs = resp.Namespaces.UserIDs[0:1]

	err = beeperapi.PostBridgeState(ctx.String("homeserver"), GetEnvConfig(ctx).Username, bridge, resp.AppToken, beeperapi.ReqPostBridgeState{
		StateEvent: status.StateRunning,
		Reason:     "SELF_HOST_REGISTERED",
		Info: map[string]any{
			"isHungry":     true,
			"isSelfHosted": true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to mark bridge as RUNNING: %w", err)
	}

	output := &RegisterJSON{
		Registration:     &resp,
		HomeserverURL:    hungryAPI.HomeserverURL.String(),
		HomeserverDomain: "beeper.local",
		YourUserID:       hungryAPI.UserID,
	}
	if homeserver == "beeper.com" || homeserver == "beeper-staging.com" {
		nodeName := whoami.User.Hungryserv.RemoteState[hungryAPI.UserID.String()].Info["node"].(string)
		output.HomeserverURL = fmt.Sprintf(hungryapi.HungryDirectURLTemplate, nodeName, envConfig.ClusterID, homeserver, envConfig.Username)
	}
	if ctx.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	yaml, err := output.Registration.YAML()
	if err != nil {
		return fmt.Errorf("failed to get yaml: %w", err)
	}
	outputPath := ctx.String("output")
	if outputPath == "-" {
		_, _ = fmt.Fprintln(os.Stderr, color.YellowString("Registration file:"))
		fmt.Print(yaml)
	} else {
		err = os.WriteFile(outputPath, []byte(yaml), 0600)
		if err != nil {
			return fmt.Errorf("failed to write registration to %s: %w", outputPath, err)
		}
		_, _ = fmt.Fprintln(os.Stderr, color.YellowString("Wrote registration file to"), color.CyanString(outputPath))
	}
	_, _ = fmt.Fprintln(os.Stderr, color.YellowString("\nAdditional bridge configuration details:"))
	_, _ = fmt.Fprintf(os.Stderr, "* Homeserver domain: %s\n", color.CyanString(output.HomeserverDomain))
	_, _ = fmt.Fprintf(os.Stderr, "* Homeserver URL: %s\n", color.CyanString(output.HomeserverURL))
	_, _ = fmt.Fprintf(os.Stderr, "* Your user ID: %s\n", color.CyanString(output.YourUserID.String()))

	return nil
}
