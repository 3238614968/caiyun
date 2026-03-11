package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// TransactionOption 事务选项
type TransactionOption func(*sql.TxOptions)

// WithIsolation 设置隔离级别
func WithIsolation(level sql.IsolationLevel) TransactionOption {
	return func(opts *sql.TxOptions) {
		opts.Isolation = level
	}
}

// WithReadOnly 设置只读事务
func WithReadOnly(readOnly bool) TransactionOption {
	return func(opts *sql.TxOptions) {
		opts.ReadOnly = readOnly
	}
}

// DBManager 数据库管理器（带连接池监控）
type DBManager struct {
	db     *gorm.DB
	sqlDB  *sql.DB
	config Config
}

// NewDBManager 创建数据库管理器
func NewDBManager(db *gorm.DB, config Config) (*DBManager, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库实例失败：%w", err)
	}

	return &DBManager{
		db:     db,
		sqlDB:  sqlDB,
		config: config,
	}, nil
}

// Stats 数据库连接池统计
type Stats struct {
	MaxOpenConnections int // 最大打开连接数
	OpenConnections    int // 当前打开的连接数
	InUse              int // 正在使用的连接数
	Idle               int // 空闲的连接数
	WaitCount          int64 // 等待连接的次数
	WaitDuration       time.Duration // 等待总时长
	MaxIdleClosed      int64 // 因超过 MaxIdleTime 而关闭的连接数
	MaxIdleTimeClosed  int64 // 因超过 ConnMaxIdleTime 而关闭的连接数
	MaxLifetimeClosed  int64 // 因超过 ConnMaxLifetime 而关闭的连接数
}

// GetStats 获取连接池统计信息
func (m *DBManager) GetStats() Stats {
	stats := m.sqlDB.Stats()
	return Stats{
		MaxOpenConnections: stats.MaxOpenConnections,
		OpenConnections:    stats.OpenConnections,
		InUse:              stats.InUse,
		Idle:               stats.Idle,
		WaitCount:          stats.WaitCount,
		WaitDuration:       stats.WaitDuration,
		MaxIdleClosed:      stats.MaxIdleClosed,
		MaxIdleTimeClosed:  stats.MaxIdleTimeClosed,
		MaxLifetimeClosed:  stats.MaxLifetimeClosed,
	}
}

// HealthCheck 健康检查
func (m *DBManager) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.sqlDB.PingContext(ctx)
}

// Close 关闭数据库连接
func (m *DBManager) Close() error {
	return m.sqlDB.Close()
}

// WithTransaction 执行事务（带选项）
func (m *DBManager) WithTransaction(ctx context.Context, fn func(tx *gorm.DB) error, opts ...TransactionOption) error {
	txOpts := &sql.TxOptions{}
	for _, opt := range opts {
		opt(txOpts)
	}

	tx := m.db.WithContext(ctx).Begin(txOpts)
	if tx.Error != nil {
		return fmt.Errorf("开始事务失败：%w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback().Error; rbErr != nil {
			return fmt.Errorf("回滚事务失败：%v (原错误：%w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交事务失败：%w", err)
	}

	return nil
}

// WithTransactionInIsolation 在指定隔离级别下执行事务
func (m *DBManager) WithTransactionInIsolation(ctx context.Context, level sql.IsolationLevel, fn func(tx *gorm.DB) error) error {
	return m.WithTransaction(ctx, fn, WithIsolation(level))
}

// RetryOnDeadlock 死锁自动重试
func (m *DBManager) RetryOnDeadlock(ctx context.Context, maxRetries int, fn func(tx *gorm.DB) error) error {
	var lastErr error
	
	for i := 0; i <= maxRetries; i++ {
		err := m.WithTransaction(ctx, fn)
		if err == nil {
			return nil
		}
		
		lastErr = err
		
		// 检查是否是死锁错误
		if isDeadlock(err) && i < maxRetries {
			// 指数退避：100ms, 200ms, 400ms...
			waitTime := time.Duration(100*(1<<uint(i))) * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitTime):
				continue
			}
		}
		
		break
	}
	
	return fmt.Errorf("执行事务失败：%w", lastErr)
}

// isDeadlock 检查是否是死锁错误
func isDeadlock(err error) bool {
	errStr := err.Error()
	// MySQL 死锁错误码：1213
	// MySQL 超时错误码：1205
	return containsString(errStr, []string{
		"Deadlock found when trying to get lock",
		"Lock wait timeout exceeded",
		"ER_LOCK_DEADLOCK",
		"ER_LOCK_WAIT_TIMEOUT",
	})
}

