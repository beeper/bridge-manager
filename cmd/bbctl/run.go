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
		&cli.BoolFlag{
			Name:    "compile",
			Usage:   "Clone the bridge repository and compile it locally instead of downloading a binary from CI. Useful for architectures that aren't built in CI. Not meant for development/modifying the bridge, use --local-dev for that instead.",
			EnvVars: []string{"BEEPER_BRIDGE_COMPILE"},
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

func updateGoBridge(ctx context.Context, binaryPath, bridgeType string, v2, noUpdate bool) error {
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
	return gitlab.DownloadMautrixBridgeBinary(ctx, bridgeType, binaryPath, v2, noUpdate, "", currentVersion.Commit)
}

func compileGoBridge(ctx context.Context, buildDir, binaryPath, bridgeType string, noUpdate bool) error {
	v2 := strings.HasSuffix(bridgeType, "v2")
	bridgeType = strings.TrimSuffix(bridgeType, "v2")
	buildDirParent := filepath.Dir(buildDir)
	err := os.MkdirAll(buildDirParent, 0700)
	if err != nil {
		return err
	}

	if _, err = os.Stat(buildDir); err != nil && errors.Is(err, fs.ErrNotExist) {
		repo := fmt.Sprintf("https://github.com/mautrix/%s.git", bridgeType)
		if bridgeType == "imessagego" {
			repo = "https://github.com/beeper/imessage.git"
		}
		log.Printf("Cloning [cyan]%s[reset] to [cyan]%s[reset]", repo, buildDir)
		err = makeCmd(ctx, buildDirParent, "git", "clone", repo, buildDir).Run()
		if err != nil {
			return fmt.Errorf("failed to clone repo: %w", err)
		}
	} else {
		if _, err = os.Stat(binaryPath); err == nil || !errors.Is(err, fs.ErrNotExist) {
			if _, err = exec.Command(binaryPath, "--version-json").Output(); err != nil {
				log.Printf("Failed to get current bridge version: [red]%v[reset] - reinstalling", err)
			} else if noUpdate {
				log.Printf("Not updating bridge because --no-update was specified")
				return nil
			}
		}
		log.Printf("Pulling [cyan]%s[reset]", buildDir)
		err = makeCmd(ctx, buildDir, "git", "pull").Run()
		if err != nil {
			return fmt.Errorf("failed to pull repo: %w", err)
		}
	}
	buildScript := "./build.sh"
	if v2 {
		buildScript = "./build-v2.sh"
	}
	log.Printf("Compiling bridge with %s", buildScript)
	err = makeCmd(ctx, buildDir, buildScript).Run()
	if err != nil {
		return fmt.Errorf("failed to compile bridge: %w", err)
	}
	log.Printf("Successfully compiled bridge")
	return nil
}

func setupPythonVenv(ctx context.Context, bridgeDir, bridgeType string, localDev bool) (string, error) {
	var installPackage string
	localRequirements := []string{"-r", "requirements.txt"}
	switch bridgeType {
	case "heisenbridge":
		installPackage = "heisenbridge"
	case "telegram", "googlechat":
		installPackage = fmt.Sprintf("mautrix-%s[all]", bridgeType)
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

	var err error
	dataDir := GetEnvConfig(ctx).BridgeDataDir
	var bridgeDir string
	compile := ctx.Bool("compile")
	localDev := ctx.Bool("local-dev")
	if localDev {
		if compile {
			log.Printf("--compile does nothing when using --local-dev")
		}
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

	var cfg *generatedBridgeConfig
	if !doWriteConfig {
		whoami, err := getCachedWhoami(ctx)
		if err != nil {
			return fmt.Errorf("failed to get whoami: %w", err)
		}
		existingBridge, ok := whoami.User.Bridges[bridgeName]
		if !ok || existingBridge.BridgeState.BridgeType == "" {
			log.Printf("Existing bridge type not found, falling back to generating new config")
			doWriteConfig = true
		} else if reg, err := doRegisterBridge(ctx, bridgeName, existingBridge.BridgeState.BridgeType, true); err != nil {
			log.Printf("Failed to get existing bridge registration: %v", err)
			log.Printf("Falling back to generating new config")
			doWriteConfig = true
		} else {
			cfg = &generatedBridgeConfig{
				BridgeType:   existingBridge.BridgeState.BridgeType,
				RegisterJSON: reg,
			}
		}
	}

	if doWriteConfig {
		cfg, err = doGenerateBridgeConfig(ctx, bridgeName)
		if err != nil {
			return err
		}
		err = os.WriteFile(configPath, []byte(cfg.Config), 0600)
		if err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
	} else {
		log.Printf("Config already exists, not overriding - if you want to regenerate it, delete [cyan]%s[reset]", configPath)
	}

	overrideBridgeCmd := ctx.String("custom-startup-command")
	if overrideBridgeCmd != "" {
		if localDev {
			log.Printf("--local-dev does nothing when using --custom-startup-command")
		}
		if compile {
			log.Printf("--compile does nothing when using --custom-startup-command")
		}
	}
	var bridgeCmd string
	var bridgeArgs []string
	var needsWebsocketProxy bool
	switch cfg.BridgeType {
	case "imessage", "imessagego", "whatsapp", "discord", "slack", "gmessages", "gvoice", "signal", "meta", "twitter", "bluesky", "linkedin":
		binaryName := fmt.Sprintf("mautrix-%s", cfg.BridgeType)
		ciV2 := false
		switch cfg.BridgeType {
		case "":
			ciV2 = true
		}
		if cfg.BridgeType == "imessagego" {
			binaryName = "beeper-imessage"
		}
		bridgeCmd = filepath.Join(dataDir, "binaries", binaryName)
		if localDev && overrideBridgeCmd == "" {
			bridgeCmd = filepath.Join(bridgeDir, binaryName)
			buildScript := "./build.sh"
			if ciV2 {
				buildScript = "./build-v2.sh"
			}
			log.Printf("Compiling [cyan]%s[reset] with %s", binaryName, buildScript)
			err = makeCmd(ctx.Context, bridgeDir, buildScript).Run()
			if err != nil {
				return fmt.Errorf("failed to compile bridge: %w", err)
			}
		} else if compile && overrideBridgeCmd == "" {
			buildDir := filepath.Join(dataDir, "compile", binaryName)
			bridgeCmd = filepath.Join(buildDir, binaryName)
			err = compileGoBridge(ctx.Context, buildDir, bridgeCmd, cfg.BridgeType, ctx.Bool("no-update"))
			if err != nil {
				return fmt.Errorf("failed to compile bridge: %w", err)
			}
		} else if overrideBridgeCmd == "" {
			err = updateGoBridge(ctx.Context, bridgeCmd, cfg.BridgeType, ciV2, ctx.Bool("no-update"))
			if errors.Is(err, gitlab.ErrNotBuiltInCI) {
				return UserError{fmt.Sprintf("Binaries for %s are not built in the CI. Use --compile to tell bbctl to build the bridge locally.", binaryName)}
			} else if err != nil {
				return fmt.Errorf("failed to update bridge: %w", err)
			}
		}
		bridgeArgs = []string{"-c", configFileName}
	case "telegram", "googlechat":
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
		if cfg.Registration.URL == "" || cfg.Registration.URL == "websocket" {
			_, _, cfg.Registration.URL = getBridgeWebsocketProxyConfig(bridgeName, cfg.BridgeType)
		}
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
