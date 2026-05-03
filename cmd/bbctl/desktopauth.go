package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/urfave/cli/v2"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/bridge-manager/api/beeperapi"

	_ "go.mau.fi/util/dbutil/litestream"
)

var loginDesktopCommand = &cli.Command{
	Name:   "login-desktop",
	Usage:  "Import Beeper Desktop's Matrix credentials into bbctl config",
	Action: loginDesktop,
	Flags:  desktopLoginFlags(),
}

func desktopLoginFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "profile",
			EnvVars: []string{"BEEPER_PROFILE"},
			Usage:   "Beeper Desktop profile name, equivalent to BEEPER_PROFILE in Desktop",
		},
		&cli.StringFlag{
			Name:    "desktop-data-dir",
			EnvVars: []string{"BBCTL_DESKTOP_DATA_DIR"},
			Usage:   "Read Matrix credentials from this Beeper Desktop user data directory",
		},
		&cli.StringFlag{
			Name:    "desktop-account-db",
			EnvVars: []string{"BBCTL_DESKTOP_ACCOUNT_DB"},
			Usage:   "Read Matrix credentials from this Beeper Desktop account.db",
		},
	}
}

type DesktopAccount struct {
	UserID      id.UserID
	DeviceID    id.DeviceID
	AccessToken string
	Homeserver  string
}

func getDesktopAccountDBPath(ctx *cli.Context) (string, bool) {
	if dbPath := ctx.String("desktop-account-db"); dbPath != "" {
		return dbPath, true
	}
	if dataDir := ctx.String("desktop-data-dir"); dataDir != "" {
		return filepath.Join(dataDir, "account.db"), true
	}
	return "", false
}

func resolveDesktopDataDir(profile string) (string, error) {
	appName := "BeeperTexts"
	if profile != "" {
		appName += "-" + profile
	}
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", appName), nil
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, appName), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, appName), nil
	default:
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			configHome = filepath.Join(home, ".config")
		}
		return filepath.Join(configHome, appName), nil
	}
}

func getLoginDesktopAccountDBPath(ctx *cli.Context) (string, error) {
	if dbPath, ok := getDesktopAccountDBPath(ctx); ok {
		return dbPath, nil
	}
	dataDir, err := resolveDesktopDataDir(ctx.String("profile"))
	if err != nil {
		return "", fmt.Errorf("failed to resolve desktop data directory: %w", err)
	}
	return filepath.Join(dataDir, "account.db"), nil
}

func readDesktopAccount(ctx context.Context, dbPath string) (*DesktopAccount, error) {
	dbURI := (&url.URL{
		Scheme:   "file",
		Path:     dbPath,
		RawQuery: "mode=ro",
	}).String()
	db, err := dbutil.NewWithDialect(dbURI, "sqlite3-fk-wal")
	if err != nil {
		return nil, fmt.Errorf("failed to open desktop account database: %w", err)
	}
	defer db.Close()

	var account DesktopAccount
	err = db.QueryRow(ctx, "SELECT user_id, device_id, access_token, homeserver FROM account LIMIT 1").
		Scan(&account.UserID, &account.DeviceID, &account.AccessToken, &account.Homeserver)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("desktop account database has no logged-in account")
	} else if err != nil {
		return nil, fmt.Errorf("failed to read desktop account database: %w", err)
	} else if account.UserID == "" || account.AccessToken == "" {
		return nil, fmt.Errorf("desktop account database has incomplete credentials")
	}
	return &account, nil
}

func desktopAccountHomeserverDomain(account *DesktopAccount) (string, error) {
	if account.Homeserver == "" {
		return "", nil
	}
	parsed, err := url.Parse(account.Homeserver)
	if err != nil {
		return "", fmt.Errorf("desktop account has invalid homeserver URL %q: %w", account.Homeserver, err)
	}
	return strings.TrimPrefix(parsed.Host, "matrix."), nil
}

func envForHomeserverDomain(domain string) string {
	for env, envDomain := range envs {
		if domain == envDomain {
			return env
		}
	}
	return ""
}

func saveDesktopLogin(ctx *cli.Context, account *DesktopAccount) (string, string, error) {
	homeserver, err := desktopAccountHomeserverDomain(account)
	if err != nil {
		return "", "", err
	}
	env := ctx.String("env")
	if homeserverEnv := envForHomeserverDomain(homeserver); homeserverEnv != "" {
		env = homeserverEnv
		homeserver = envs[env]
	} else if homeserver == "" {
		homeserver = ctx.String("homeserver")
	}

	whoami, err := beeperapi.Whoami(homeserver, account.AccessToken)
	if err != nil {
		return "", "", fmt.Errorf("failed to verify desktop credentials with whoami: %w", err)
	}

	cfg := GetConfig(ctx)
	envCfg := cfg.Environments.Get(env)
	envCfg.ClusterID = whoami.UserInfo.BridgeClusterID
	envCfg.Username = whoami.UserInfo.Username
	envCfg.AccessToken = account.AccessToken
	envCfg.BridgeDataDir = filepath.Join(UserDataDir, "bbctl", env)
	err = cfg.Save()
	if err != nil {
		return "", "", fmt.Errorf("failed to save config: %w", err)
	}

	return env, homeserver, nil
}

func loginDesktop(ctx *cli.Context) error {
	dbPath, err := getLoginDesktopAccountDBPath(ctx)
	if err != nil {
		return err
	}

	account, err := readDesktopAccount(ctx.Context, dbPath)
	if err != nil {
		return err
	}

	env, homeserver, err := saveDesktopLogin(ctx, account)
	if err != nil {
		return err
	}

	fmt.Printf("Imported Desktop login for %s into bbctl env %q (%s)\n", account.UserID, env, homeserver)
	return nil
}
