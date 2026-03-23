package middleware

import (
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
	"github.com/xrcuo/xrcuo-lib/config"
	"github.com/xrcuo/xrcuo-lib/db"
	"github.com/xrcuo/xrcuo-lib/models"
)

var apiKeyCacheInstance *cache.Cache

func init() {
	apiKeyCacheInstance = cache.New(5*time.Minute, 10*time.Minute)
}

func GetAPICache() *cache.Cache {
	return apiKeyCacheInstance
}

func RecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logrus.Errorf("Panic recovered: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "Internal server error",
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}

func RequestLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		c.Next()

		endTime := time.Now()
		latency := endTime.Sub(startTime)

		if config.GetInstance().GetConfig().Log.RequestLog {
			logrus.WithFields(logrus.Fields{
				"method":     c.Request.Method,
				"path":       c.Request.URL.Path,
				"status":     c.Writer.Status(),
				"client_ip":  c.ClientIP(),
				"latency":    latency,
				"latency_ms": latency.Milliseconds(),
				"size":       c.Writer.Size(),
				"timestamp":  endTime.Format(time.RFC3339),
			}).Info("API请求")
		}
	}
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}

		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")
		c.Writer.Header().Set("Access-Control-Max-Age", "3600")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Length, X-Response-Time")

		c.Writer.Header().Set("X-Frame-Options", "DENY")
		c.Writer.Header().Set("X-XSS-Protection", "1; mode=block")
		c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
		c.Writer.Header().Set("X-Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src * data:; font-src 'self' data:")
		c.Writer.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Writer.Header().Set("Permissions-Policy", "geolocation=(self), camera=(), microphone=(), payment=()")
		c.Writer.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Writer.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Writer.Header().Set("Pragma", "no-cache")
		c.Writer.Header().Set("Expires", "0")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

type APIKeyResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func ErrorResponse(c *gin.Context, statusCode int, code int, message string) {
	c.JSON(statusCode, APIKeyResponse{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}

func APIKeyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("Authorization")
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}

		if apiKey == "" {
			ErrorResponse(c, http.StatusUnauthorized, 401, "API密钥不能为空")
			c.Abort()
			return
		}

		var keyInfo *models.APIKey
		if val, found := apiKeyCacheInstance.Get(apiKey); found {
			keyInfo = val.(*models.APIKey)
		}

		if keyInfo == nil {
			var err error
			keyInfo, err = db.GetAPIKeyByKey(apiKey)
			if err != nil {
				ErrorResponse(c, http.StatusUnauthorized, 401, "无效的API密钥")
				c.Abort()
				return
			}
			apiKeyCacheInstance.Set(apiKey, keyInfo, cache.DefaultExpiration)
		}

		if !keyInfo.IsPermanent && keyInfo.CurrentUsage >= keyInfo.MaxUsage {
			ErrorResponse(c, http.StatusForbidden, 403, "API密钥已达到使用上限")
			c.Abort()
			return
		}

		if err := db.UpdateAPIKeyUsage(apiKey); err != nil {
			ErrorResponse(c, http.StatusInternalServerError, 500, "更新API密钥使用次数失败")
			c.Abort()
			return
		}

		keyInfo.CurrentUsage++
		apiKeyCacheInstance.Set(apiKey, keyInfo, cache.DefaultExpiration)

		c.Set("api_key", keyInfo)

		c.Next()
	}
}

type tokenBucket struct {
	capacity       float64
	rate           float64
	tokens         float64
	lastRefillTime time.Time
	mutex          sync.Mutex
}

func (tb *tokenBucket) take() bool {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefillTime).Seconds()

	newTokens := elapsed * tb.rate
	if newTokens > 0 {
		tb.tokens = math.Min(tb.tokens+newTokens, tb.capacity)
		tb.lastRefillTime = now
	}

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}

	return false
}

type rateLimiter struct {
	buckets         map[string]*tokenBucket
	mutex           sync.RWMutex
	capacity        float64
	rate            float64
	cleanupInterval time.Duration
	inactiveTimeout time.Duration
}

var globalRateLimiter *rateLimiter

func init() {
	globalRateLimiter = &rateLimiter{
		buckets:         make(map[string]*tokenBucket),
		capacity:        100,
		rate:            1.666,
		cleanupInterval: 10 * time.Minute,
		inactiveTimeout: 30 * time.Minute,
	}

	go func() {
		ticker := time.NewTicker(globalRateLimiter.cleanupInterval)
		defer ticker.Stop()

		for range ticker.C {
			globalRateLimiter.cleanupInactiveBuckets()
		}
	}()
}

func (rl *rateLimiter) Allow(key string) bool {
	rl.mutex.RLock()
	tb, exists := rl.buckets[key]
	rl.mutex.RUnlock()

	if !exists {
		rl.mutex.Lock()
		if tb, exists = rl.buckets[key]; !exists {
			tb = &tokenBucket{
				capacity:       rl.capacity,
				rate:           rl.rate,
				tokens:         rl.capacity,
				lastRefillTime: time.Now(),
			}
			rl.buckets[key] = tb
		}
		rl.mutex.Unlock()
	}

	return tb.take()
}

func (rl *rateLimiter) cleanupInactiveBuckets() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	for key, tb := range rl.buckets {
		tb.mutex.Lock()
		inactiveTime := now.Sub(tb.lastRefillTime)
		tb.mutex.Unlock()

		if inactiveTime > rl.inactiveTimeout {
			delete(rl.buckets, key)
		}
	}
}

func RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()

		if !globalRateLimiter.Allow(clientIP) {
			ErrorResponse(c, http.StatusTooManyRequests, 429, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}

		c.Next()
	}
}

type StatsCollector interface {
	RecordCall(path, method, clientIP string, statusCode int)
}

var GlobalStats StatsCollector

func SetStatsCollector(s StatsCollector) {
	GlobalStats = s
}

func StatsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		method := c.Request.Method
		clientIP := c.ClientIP()

		c.Next()

		statusCode := c.Writer.Status()

		if GlobalStats != nil {
			go GlobalStats.RecordCall(path, method, clientIP, statusCode)
		}
	}
}

type PerformanceMetrics struct {
	TotalRequests     int64                   `json:"total_requests"`
	TotalResponseTime time.Duration           `json:"total_response_time"`
	AvgResponseTime   time.Duration           `json:"avg_response_time"`
	MaxResponseTime   time.Duration           `json:"max_response_time"`
	MinResponseTime   time.Duration           `json:"min_response_time"`
	QPS               float64                 `json:"qps"`
	LastResetTime     time.Time               `json:"last_reset_time"`
	MethodStats       map[string]*MethodStats `json:"method_stats"`
	PathStats         map[string]*PathStats   `json:"path_stats"`
	StatusStats       map[int]*StatusStats    `json:"status_stats"`
}

type MethodStats struct {
	Count             int64         `json:"count"`
	TotalResponseTime time.Duration `json:"total_response_time"`
	AvgResponseTime   time.Duration `json:"avg_response_time"`
}

type PathStats struct {
	Count             int64         `json:"count"`
	TotalResponseTime time.Duration `json:"total_response_time"`
	AvgResponseTime   time.Duration `json:"avg_response_time"`
}

type StatusStats struct {
	Count             int64         `json:"count"`
	TotalResponseTime time.Duration `json:"total_response_time"`
	AvgResponseTime   time.Duration `json:"avg_response_time"`
}

var (
	performanceMetrics = &PerformanceMetrics{
		TotalRequests:     0,
		TotalResponseTime: 0,
		MaxResponseTime:   0,
		MinResponseTime:   time.Hour,
		LastResetTime:     time.Now(),
		MethodStats:       make(map[string]*MethodStats),
		PathStats:         make(map[string]*PathStats),
		StatusStats:       make(map[int]*StatusStats),
	}
	metricsMutex = &sync.RWMutex{}
)

func PerformanceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		c.Next()

		endTime := time.Now()
		latency := endTime.Sub(startTime)

		statusCode := c.Writer.Status()
		method := c.Request.Method
		path := c.Request.URL.Path

		metricsMutex.Lock()
		defer metricsMutex.Unlock()

		performanceMetrics.TotalRequests++
		performanceMetrics.TotalResponseTime += latency
		performanceMetrics.AvgResponseTime = performanceMetrics.TotalResponseTime / time.Duration(performanceMetrics.TotalRequests)

		if latency > performanceMetrics.MaxResponseTime {
			performanceMetrics.MaxResponseTime = latency
		}

		if latency < performanceMetrics.MinResponseTime {
			performanceMetrics.MinResponseTime = latency
		}

		elapsed := endTime.Sub(performanceMetrics.LastResetTime).Seconds()
		if elapsed > 0 {
			performanceMetrics.QPS = float64(performanceMetrics.TotalRequests) / elapsed
		}

		if _, exists := performanceMetrics.MethodStats[method]; !exists {
			performanceMetrics.MethodStats[method] = &MethodStats{
				Count:             0,
				TotalResponseTime: 0,
			}
		}
		methodStat := performanceMetrics.MethodStats[method]
		methodStat.Count++
		methodStat.TotalResponseTime += latency
		methodStat.AvgResponseTime = methodStat.TotalResponseTime / time.Duration(methodStat.Count)

		if _, exists := performanceMetrics.PathStats[path]; !exists {
			performanceMetrics.PathStats[path] = &PathStats{
				Count:             0,
				TotalResponseTime: 0,
			}
		}
		pathStat := performanceMetrics.PathStats[path]
		pathStat.Count++
		pathStat.TotalResponseTime += latency
		pathStat.AvgResponseTime = pathStat.TotalResponseTime / time.Duration(pathStat.Count)

		if _, exists := performanceMetrics.StatusStats[statusCode]; !exists {
			performanceMetrics.StatusStats[statusCode] = &StatusStats{
				Count:             0,
				TotalResponseTime: 0,
			}
		}
		statusStat := performanceMetrics.StatusStats[statusCode]
		statusStat.Count++
		statusStat.TotalResponseTime += latency
		statusStat.AvgResponseTime = statusStat.TotalResponseTime / time.Duration(statusStat.Count)

		c.Writer.Header().Set("X-Response-Time", latency.String())
	}
}

func GetPerformanceMetrics() *PerformanceMetrics {
	metricsMutex.RLock()
	defer metricsMutex.RUnlock()

	return &PerformanceMetrics{
		TotalRequests:     performanceMetrics.TotalRequests,
		TotalResponseTime: performanceMetrics.TotalResponseTime,
		AvgResponseTime:   performanceMetrics.AvgResponseTime,
		MaxResponseTime:   performanceMetrics.MaxResponseTime,
		MinResponseTime:   performanceMetrics.MinResponseTime,
		QPS:               performanceMetrics.QPS,
		LastResetTime:     performanceMetrics.LastResetTime,
		MethodStats:       performanceMetrics.MethodStats,
		PathStats:         performanceMetrics.PathStats,
		StatusStats:       performanceMetrics.StatusStats,
	}
}
