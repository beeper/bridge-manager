package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/beeper/bridge-manager/api/gitlab"
	"github.com/beeper/bridge-manager/log"
)

var runCommand = &cli.Command{
	Name:      "run",
	Usage:     "Run an official Beeper bridge",
	ArgsUsage: "BRIDGE",
	Before:    RequiresAuth,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "type",
			Aliases: []string{"t"},
			EnvVars: []string{"BEEPER_BRIDGE_TYPE"},
			Usage:   "The type of bridge to run.",
		},
		&cli.StringSliceFlag{
			Name:    "param",
			Aliases: []string{"p"},
			Usage:   "Set a bridge-specific config generation option. Can be specified multiple times for different keys. Format: key=value",
		},
		&cli.BoolFlag{
			Name:    "no-update",
			Aliases: []string{"n"},
			Usage:   "Don't update the bridge even if it is out of date.",
			EnvVars: []string{"BEEPER_BRIDGE_NO_UPDATE"},
		},
		&cli.StringFlag{
			Name:    "custom-startup-command",
			Usage:   "A custom binary or script to run for startup. Disables checking for updates entirely.",
			EnvVars: []string{"BEEPER_BRIDGE_CUSTOM_STARTUP_COMMAND"},
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
	Action: runBridge,
}

type VersionJSONOutput struct {
	Name string
	URL  string

	Version          string
	IsRelease        bool
	Commit           string
	FormattedVersion string
	BuildTime        string

	Mautrix struct {
		Version string
		Commit  string
	}
}

func updateGoBridge(ctx context.Context, binaryPath, bridgeType string, noUpdate bool) error {
	var currentVersion VersionJSONOutput

	err := os.MkdirAll(filepath.Dir(binaryPath), 0700)
	if err != nil {
		return err
	}

	if _, err = os.Stat(binaryPath); err == nil || !errors.Is(err, fs.ErrNotExist) {
		if currentVersionBytes, err := exec.Command(binaryPath, "--version-json").Output(); err != nil {
			log.Printf("Failed to get current bridge version: [red]%v[reset] - reinstalling", err)
		} else if err = json.Unmarshal(currentVersionBytes, &currentVersion); err != nil {
			log.Printf("Failed to get parse bridge version: [red]%v[reset] - reinstalling", err)
		}
	}
	return gitlab.DownloadMautrixBridgeBinary(ctx, bridgeType, binaryPath, noUpdate, "", currentVersion.Commit)
}

func setupPythonVenv(ctx context.Context, bridgeDir, bridgeType string) error {
	var installPackage string
	switch bridgeType {
	case "heisenbridge":
		installPackage = "heisenbridge"
	case "telegram", "facebook", "googlechat", "instagram", "twitter":
		installPackage = fmt.Sprintf("mautrix-%s[all]", bridgeType)
	default:
		return fmt.Errorf("unknown python bridge type %s", bridgeType)
	}
	venvPath := filepath.Join(bridgeDir, "venv")
	log.Printf("Creating Python virtualenv at [magenta]%s[reset]", venvPath)
	err := makeCmd(ctx, bridgeDir, "python3", "-m", "venv", venvPath).Run()
	if err != nil {
		return fmt.Errorf("failed to create venv: %w", err)
	}
	log.Printf("Installing [cyan]%s[reset] into virtualenv", installPackage)
	pipPath := filepath.Join(venvPath, "bin", "pip3")
	err = makeCmd(ctx, bridgeDir, pipPath, "install", "--upgrade", installPackage).Run()
	if err != nil {
		return fmt.Errorf("failed to install package: %w", err)
	}
	log.Printf("[green]Installation complete[reset]")
	return nil
}

func makeCmd(ctx context.Context, pwd, path string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = pwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd
}

type installedBridge struct {
	Dir  string
	Cmd  string
	Args []string
	Cfg  *generatedBridgeConfig
}

func doInstallBridge(ctx *cli.Context, bridgeName string, installBinary bool) (*installedBridge, error) {
	cfg, err := doGenerateBridgeConfig(ctx, bridgeName)
	if err != nil {
		return nil, err
	}

	dataDir := GetEnvConfig(ctx).BridgeDataDir
	bridgeDir := filepath.Join(dataDir, bridgeName)
	err = os.MkdirAll(bridgeDir, 0700)
	if err != nil {
		return nil, err
	}

	err = os.WriteFile(filepath.Join(bridgeDir, "config.yaml"), []byte(cfg.Config), 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	var bridgeCmd string
	var bridgeArgs []string
	switch cfg.BridgeType {
	case "imessage", "whatsapp", "discord", "slack", "gmessages":
		bridgeCmd = filepath.Join(dataDir, "binaries", fmt.Sprintf("mautrix-%s", cfg.BridgeType))
		if installBinary {
			err = updateGoBridge(ctx.Context, bridgeCmd, cfg.BridgeType, ctx.Bool("no-update"))
			if err != nil {
				return nil, fmt.Errorf("failed to update bridge: %w", err)
			}
		}
	case "heisenbridge":
		if installBinary {
			err = setupPythonVenv(ctx.Context, bridgeDir, cfg.BridgeType)
			if err != nil {
				return nil, fmt.Errorf("failed to update bridge: %w", err)
			}
		}
		heisenHomeserverURL := strings.Replace(cfg.HomeserverURL, "https://", "wss://", 1)
		bridgeCmd = filepath.Join(bridgeDir, "venv", "bin", "python3")
		bridgeArgs = []string{"-m", "heisenbridge", "-c", "config.yaml", "-o", cfg.YourUserID.String(), heisenHomeserverURL}
	}
	return &installedBridge{
		Dir:  bridgeDir,
		Cmd:  bridgeCmd,
		Args: bridgeArgs,
		Cfg:  cfg,
	}, nil
}

func runBridge(ctx *cli.Context) error {
	if ctx.NArg() == 0 {
		return UserError{"You must specify a bridge to run"}
	} else if ctx.NArg() > 1 {
		return UserError{"Too many arguments specified (flags must come before arguments)"}
	}
	bridgeName := ctx.Args().Get(0)
	overrideBridgeCmd := ctx.String("custom-startup-command")

	installed, err := doInstallBridge(ctx, bridgeName, overrideBridgeCmd == "")
	if err != nil {
		return err
	}
	if overrideBridgeCmd != "" {
		installed.Cmd = overrideBridgeCmd
	}

	cmd := makeCmd(ctx.Context, installed.Dir, installed.Cmd, installed.Args...)
	log.Printf("Starting [cyan]%s[reset]", installed.Cfg.BridgeType)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Printf("Shutting down [cyan]%s[reset]", installed.Cfg.BridgeType)
		proc := cmd.Process
		if proc != nil {
			err := proc.Signal(syscall.SIGTERM)
			if err != nil {
				log.Printf("Failed to send SIGTERM to bridge: %v", err)
			}
		}
		time.Sleep(3 * time.Second)
		log.Printf("Killing process")
		err := proc.Kill()
		if err != nil {
			log.Printf("Failed to kill bridge: %v", err)
		}
		os.Exit(1)
	}()

	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}
