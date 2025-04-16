package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/maps"
	"maunium.net/go/mautrix/bridge/status"

	"github.com/beeper/bridge-manager/api/beeperapi"
	"github.com/beeper/bridge-manager/cli/hyper"
	"github.com/beeper/bridge-manager/log"
)

var whoamiCommand = &cli.Command{
	Name:    "whoami",
	Aliases: []string{"w"},
	Usage:   "Get info about yourself",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "raw",
			Aliases: []string{"r"},
			EnvVars: []string{"BEEPER_WHOAMI_RAW"},
			Usage:   "Get raw JSON output instead of pretty-printed bridge status",
		},
	},
	Before: RequiresAuth,
	Action: whoamiFunction,
}

func coloredHomeserver(domain string) string {
	switch domain {
	case "beeper.com":
		return color.GreenString(domain)
	case "beeper-staging.com":
		return color.CyanString(domain)
	case "beeper-dev.com":
		return color.RedString(domain)
	case "beeper.localtest.me":
		return color.YellowString(domain)
	default:
		return domain
	}
}

func coloredChannel(channel string) string {
	switch channel {
	case "STABLE":
		return color.GreenString(channel)
	case "NIGHTLY":
		return color.YellowString(channel)
	case "INTERNAL":
		return color.RedString(channel)
	default:
		return channel
	}
}

func coloredBridgeState(state status.BridgeStateEvent) string {
	switch state {
	case status.StateStarting, status.StateConnecting:
		return color.CyanString(string(state))
	case status.StateTransientDisconnect, status.StateBridgeUnreachable:
		return color.YellowString(string(state))
	case status.StateUnknownError, status.StateBadCredentials:
		return color.RedString(string(state))
	case status.StateRunning, status.StateConnected:
		return color.GreenString(string(state))
	default:
		return string(state)
	}
}

var bridgeImageRegex = regexp.MustCompile(`^docker\.beeper-tools\.com/(?:bridge/)?([a-z]+):(v2-)?([0-9a-f]{40})(?:-amd64)?$`)

var dockerToGitRepo = map[string]string{
	"hungryserv":  "https://github.com/beeper/hungryserv/commit/%s",
	"discordgo":   "https://github.com/mautrix/discord/commit/%s",
	"dummybridge": "https://github.com/beeper/dummybridge/commit/%s",
	"facebook":    "https://github.com/mautrix/facebook/commit/%s",
	"googlechat":  "https://github.com/mautrix/googlechat/commit/%s",
	"instagram":   "https://github.com/mautrix/instagram/commit/%s",
	"meta":        "https://github.com/mautrix/meta/commit/%s",
	"linkedin":    "https://github.com/mautrix/linkedin/commit/%s",
	"signal":      "https://github.com/mautrix/signal/commit/%s",
	"slackgo":     "https://github.com/mautrix/slack/commit/%s",
	"telegram":    "https://github.com/mautrix/telegram/commit/%s",
	"telegramgo":  "https://github.com/mautrix/telegramgo/commit/%s",
	"twitter":     "https://github.com/mautrix/twitter/commit/%s",
	"bluesky":     "https://github.com/mautrix/bluesky/commit/%s",
	"whatsapp":    "https://github.com/mautrix/whatsapp/commit/%s",
}

func parseBridgeImage(bridge, image string, internal bool) string {
	if image == "" || image == "?" {
		// Self-hosted bridges don't have a version in whoami
		return ""
	} else if bridge == "imessagecloud" {
		return image[:8]
	}
	match := bridgeImageRegex.FindStringSubmatch(image)
	if match == nil {
		return color.YellowString(image)
	}
	if match[1] == "hungryserv" && !internal {
		return match[3][:8]
	}
	return color.HiBlueString(match[2] + hyper.Link(match[3][:8], fmt.Sprintf(dockerToGitRepo[match[1]], match[3]), false))
}

func formatBridgeRemotes(name string, bridge beeperapi.WhoamiBridge) string {
	switch {
	case name == "hungryserv", name == "androidsms", name == "imessage":
		return ""
	case len(bridge.RemoteState) == 0:
		if bridge.BridgeState.IsSelfHosted {
			return ""
		}
		return color.YellowString("not logged in")
	case len(bridge.RemoteState) == 1:
		remoteState := maps.Values(bridge.RemoteState)[0]
		return fmt.Sprintf("remote: %s (%s / %s)", coloredBridgeState(remoteState.StateEvent), color.CyanString(remoteState.RemoteName), color.CyanString(remoteState.RemoteID))
	case len(bridge.RemoteState) > 1:
		return "multiple remotes"
	}
	return ""
}

