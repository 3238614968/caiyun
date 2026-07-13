package bootstrap

import (
	"fmt"
	"os"
	"strings"
)

const (
	appEnvironmentKey  = "APP_ENV"
	autoMigrateEnvKey  = "DB_AUTO_MIGRATE"
	productionEnvValue = "production"
)

// resolveEmbeddedMigrationPolicy controls migrations performed as a side effect
// of API/Worker startup. Production runtimes must always use the dedicated
// migrator so rolling replicas cannot race DDL with serving traffic.
func resolveEmbeddedMigrationPolicy() (bool, error) {
	production := strings.EqualFold(strings.TrimSpace(os.Getenv(appEnvironmentKey)), productionEnvValue)
	raw, configured := os.LookupEnv(autoMigrateEnvKey)
	raw = strings.TrimSpace(raw)

	if !configured || raw == "" {
		// Preserve the convenient local-development behavior while making
		// production fail safe when the variable is omitted.
		return !production, nil
	}

	enabled, err := parseMigrationBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s=%q 无效（仅支持 true/false、1/0、yes/no、on/off）", autoMigrateEnvKey, raw)
	}
	if production && enabled {
		return false, fmt.Errorf("%s=%s 在 %s=%s 时被禁止；请先运行专用 migrator，再启动 API/Worker", autoMigrateEnvKey, raw, appEnvironmentKey, productionEnvValue)
	}
	return enabled, nil
}

func parseMigrationBool(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean")
	}
}
