package db

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"

	"github.com/xrcuo/xrcuo-lib/models"
)

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

func CreateAPIKey(name string, maxUsage int64, isPermanent bool) (*models.APIKey, error) {
	key, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %v", err)
	}

	now := time.Now()
	result, err := DB.Exec(
		"INSERT INTO api_keys (key, name, max_usage, current_usage, is_permanent, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		key, name, maxUsage, 0, isPermanent, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get API key ID: %v", err)
	}

	return &models.APIKey{
		ID:           id,
		Key:          key,
		Name:         name,
		MaxUsage:     maxUsage,
		CurrentUsage: 0,
		IsPermanent:  isPermanent,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func GetAPIKeyByKey(key string) (*models.APIKey, error) {
	apiKey := &models.APIKey{}
	err := DB.QueryRow(
		"SELECT id, key, name, max_usage, current_usage, is_permanent, created_at, updated_at FROM api_keys WHERE key = ?",
		key,
	).Scan(
		&apiKey.ID, &apiKey.Key, &apiKey.Name, &apiKey.MaxUsage,
		&apiKey.CurrentUsage, &apiKey.IsPermanent, &apiKey.CreatedAt, &apiKey.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("API key not found")
		}
		return nil, fmt.Errorf("failed to query API key: %v", err)
	}

	return apiKey, nil
}

func UpdateAPIKeyUsage(key string) error {
	result, err := DB.Exec(
		"UPDATE api_keys SET current_usage = current_usage + 1, updated_at = ? WHERE key = ? AND (is_permanent = 1 OR current_usage < max_usage)",
		time.Now(), key,
	)
	if err != nil {
		return fmt.Errorf("failed to update API key usage: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %v", err)
	}

	if rowsAffected == 0 {
		var count int
		if err := DB.QueryRow("SELECT COUNT(*) FROM api_keys WHERE key = ?", key).Scan(&count); err != nil {
			return fmt.Errorf("failed to check API key existence: %v", err)
		}

		if count == 0 {
			return fmt.Errorf("API key not found")
		}
		return fmt.Errorf("API key usage limit reached")
	}

	return nil
}

func DeleteAPIKey(id int64) error {
	_, err := DB.Exec("DELETE FROM api_keys WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete API key: %v", err)
	}
	return nil
}

func GetAllAPIKeys() ([]*models.APIKey, error) {
	rows, err := DB.Query(
		"SELECT id, key, name, max_usage, current_usage, is_permanent, created_at, updated_at FROM api_keys ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query API keys: %v", err)
	}
	defer rows.Close()

	var apiKeys []*models.APIKey
	for rows.Next() {
		apiKey := &models.APIKey{}
		if err := rows.Scan(
			&apiKey.ID, &apiKey.Key, &apiKey.Name, &apiKey.MaxUsage,
			&apiKey.CurrentUsage, &apiKey.IsPermanent, &apiKey.CreatedAt, &apiKey.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan API key: %v", err)
		}
		apiKeys = append(apiKeys, apiKey)
	}

	return apiKeys, nil
}
