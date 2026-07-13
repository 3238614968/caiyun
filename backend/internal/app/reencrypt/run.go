package reencrypt

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/security"
	"caiyun/internal/version"
	"caiyun/pkg/database"

	"gorm.io/gorm"
)

const defaultBatchSize = 200

type tableTarget struct {
	Name string
}

var supportedTableTargets = map[string]tableTarget{
	"accounts":       {Name: "accounts"},
	"exchange_rules": {Name: "exchange_rules"},
}

type credentialRow struct {
	ID       uint   `gorm:"column:id"`
	Auth     string `gorm:"column:auth"`
	Token    string `gorm:"column:token"`
	JWTToken string `gorm:"column:jwt_token"`
}

type rotationState string

const (
	rotationStateEmpty     rotationState = "empty"
	rotationStateCurrent   rotationState = "current"
	rotationStatePlaintext rotationState = "plaintext"
	rotationStateLegacy    rotationState = "legacy"
)

type tableSummary struct {
	Table               string
	ScannedRows         int64
	ChangedRows         int64
	ChangedFields       int64
	EmptyFields         int64
	CurrentFields       int64
	PlaintextFields     int64
	LegacyVersionFields int64
	ConcurrentSkipped   int64
}

// Run scans encrypted credential columns and optionally rewrites them using
// the configured current encryption-key version.
func Run(ctx context.Context, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	bootstrap.LoadEnvFile()
	if err := security.ValidateFieldCryptoConfig(); err != nil {
		return fmt.Errorf("数据加密配置校验失败: %w", err)
	}
	closeLogger := bootstrap.ConfigureStandardLogger("reencrypt")
	defer closeLogger()

	flags := flag.NewFlagSet("caiyun reencrypt", flag.ContinueOnError)
	apply := flags.Bool("apply", false, "实际写入数据库；默认 dry-run 仅统计待重加密项")
	tableFlag := flags.String("table", "all", "目标表：all/accounts/exchange_rules（兼容 exchange_accounts/rules）")
	batchSize := flags.Int("batch-size", defaultBatchSize, "每批扫描的记录数")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("reencrypt 不支持位置参数: %s", strings.Join(flags.Args(), " "))
	}

	log.Printf("启动 caiyun reencrypt: %+v", version.Get())
	currentVersion, err := security.CurrentEncryptionVersion()
	if err != nil {
		return fmt.Errorf("读取当前数据加密配置失败: %w", err)
	}
	if currentVersion == "" {
		return fmt.Errorf("未检测到 DATA_ENCRYPTION_KEYS / DATA_ENCRYPTION_CURRENT_VERSION，无法执行重加密")
	}

	targets, err := resolveTableTargets(*tableFlag)
	if err != nil {
		return fmt.Errorf("解析 -table 参数失败: %w", err)
	}
	if *batchSize <= 0 {
		*batchSize = defaultBatchSize
	}

	db, err := database.NewMySQL(database.Config{
		Host:            bootstrap.GetEnv("DB_HOST", "localhost"),
		Port:            bootstrap.GetEnv("DB_PORT", "3306"),
		User:            bootstrap.GetEnv("DB_USER", "caiyun_app"),
		Password:        bootstrap.GetEnv("DB_PASSWORD", ""),
		DBName:          bootstrap.GetEnv("DB_NAME", "caiyun"),
		MaxIdleConns:    bootstrap.GetIntEnv("DB_MAX_IDLE_CONNS", 5),
		MaxOpenConns:    bootstrap.GetIntEnv("DB_MAX_OPEN_CONNS", 10),
		ConnMaxLifetime: bootstrap.GetDurationEnv("DB_CONN_MAX_LIFETIME", time.Hour),
		ConnMaxIdleTime: bootstrap.GetDurationEnv("DB_CONN_MAX_IDLE_TIME", 10*time.Minute),
	})
	if err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}
	defer closeDB(db)

	log.Printf("当前写入加密版本: %s，模式: %s，批大小: %d，目标表: %s", currentVersion, executionMode(*apply), *batchSize, joinTargetNames(targets))
	var totalChangedRows int64
	var totalChangedFields int64
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		summary, err := reencryptTable(ctx, db, target, currentVersion, *batchSize, *apply)
		if err != nil {
			return fmt.Errorf("处理表 %s 失败: %w", target.Name, err)
		}
		totalChangedRows += summary.ChangedRows
		totalChangedFields += summary.ChangedFields
		log.Printf("表 %s 完成：scanned_rows=%d changed_rows=%d changed_fields=%d concurrent_skipped=%d plaintext_fields=%d legacy_fields=%d current_fields=%d empty_fields=%d", summary.Table, summary.ScannedRows, summary.ChangedRows, summary.ChangedFields, summary.ConcurrentSkipped, summary.PlaintextFields, summary.LegacyVersionFields, summary.CurrentFields, summary.EmptyFields)
	}

	if !*apply {
		if totalChangedRows > 0 {
			log.Printf("dry-run 完成：共有 %d 行、%d 个字段需要重加密。确认无误后重新执行并加上 -apply。", totalChangedRows, totalChangedFields)
		} else {
			log.Println("dry-run 完成：未发现需要重加密的数据。")
		}
		return nil
	}
	log.Printf("重加密完成：共更新 %d 行、%d 个字段。", totalChangedRows, totalChangedFields)
	return nil
}

