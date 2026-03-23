package models

import "time"

type APIKey struct {
	ID           int64     `json:"id"`
	Key          string    `json:"key"`
	Name         string    `json:"name"`
	MaxUsage     int64     `json:"max_usage"`
	CurrentUsage int64     `json:"current_usage"`
	IsPermanent  bool      `json:"is_permanent"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Stats struct {
	TotalCalls     int64            `json:"total_calls"`
	DailyCalls     int64            `json:"daily_calls"`
	HourlyCalls    int64            `json:"hourly_calls"`
	LastResetTime  time.Time        `json:"last_reset_time"`
	MethodCalls    map[string]int64 `json:"method_calls"`
	PathCalls      map[string]int64 `json:"path_calls"`
	IPCalls        map[string]int64 `json:"ip_calls"`
	LastCallDetails []*CallDetail   `json:"last_call_details"`
}

type CallDetail struct {
	Path      string    `json:"path"`
	Method    string    `json:"method"`
	IP        string    `json:"ip"`
	Timestamp time.Time `json:"timestamp"`
	StatusCode int      `json:"status_code"`
}
