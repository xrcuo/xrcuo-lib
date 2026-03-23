package xrcuolib

import (
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xrcuo/xrcuo-lib/db"
	"github.com/xrcuo/xrcuo-lib/models"
)

type Stats struct {
	models.Stats
	mu sync.RWMutex

	callDetailBuffer []*models.CallDetail
	bufferMutex     sync.Mutex
	maxBufferSize   int
}

var GlobalStats *Stats

func InitStats() {
	log.Println("Initializing stats...")

	statsData, err := db.LoadStats()
	if err != nil {
		log.Printf("Failed to load stats from database: %v, using defaults", err)
		statsData = &models.Stats{
			TotalCalls:      0,
			DailyCalls:      0,
			HourlyCalls:     0,
			MethodCalls:     make(map[string]int64),
			PathCalls:       make(map[string]int64),
			IPCalls:         make(map[string]int64),
			LastResetTime:   time.Now(),
			LastCallDetails: make([]*models.CallDetail, 0, 100),
		}
	}

	stats := &Stats{
		Stats:            *statsData,
		callDetailBuffer: make([]*models.CallDetail, 0, 100),
		maxBufferSize:    100,
	}

	GlobalStats = stats

	log.Println("Stats initialized")

	go startPeriodicSave()
}

func (s *Stats) RecordCall(path, method, ip string, statusCode int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalCalls++
	s.MethodCalls[method]++
	s.PathCalls[path]++
	s.IPCalls[ip]++

	now := time.Now()

	if now.Day() != s.LastResetTime.Day() {
		s.DailyCalls = 1
		s.LastResetTime = now
	} else {
		s.DailyCalls++
	}

	detail := &models.CallDetail{
		Path:       path,
		Method:     method,
		IP:         ip,
		Timestamp:  now,
		StatusCode: statusCode,
	}

	if len(s.LastCallDetails) >= 100 {
		s.LastCallDetails = s.LastCallDetails[1:]
	}
	s.LastCallDetails = append(s.LastCallDetails, detail)

	s.bufferMutex.Lock()
	s.callDetailBuffer = append(s.callDetailBuffer, detail)
	bufferSize := len(s.callDetailBuffer)
	s.bufferMutex.Unlock()

	if bufferSize >= s.maxBufferSize {
		go s.flushCallDetailBuffer()
	}
}

func (s *Stats) flushCallDetailBuffer() {
	s.bufferMutex.Lock()

	if len(s.callDetailBuffer) == 0 {
		s.bufferMutex.Unlock()
		return
	}

	details := make([]*models.CallDetail, len(s.callDetailBuffer))
	copy(details, s.callDetailBuffer)
	s.callDetailBuffer = s.callDetailBuffer[:0]
	s.bufferMutex.Unlock()

	if err := db.SaveCallDetailsBatch(details); err != nil {
		log.Printf("Failed to batch save call details: %v", err)
	}
}

func (s *Stats) SaveStats() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statsCopy := s.GetStats()

	err := db.SaveStats(statsCopy)
	if err != nil {
		return err
	}

	s.flushCallDetailBuffer()

	return nil
}

func startPeriodicSave() {
	interval := 30 * time.Second

	log.Printf("Starting periodic stats save, interval: %v", interval)

	ticker := time.NewTicker(interval)
	defer func() {
		ticker.Stop()
		log.Println("Periodic stats save stopped")
	}()

	for range ticker.C {
		if GlobalStats == nil {
			log.Println("Stats instance not initialized, skipping save")
			continue
		}

		if err := GlobalStats.SaveStats(); err != nil {
			log.Printf("Periodic stats save failed: %v", err)
		}
	}
}

func (s *Stats) GetStats() *models.Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copy := &models.Stats{
		TotalCalls:      s.TotalCalls,
		DailyCalls:      s.DailyCalls,
		HourlyCalls:     s.HourlyCalls,
		MethodCalls:     make(map[string]int64),
		PathCalls:       make(map[string]int64),
		IPCalls:         make(map[string]int64),
		LastResetTime:   s.LastResetTime,
		LastCallDetails: make([]*models.CallDetail, len(s.LastCallDetails)),
	}

	for k, v := range s.MethodCalls {
		copy.MethodCalls[k] = v
	}
	for k, v := range s.PathCalls {
		copy.PathCalls[k] = v
	}
	for k, v := range s.IPCalls {
		copy.IPCalls[k] = v
	}

	for i, detail := range s.LastCallDetails {
		copy.LastCallDetails[i] = &models.CallDetail{
			Path:       detail.Path,
			Method:     detail.Method,
			IP:         detail.IP,
			Timestamp:  detail.Timestamp,
			StatusCode: detail.StatusCode,
		}
	}

	return copy
}

var templateFuncs = template.FuncMap{
	"percentage": func(total, count int64) int {
		if total == 0 {
			return 0
		}
		return int((float64(count) / float64(total)) * 100)
	},
}

func StatsHandler(c *gin.Context) {
	stats := GlobalStats.GetStats()

	c.HTML(http.StatusOK, "stats.html", gin.H{
		"Stats": stats,
		"Funcs": templateFuncs,
	})
}

func StatsAPIHandler(c *gin.Context) {
	stats := GlobalStats.GetStats()
	c.JSON(http.StatusOK, stats)
}

func APIKeyHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "api_key.html", gin.H{})
}