func formatBridge(name string, bridge beeperapi.WhoamiBridge, internal bool) string {
	formatted := color.CyanString(name)
	versionString := parseBridgeImage(name, bridge.Version, internal)
	if versionString != "" {
		formatted += fmt.Sprintf(" (version: %s)", versionString)
	}
	if bridge.BridgeState.IsSelfHosted {
		var typeName string
		if !strings.Contains(name, bridge.BridgeState.BridgeType) {
			typeName = bridge.BridgeState.BridgeType + ", "
		}
		formatted += fmt.Sprintf(" (%s%s)", typeName, color.HiGreenString("self-hosted"))
	}
	formatted += fmt.Sprintf(" - %s", coloredBridgeState(bridge.BridgeState.StateEvent))
	remotes := formatBridgeRemotes(name, bridge)
	if remotes != "" {
		formatted += " - " + remotes
	}
	return formatted
}

var cachedWhoami *beeperapi.RespWhoami

func getCachedWhoami(ctx *cli.Context) (*beeperapi.RespWhoami, error) {
	if cachedWhoami != nil {
		return cachedWhoami, nil
	}
	ec := GetEnvConfig(ctx)
	resp, err := beeperapi.Whoami(ctx.String("homeserver"), ec.AccessToken)
	if err != nil {
		return nil, err
	}
	changed := false
	if ec.Username != resp.UserInfo.Username {
		ec.Username = resp.UserInfo.Username
		changed = true
	}
	if ec.ClusterID != resp.UserInfo.BridgeClusterID {
		ec.ClusterID = resp.UserInfo.BridgeClusterID
		changed = true
	}
	if changed {
		err = GetConfig(ctx).Save()
		if err != nil {
			log.Printf("Failed to save config after updating: %v", err)
		}
	}
	cachedWhoami = resp
	return resp, nil
}

func whoamiFunction(ctx *cli.Context) error {
	whoami, err := getCachedWhoami(ctx)
	if err != nil {
		return fmt.Errorf("failed to get whoami: %w", err)
	}
	if ctx.Bool("raw") {
		data, err := json.MarshalIndent(whoami, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	if oldID := GetEnvConfig(ctx).ClusterID; whoami.UserInfo.BridgeClusterID != oldID {
		GetEnvConfig(ctx).ClusterID = whoami.UserInfo.BridgeClusterID
		err = GetConfig(ctx).Save()
		if err != nil {
			fmt.Printf("Noticed cluster ID changed from %s to %s, but failed to save change: %v\n", oldID, whoami.UserInfo.BridgeClusterID, err)
		} else {
			fmt.Printf("Noticed cluster ID changed from %s to %s and saved to config\n", oldID, whoami.UserInfo.BridgeClusterID)
		}
	}
	homeserver := ctx.String("homeserver")
	fmt.Printf("User ID: @%s:%s\n", color.GreenString(whoami.UserInfo.Username), coloredHomeserver(homeserver))
	if whoami.UserInfo.Admin {
		fmt.Printf("Admin: %s\n", color.RedString("true"))
	}
	if whoami.UserInfo.Free {
		fmt.Printf("Free: %s\n", color.GreenString("true"))
	}
	fmt.Printf("Name: %s\n", color.CyanString(whoami.UserInfo.FullName))
	fmt.Printf("Email: %s\n", color.CyanString(whoami.UserInfo.Email))
	fmt.Printf("Support room ID: %s\n", color.CyanString(whoami.UserInfo.SupportRoomID.String()))
	fmt.Printf("Registered at: %s\n", color.CyanString(whoami.UserInfo.CreatedAt.Local().Format(BuildTimeFormat)))
	fmt.Printf("Cloud bridge details:\n")
	fmt.Printf("  Update channel: %s\n", coloredChannel(whoami.UserInfo.Channel))
	fmt.Printf("  Cluster ID: %s\n", color.CyanString(whoami.UserInfo.BridgeClusterID))
	hungryAPI := GetHungryClient(ctx)
	homeserverURL := hungryAPI.HomeserverURL.String()
	fmt.Printf("  Hungryserv URL: %s\n", color.CyanString(hyper.Link(homeserverURL, homeserverURL, false)))
	fmt.Printf("Bridges:\n")
	internal := homeserver != "beeper.com" || whoami.UserInfo.Channel == "INTERNAL"
	fmt.Println(" ", formatBridge("hungryserv", whoami.User.Hungryserv, internal))
	keys := maps.Keys(whoami.User.Bridges)
	sort.Strings(keys)
	for _, name := range keys {
		fmt.Println(" ", formatBridge(name, whoami.User.Bridges[name], internal))
	}
	return nil
}