func containsString(s string, substrings []string) bool {
	for _, sub := range substrings {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// OptimizeConnectionPool 优化连接池配置
func (m *DBManager) OptimizeConnectionPool(maxIdle, maxOpen int, lifetime, idleTime time.Duration) {
	m.sqlDB.SetMaxIdleConns(maxIdle)
	m.sqlDB.SetMaxOpenConns(maxOpen)
	m.sqlDB.SetConnMaxLifetime(lifetime)
	m.sqlDB.SetConnMaxIdleTime(idleTime)
}

// GetDB 获取 GORM DB 实例
func (m *DBManager) GetDB() *gorm.DB {
	return m.db
}

// GetSQLDB 获取 sql.DB 实例
func (m *DBManager) GetSQLDB() *sql.DB {
	return m.sqlDB
}

// BatchInsert 批量插入（分批次，避免单次数据过大）
func (m *DBManager) BatchInsert(ctx context.Context, tableName string, data []map[string]interface{}, batchSize int) error {
	if len(data) == 0 {
		return nil
	}

	if batchSize <= 0 {
		batchSize = 1000
	}

	total := len(data)
	for i := 0; i < total; i += batchSize {
		end := i + batchSize
		if end > total {
			end = total
		}

		batch := data[i:end]
		if err := m.db.WithContext(ctx).Table(tableName).Create(batch).Error; err != nil {
			return fmt.Errorf("批量插入失败：%w", err)
		}
	}

	return nil
}

// Upsert 插入或更新（ON DUPLICATE KEY UPDATE）
func (m *DBManager) Upsert(ctx context.Context, tableName string, data map[string]interface{}, updateColumns []string) error {
	// 使用原生 SQL 实现 UPSERT
	columns := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data))
	placeholders := make([]string, 0, len(data))
	
	for k, v := range data {
		columns = append(columns, k)
		values = append(values, v)
		placeholders = append(placeholders, "?")
	}
	
	updateSet := ""
	if len(updateColumns) > 0 {
		for i, col := range updateColumns {
			if i > 0 {
				updateSet += ", "
			}
			updateSet += fmt.Sprintf("%s = VALUES(%s)", col, col)
		}
	} else {
		// 默认更新所有非主键列
		for i, col := range columns {
			if i > 0 {
				updateSet += ", "
			}
			updateSet += fmt.Sprintf("%s = VALUES(%s)", col, col)
		}
	}
	
	sql := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON DUPLICATE KEY UPDATE %s",
		tableName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
		updateSet,
	)
	
	return m.db.WithContext(ctx).Exec(sql, values...).Error
}

// BulkUpdate 批量更新
func (m *DBManager) BulkUpdate(ctx context.Context, tableName string, updates []map[string]interface{}, keyColumn string) error {
	if len(updates) == 0 {
		return nil
	}

	// 使用原生 SQL 实现批量更新
	for _, update := range updates {
		keyValue, ok := update[keyColumn]
		if !ok {
			return fmt.Errorf("缺少键列：%s", keyColumn)
		}
		
		// 构建 SET 子句
		setClauses := make([]string, 0)
		values := make([]interface{}, 0)
		
		for k, v := range update {
			if k != keyColumn {
				setClauses = append(setClauses, fmt.Sprintf("%s = ?", k))
				values = append(values, v)
			}
		}
		
		if len(setClauses) == 0 {
			continue
		}
		
		sql := fmt.Sprintf(
			"UPDATE %s SET %s WHERE %s = ?",
			tableName,
			strings.Join(setClauses, ", "),
			keyColumn,
		)
		
		values = append(values, keyValue)
		
		if err := m.db.WithContext(ctx).Exec(sql, values...).Error; err != nil {
			return err
		}
	}
	
	return nil
}

// QueryWithCache 带缓存的查询（需要外部缓存支持）
// cacheKey: 缓存键
// ttl: 缓存时间
// queryFn: 实际查询函数
func (m *DBManager) QueryWithCache(ctx context.Context, cacheKey string, ttl time.Duration, queryFn func() ([]map[string]interface{}, error)) ([]map[string]interface{}, error) {
	// 这里可以集成 Redis 或其他缓存
	// 为了保持简单，直接执行查询
	return queryFn()
}

// StreamQuery 流式查询（适用于大数据集）
func (m *DBManager) StreamQuery(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	rows, err := m.sqlDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	// 设置流式选项
	rows.ColumnTypes()
	
	return rows, nil
}

// ExecWithTimeout 带超时的执行
func (m *DBManager) ExecWithTimeout(ctx context.Context, timeout time.Duration, query string, args ...interface{}) (sql.Result, error) {
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return m.sqlDB.ExecContext(execCtx, query, args...)
}

// PrepareStmt 预编译语句
func (m *DBManager) PrepareStmt(ctx context.Context, query string) (*sql.Stmt, error) {
	return m.sqlDB.PrepareContext(ctx, query)
}

// Ping 测试数据库连接
func (m *DBManager) Ping(ctx context.Context) error {
	return m.sqlDB.PingContext(ctx)
}

// BeginTx 开始事务（原生 SQL）
func (m *DBManager) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	// 使用 BeginTx 代替 BeginTxx（Go 1.8+ 标准方法）
	return m.sqlDB.BeginTx(ctx, opts)
}

// SetConfig 动态设置配置
func (m *DBManager) SetConfig(config Config) {
	m.config = config
}

// GetConfig 获取当前配置
func (m *DBManager) GetConfig() Config {
	return m.config
}