func executionMode(apply bool) string {
	if apply {
		return "apply"
	}
	return "dry-run"
}

func closeDB(db *gorm.DB) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Printf("获取底层数据库句柄失败: %v", err)
		return
	}
	if err := sqlDB.Close(); err != nil {
		log.Printf("关闭数据库连接失败: %v", err)
	}
}

func resolveTableTargets(raw string) ([]tableTarget, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || raw == "all" {
		return []tableTarget{supportedTableTargets["accounts"], supportedTableTargets["exchange_rules"]}, nil
	}

	aliasMap := map[string]string{
		"accounts":          "accounts",
		"account":           "accounts",
		"exchange_rules":    "exchange_rules",
		"exchange-rules":    "exchange_rules",
		"rules":             "exchange_rules",
		"exchange_accounts": "exchange_rules",
		"exchange-accounts": "exchange_rules",
		"exchange_account":  "exchange_rules",
		"exchange-account":  "exchange_rules",
		"exchangeaccount":   "exchange_rules",
		"exchangerule":      "exchange_rules",
		"exchange_rule":     "exchange_rules",
		"exchange-rule":     "exchange_rules",
	}

	tokens := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == ' '
	})
	if len(tokens) == 0 {
		return nil, fmt.Errorf("未指定可用表名")
	}

	seen := make(map[string]struct{})
	resolved := make([]tableTarget, 0, len(tokens))
	for _, token := range tokens {
		key, ok := aliasMap[strings.TrimSpace(token)]
		if !ok {
			return nil, fmt.Errorf("不支持的表名 %q，仅支持 all/accounts/exchange_rules", token)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		resolved = append(resolved, supportedTableTargets[key])
	}
	sort.Slice(resolved, func(i, j int) bool { return resolved[i].Name < resolved[j].Name })
	return resolved, nil
}

func joinTargetNames(targets []tableTarget) string {
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		names = append(names, target.Name)
	}
	return strings.Join(names, ",")
}

func reencryptTable(ctx context.Context, db *gorm.DB, target tableTarget, currentVersion string, batchSize int, apply bool) (tableSummary, error) {
	summary := tableSummary{Table: target.Name}
	if db == nil {
		return summary, fmt.Errorf("数据库连接为空")
	}
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	session := db.WithContext(ctx).Session(&gorm.Session{SkipHooks: true}).Unscoped()
	var lastID uint
	for {
		var rows []credentialRow
		err := session.Table(target.Name).
			Select("id, auth, token, jwt_token").
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(batchSize).
			Find(&rows).Error
		if err != nil {
			return summary, err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			lastID = row.ID
			summary.ScannedRows++
			updates := map[string]interface{}{}

			for _, field := range []struct {
				column string
				value  string
			}{
				{column: "auth", value: row.Auth},
				{column: "token", value: row.Token},
				{column: "jwt_token", value: row.JWTToken},
			} {
				newValue, changed, state, err := rotateCredentialValue(field.value, currentVersion)
				if err != nil {
					return summary, fmt.Errorf("id=%d column=%s: %w", row.ID, field.column, err)
				}
				applyRotationState(&summary, state)
				if changed {
					updates[field.column] = newValue
				}
			}

			if len(updates) == 0 {
				continue
			}
			changedFieldCount := int64(len(updates))
			if !apply {
				summary.ChangedRows++
				summary.ChangedFields += changedFieldCount
				continue
			}
			updates["updated_at"] = time.Now()
			result := session.Table(target.Name).
				Where("id = ?", row.ID).
				Where("BINARY COALESCE(auth, '') = BINARY ?", row.Auth).
				Where("BINARY COALESCE(token, '') = BINARY ?", row.Token).
				Where("BINARY COALESCE(jwt_token, '') = BINARY ?", row.JWTToken).
				Updates(updates)
			if result.Error != nil {
				return summary, result.Error
			}
			if result.RowsAffected != 1 {
				summary.ConcurrentSkipped++
				log.Printf("表 %s id=%d 在扫描后发生并发更新，已跳过以避免覆盖新凭据", target.Name, row.ID)
				continue
			}
			summary.ChangedRows++
			summary.ChangedFields += changedFieldCount
		}
		if len(rows) < batchSize {
			break
		}
	}
	return summary, nil
}

func applyRotationState(summary *tableSummary, state rotationState) {
	if summary == nil {
		return
	}
	switch state {
	case rotationStateEmpty:
		summary.EmptyFields++
	case rotationStateCurrent:
		summary.CurrentFields++
	case rotationStatePlaintext:
		summary.PlaintextFields++
	case rotationStateLegacy:
		summary.LegacyVersionFields++
	}
}

func rotateCredentialValue(value, currentVersion string) (string, bool, rotationState, error) {
	if value == "" {
		return value, false, rotationStateEmpty, nil
	}

	plaintext, err := security.DecryptStringAllowPlaintext(value)
	if err != nil {
		return "", false, "", err
	}

	currentFieldVersion := security.EncryptionVersion(value)
	if currentFieldVersion == currentVersion && currentFieldVersion != "" {
		return value, false, rotationStateCurrent, nil
	}

	reencrypted, err := security.EncryptString(plaintext)
	if err != nil {
		return "", false, "", err
	}
	if currentFieldVersion == "" {
		return reencrypted, reencrypted != value, rotationStatePlaintext, nil
	}
	return reencrypted, reencrypted != value, rotationStateLegacy, nil
}
