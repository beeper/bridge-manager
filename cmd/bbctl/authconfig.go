package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"go.mau.fi/util/random"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/bridge-manager/log"
)

var envs = map[string]string{
	"prod":    "beeper.com",
	"staging": "beeper-staging.com",
	"dev":     "beeper-dev.com",
	"local":   "beeper.localtest.me",
}

type EnvConfig struct {
	ClusterID     string `json:"cluster_id"`
	Username      string `json:"username"`
	AccessToken   string `json:"access_token"`
	BridgeDataDir string `json:"bridge_data_dir"`
	DatabaseDir   string `json:"database_dir,omitempty"`
}

func (ec *EnvConfig) HasCredentials() bool {
	return strings.HasPrefix(ec.AccessToken, "syt_")
}

type EnvConfigs map[string]*EnvConfig

func (ec EnvConfigs) Get(env string) *EnvConfig {
	conf, ok := ec[env]
	if !ok {
		conf = &EnvConfig{}
		ec[env] = conf
	}
	return conf
}

type Config struct {
	DeviceID     id.DeviceID `json:"device_id"`
	Environments EnvConfigs  `json:"environments"`
	Path         string      `json:"-"`
}

var UserDataDir string

func getUserDataDir() (dir string, err error) {
	dir = os.Getenv("BBCTL_DATA_HOME")
	if dir != "" {
		return
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return os.UserConfigDir()
	}
	dir = os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		dir = os.Getenv("HOME")
		if dir == "" {
			return "", errors.New("neither $XDG_DATA_HOME nor $HOME are defined")
		}
		dir = filepath.Join(dir, ".local", "share")
	}
	return
}

func init() {
	var err error
	UserDataDir, err = getUserDataDir()
	if err != nil {
		panic(fmt.Errorf("couldn't find data directory: %w", err))
	}
}

func migrateOldConfig(currentPath string) error {
	baseConfigDir, err := os.UserConfigDir()
	if err != nil {
		panic(err)
	}
	newDefault := path.Join(baseConfigDir, "bbctl", "config.json")
	oldDefault := path.Join(baseConfigDir, "bbctl.json")
	if currentPath != newDefault {
		return nil
	} else if _, err = os.Stat(oldDefault); err != nil {
		return nil
	} else if err = os.MkdirAll(filepath.Dir(newDefault), 0700); err != nil {
		return err
	} else if err = os.Rename(oldDefault, newDefault); err != nil {
		return err
	} else {
		log.Printf("Moved config to new path (from %s to %s)", oldDefault, newDefault)
		return nil
	}
}

func loadConfig(path string) (ret *Config, err error) {
	defer func() {
		if ret == nil {
			return
		}
		ret.Path = path
		if ret.DeviceID == "" {
			ret.DeviceID = id.DeviceID("bbctl_" + strings.ToUpper(random.String(8)))
		}
		if ret.Environments == nil {
			ret.Environments = make(EnvConfigs)
		}
		for key, env := range ret.Environments {
			if env == nil {
				delete(ret.Environments, key)
				continue
			}
			if env.BridgeDataDir == "" {
				env.BridgeDataDir = filepath.Join(UserDataDir, "bbctl", key)
				saveErr := ret.Save()
				if saveErr != nil {
					err = fmt.Errorf("failed to save config after updating data directory: %w", err)
				}
			}
		}
	}()

	err = migrateOldConfig(path)
	if err != nil {
		return nil, fmt.Errorf("failed to move config to new path: %w", err)
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to open config at %s for reading: %v", path, err)
	}
	var cfg Config
	err = json.NewDecoder(file).Decode(&cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config at %s: %v", path, err)
	}
	return &cfg, nil
}

func (cfg *Config) Save() error {
	dirName := filepath.Dir(cfg.Path)
	err := os.MkdirAll(dirName, 0700)
	if err != nil {
		return fmt.Errorf("failed to create config directory at %s: %w", dirName, err)
	}
	file, err := os.OpenFile(cfg.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open config at %s for writing: %v", cfg.Path, err)
	}
	err = json.NewEncoder(file).Encode(cfg)
	if err != nil {
		return fmt.Errorf("failed to write config to %s: %v", cfg.Path, err)
	}
	return nil
}
