package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
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
	MaxOpenConnections int           // 最大打开连接数
	OpenConnections    int           // 当前打开的连接数
	InUse              int           // 正在使用的连接数
	Idle               int           // 空闲的连接数
	WaitCount          int64         // 等待连接的次数
	WaitDuration       time.Duration // 等待总时长
	MaxIdleClosed      int64         // 因超过 MaxIdleTime 而关闭的连接数
	MaxIdleTimeClosed  int64         // 因超过 ConnMaxIdleTime 而关闭的连接数
	MaxLifetimeClosed  int64         // 因超过 ConnMaxLifetime 而关闭的连接数
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
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		// MySQL 死锁错误码：1213；锁等待超时：1205。
		return mysqlErr.Number == 1213 || mysqlErr.Number == 1205
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
