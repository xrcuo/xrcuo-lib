package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/xrcuo/xrcuo-lib/models"
)

func LoadStats() (*models.Stats, error) {
	stats := &models.Stats{
		MethodCalls: make(map[string]int64),
		PathCalls:   make(map[string]int64),
		IPCalls:     make(map[string]int64),
	}

	row := DB.QueryRow("SELECT total_calls, daily_calls, last_reset_time FROM stats ORDER BY updated_at DESC LIMIT 1")
	err := row.Scan(&stats.TotalCalls, &stats.DailyCalls, &stats.LastResetTime)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to load basic stats: %v", err)
	}

	if err == sql.ErrNoRows {
		stats.TotalCalls = 0
		stats.DailyCalls = 0
		stats.LastResetTime = time.Now()
	}

	rows, err := DB.Query("SELECT method, count FROM method_calls")
	if err != nil {
		return nil, fmt.Errorf("failed to load method stats: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var method string
		var count int64
		if scanErr := rows.Scan(&method, &count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan method stat: %v", scanErr)
		}
		stats.MethodCalls[method] = count
	}

	rows, err = DB.Query("SELECT path, count FROM path_calls")
	if err != nil {
		return nil, fmt.Errorf("failed to load path stats: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var path string
		var count int64
		if scanErr := rows.Scan(&path, &count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan path stat: %v", scanErr)
		}
		stats.PathCalls[path] = count
	}

	rows, err = DB.Query("SELECT ip, count FROM ip_calls")
	if err != nil {
		return nil, fmt.Errorf("failed to load IP stats: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ip string
		var count int64
		if scanErr := rows.Scan(&ip, &count); scanErr != nil {
			return nil, fmt.Errorf("failed to scan IP stat: %v", scanErr)
		}
		stats.IPCalls[ip] = count
	}

	rows, err = DB.Query(
		"SELECT path, method, ip, timestamp, status_code FROM call_details ORDER BY timestamp DESC LIMIT 100",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load call details: %v", err)
	}
	defer rows.Close()

	details := make([]*models.CallDetail, 0, 100)
	for rows.Next() {
		var detail models.CallDetail
		if scanErr := rows.Scan(&detail.Path, &detail.Method, &detail.IP, &detail.Timestamp, &detail.StatusCode); scanErr != nil {
			return nil, fmt.Errorf("failed to scan call detail: %v", scanErr)
		}
		details = append(details, &detail)
	}

	for i, j := 0, len(details)-1; i < j; i, j = i+1, j-1 {
		details[i], details[j] = details[j], details[i]
	}

	stats.LastCallDetails = details

	return stats, nil
}

func SaveStats(stats *models.Stats) error {
	tx, err := DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	_, err = tx.Exec(
		"INSERT OR REPLACE INTO stats (id, total_calls, daily_calls, last_reset_time, updated_at) "+
			"VALUES ((SELECT id FROM stats ORDER BY updated_at DESC LIMIT 1), ?, ?, ?, ?)",
		stats.TotalCalls, stats.DailyCalls, stats.LastResetTime, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to save basic stats: %v", err)
	}

	for method, count := range stats.MethodCalls {
		_, err = tx.Exec(
			"INSERT OR REPLACE INTO method_calls (method, count, updated_at) VALUES (?, ?, ?)",
			method, count, time.Now(),
		)
		if err != nil {
			return fmt.Errorf("failed to save method stats: %v", err)
		}
	}

	for path, count := range stats.PathCalls {
		_, err = tx.Exec(
			"INSERT OR REPLACE INTO path_calls (path, count, updated_at) VALUES (?, ?, ?)",
			path, count, time.Now(),
		)
		if err != nil {
			return fmt.Errorf("failed to save path stats: %v", err)
		}
	}

	for ip, count := range stats.IPCalls {
		_, err = tx.Exec(
			"INSERT OR REPLACE INTO ip_calls (ip, count, updated_at) VALUES (?, ?, ?)",
			ip, count, time.Now(),
		)
		if err != nil {
			return fmt.Errorf("failed to save IP stats: %v", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

func SaveCallDetail(detail *models.CallDetail) error {
	return SaveCallDetailsBatch([]*models.CallDetail{detail})
}

func SaveCallDetailsBatch(details []*models.CallDetail) error {
	if len(details) == 0 {
		return nil
	}

	tx, err := DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare("INSERT INTO call_details (path, method, ip, timestamp, status_code) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	for _, detail := range details {
		_, err = stmt.Exec(detail.Path, detail.Method, detail.IP, detail.Timestamp, detail.StatusCode)
		if err != nil {
			return fmt.Errorf("failed to insert call detail: %v", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	go func() {
		_, err := DB.Exec(
			"DELETE FROM call_details WHERE id NOT IN (SELECT id FROM call_details ORDER BY timestamp DESC LIMIT 1000)",
		)
		if err != nil {
			log.Printf("Failed to cleanup old call details: %v", err)
		}
	}()

	return nil
}
