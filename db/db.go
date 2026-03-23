package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/xrcuo/xrcuo-lib/config"
	_ "modernc.org/sqlite"
)

var DB *sql.DB

func InitDB() error {
	var err error

	dbPath := config.GetDatabasePath()

	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	DB.SetMaxOpenConns(config.GetMaxOpenConns())
	DB.SetMaxIdleConns(config.GetMaxIdleConns())
	DB.SetConnMaxLifetime(-1)
	DB.SetConnMaxIdleTime(10 * time.Minute)

	logrus.Debug("Database connection pool configured")

	if err = DB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %v", err)
	}

	if err = createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %v", err)
	}

	logrus.Info("Database initialized successfully")
	return nil
}

func createTables() error {
	createTableSQLs := []string{
		`CREATE TABLE IF NOT EXISTS stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			total_calls INTEGER NOT NULL DEFAULT 0,
			daily_calls INTEGER NOT NULL DEFAULT 0,
			last_reset_time DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS method_calls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			method TEXT NOT NULL,
			count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(method)
		);`,
		`CREATE TABLE IF NOT EXISTS path_calls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT NOT NULL,
			count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(path)
		);`,
		`CREATE TABLE IF NOT EXISTS ip_calls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ip TEXT NOT NULL,
			count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(ip)
		);`,
		`CREATE TABLE IF NOT EXISTS call_details (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT NOT NULL,
			method TEXT NOT NULL,
			ip TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			status_code INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			max_usage INTEGER NOT NULL DEFAULT 0,
			current_usage INTEGER NOT NULL DEFAULT 0,
			is_permanent BOOLEAN NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, sql := range createTableSQLs {
		if _, err := DB.Exec(sql); err != nil {
			return fmt.Errorf("failed to create table: %v", err)
		}
	}

	indexSQLs := []string{
		"CREATE INDEX IF NOT EXISTS idx_call_details_timestamp ON call_details(timestamp DESC);",
		"CREATE INDEX IF NOT EXISTS idx_call_details_path ON call_details(path);",
		"CREATE INDEX IF NOT EXISTS idx_call_details_method ON call_details(method);",
		"CREATE INDEX IF NOT EXISTS idx_call_details_status ON call_details(status_code);",
		"CREATE INDEX IF NOT EXISTS idx_api_keys_key ON api_keys(key);",
		"CREATE INDEX IF NOT EXISTS idx_ip_calls_ip ON ip_calls(ip);",
		"CREATE INDEX IF NOT EXISTS idx_path_calls_path ON path_calls(path);",
		"CREATE INDEX IF NOT EXISTS idx_method_calls_method ON method_calls(method);",
	}

	for _, sql := range indexSQLs {
		if _, err := DB.Exec(sql); err != nil {
			return fmt.Errorf("failed to create index: %v", err)
		}
	}

	return nil
}

func CloseDB() error {
	if DB != nil {
		logrus.Info("Closing database connection")
		return DB.Close()
	}
	return nil
}

func GetDB() *sql.DB {
	return DB
}

func Transaction(fn func(tx *sql.Tx) error) error {
	tx, err := DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}

	if err := fn(tx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("transaction rollback failed: %v, original error: %v", rollbackErr, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

func WithTransaction(tx *sql.Tx, query string, args ...interface{}) (*sql.Rows, error) {
	if tx != nil {
		return tx.Query(query, args...)
	}
	return DB.Query(query, args...)
}

func WithTransactionExec(tx *sql.Tx, query string, args ...interface{}) (sql.Result, error) {
	if tx != nil {
		return tx.Exec(query, args...)
	}
	return DB.Exec(query, args...)
}
