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
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"
	"maunium.net/go/mautrix/appservice"

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
		&cli.BoolFlag{
			Name:    "local-dev",
			Aliases: []string{"l"},
			Usage:   "Run the bridge in your current working directory instead of downloading and installing a new copy. Useful for developing bridges.",
			EnvVars: []string{"BEEPER_BRIDGE_LOCAL"},
		},
		&cli.StringFlag{
			Name:    "config-file",
			Aliases: []string{"c"},
			Value:   "config.yaml",
			EnvVars: []string{"BEEPER_BRIDGE_CONFIG_FILE"},
			Usage:   "File name to save the config to. Mostly relevant for local dev mode.",
		},
		&cli.BoolFlag{
			Name:    "no-override-config",
			Usage:   "Don't override the config file if it already exists. Defaults to true with --local-dev mode, otherwise false (always override)",
			EnvVars: []string{"BEEPER_BRIDGE_NO_OVERRIDE_CONFIG"},
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

func setupPythonVenv(ctx context.Context, bridgeDir, bridgeType string, localDev bool) (string, error) {
	var installPackage string
	localRequirements := []string{"-r", "requirements.txt"}
	switch bridgeType {
	case "heisenbridge":
		installPackage = "heisenbridge"
	case "telegram", "facebook", "googlechat", "instagram", "twitter":
		//installPackage = fmt.Sprintf("mautrix-%s[all]", bridgeType)
		installPackage = fmt.Sprintf("mautrix-%s[all] @ git+https://github.com/mautrix/%s.git@master", bridgeType, bridgeType)
		localRequirements = append(localRequirements, "-r", "optional-requirements.txt")
	default:
		return "", fmt.Errorf("unknown python bridge type %s", bridgeType)
	}
	var venvPath string
	if localDev {
		venvPath = filepath.Join(bridgeDir, ".venv")
	} else {
		venvPath = filepath.Join(bridgeDir, "venv")
	}
	log.Printf("Creating Python virtualenv at [magenta]%s[reset]", venvPath)
	venvArgs := []string{"-m", "venv"}
	if os.Getenv("SYSTEM_SITE_PACKAGES") == "true" {
		venvArgs = append(venvArgs, "--system-site-packages")
	}
	venvArgs = append(venvArgs, venvPath)
	err := makeCmd(ctx, bridgeDir, "python3", venvArgs...).Run()
	if err != nil {
		return venvPath, fmt.Errorf("failed to create venv: %w", err)
	}
	packages := []string{installPackage}
	if localDev {
		packages = localRequirements
	}
	log.Printf("Installing [cyan]%s[reset] into virtualenv", strings.Join(packages, " "))
	pipPath := filepath.Join(venvPath, "bin", "pip3")
	installArgs := append([]string{"install", "--upgrade"}, packages...)
	err = makeCmd(ctx, bridgeDir, pipPath, installArgs...).Run()
	if err != nil {
		return venvPath, fmt.Errorf("failed to install package: %w", err)
	}
	log.Printf("[green]Installation complete[reset]")
	return venvPath, nil
}

func makeCmd(ctx context.Context, pwd, path string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = pwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd
}

func runBridge(ctx *cli.Context) error {
	if ctx.NArg() == 0 {
		return UserError{"You must specify a bridge to run"}
	} else if ctx.NArg() > 1 {
		return UserError{"Too many arguments specified (flags must come before arguments)"}
	}
	bridgeName := ctx.Args().Get(0)

	cfg, err := doGenerateBridgeConfig(ctx, bridgeName)
	if err != nil {
		return err
	}

	dataDir := GetEnvConfig(ctx).BridgeDataDir
	var bridgeDir string
	localDev := ctx.Bool("local-dev")
	if localDev {
		bridgeDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	} else {
		bridgeDir = filepath.Join(dataDir, bridgeName)
		err = os.MkdirAll(bridgeDir, 0700)
		if err != nil {
			return fmt.Errorf("failed to create bridge directory: %w", err)
		}
	}
	// TODO creating this here feels a bit hacky
	err = os.MkdirAll(filepath.Join(bridgeDir, "logs"), 0700)
	if err != nil {
		return err
	}

	configFileName := ctx.String("config-file")
	configPath := filepath.Join(bridgeDir, configFileName)
	noOverrideConfig := ctx.Bool("no-override-config") || localDev
	doWriteConfig := true
	if noOverrideConfig {
		_, err = os.Stat(configPath)
		doWriteConfig = errors.Is(err, fs.ErrNotExist)
	}
	if doWriteConfig {
		err = os.WriteFile(configPath, []byte(cfg.Config), 0600)
		if err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
	} else {
		log.Printf("Config already exists, not overriding - if you want to regenerate it, delete [cyan]%s[reset]", configPath)
	}

	overrideBridgeCmd := ctx.String("custom-startup-command")
	var bridgeCmd string
	var bridgeArgs []string
	var needsWebsocketProxy bool
	switch cfg.BridgeType {
	case "imessage", "whatsapp", "discord", "slack", "gmessages", "signalgo":
		binaryName := fmt.Sprintf("mautrix-%s", cfg.BridgeType)
		bridgeCmd = filepath.Join(dataDir, "binaries", binaryName)
		if localDev && overrideBridgeCmd == "" {
			bridgeCmd = filepath.Join(bridgeDir, binaryName)
			log.Printf("Compiling [cyan]%s[reset] with ./build.sh", binaryName)
			err = makeCmd(ctx.Context, bridgeDir, "./build.sh").Run()
			if err != nil {
				return fmt.Errorf("failed to compile bridge: %w", err)
			}
		} else if overrideBridgeCmd == "" {
			err = updateGoBridge(ctx.Context, bridgeCmd, cfg.BridgeType, ctx.Bool("no-update"))
			if err != nil {
				return fmt.Errorf("failed to update bridge: %w", err)
			}
		}
		bridgeArgs = []string{"-c", configFileName}
	case "imessagego":
		binaryName := "beeper-imessage"
		if localDev && overrideBridgeCmd == "" {
			bridgeCmd = filepath.Join(bridgeDir, binaryName)
			log.Printf("Compiling [cyan]%s[reset] with ./build.sh", binaryName)
			err = makeCmd(ctx.Context, bridgeDir, "./build.sh").Run()
			if err != nil {
				return fmt.Errorf("failed to compile bridge: %w", err)
			}
		} else if overrideBridgeCmd == "" {
			return UserError{"imessagego only supports --local-dev currently"}
		}
		bridgeArgs = []string{"-c", configFileName}
	case "telegram", "facebook", "googlechat", "instagram", "twitter":
		if overrideBridgeCmd == "" {
			var venvPath string
			venvPath, err = setupPythonVenv(ctx.Context, bridgeDir, cfg.BridgeType, localDev)
			if err != nil {
				return fmt.Errorf("failed to update bridge: %w", err)
			}
			bridgeCmd = filepath.Join(venvPath, "bin", "python3")
		}
		bridgeArgs = []string{"-m", "mautrix_" + cfg.BridgeType, "-c", configFileName}
		needsWebsocketProxy = true
	case "heisenbridge":
		if overrideBridgeCmd == "" {
			var venvPath string
			venvPath, err = setupPythonVenv(ctx.Context, bridgeDir, cfg.BridgeType, localDev)
			if err != nil {
				return fmt.Errorf("failed to update bridge: %w", err)
			}
			bridgeCmd = filepath.Join(venvPath, "bin", "python3")
		}
		heisenHomeserverURL := strings.Replace(cfg.HomeserverURL, "https://", "wss://", 1)
		bridgeArgs = []string{"-m", "heisenbridge", "-c", configFileName, "-o", cfg.YourUserID.String(), heisenHomeserverURL}
	default:
		if overrideBridgeCmd == "" {
			return UserError{"Unsupported bridge type for bbctl run"}
		}
	}
	if overrideBridgeCmd != "" {
		bridgeCmd = overrideBridgeCmd
	}

	cmd := makeCmd(ctx.Context, bridgeDir, bridgeCmd, bridgeArgs...)
	if runtime.GOOS == "linux" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			// Don't pass through signals to the bridge, we'll send a sigterm when we want to stop it.
			// Causes weird issues on macOS, so limited to Linux.
			Setpgid: true,
		}
	}
	var as *appservice.AppService
	var wg sync.WaitGroup
	var cancelWS context.CancelFunc
	wsProxyClosed := make(chan struct{})
	if needsWebsocketProxy {
		wg.Add(2)
		log.Printf("Starting websocket proxy")
		as = appservice.Create()
		as.Registration = cfg.Registration
		as.HomeserverDomain = "beeper.local"
		prepareAppserviceWebsocketProxy(ctx, as)
		var wsCtx context.Context
		wsCtx, cancelWS = context.WithCancel(ctx.Context)
		defer cancelWS()
		go runAppserviceWebsocket(wsCtx, func() {
			wg.Done()
			close(wsProxyClosed)
		}, as)
		go keepaliveAppserviceWebsocket(wsCtx, wg.Done, as)
	}

	log.Printf("Starting [cyan]%s[reset]", cfg.BridgeType)

	c := make(chan os.Signal, 1)
	interrupted := false
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-c:
			interrupted = true
			fmt.Println()
		case <-wsProxyClosed:
			log.Printf("Websocket proxy exited, shutting down bridge")
		}
		log.Printf("Shutting down [cyan]%s[reset]", cfg.BridgeType)
		if as != nil && as.StopWebsocket != nil {
			as.StopWebsocket(appservice.ErrWebsocketManualStop)
		}
		proc := cmd.Process
		// On non-Linux, assume setpgid wasn't set, so the signal will be automatically sent to both processes.
		if proc != nil && runtime.GOOS == "linux" {
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
	if !interrupted {
		log.Printf("Bridge exited")
	}
	if as != nil && as.StopWebsocket != nil {
		as.StopWebsocket(appservice.ErrWebsocketManualStop)
	}
	if cancelWS != nil {
		cancelWS()
	}
	if err != nil {
		return err
	}
	wg.Wait()
	return nil
}
