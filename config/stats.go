package config

import (
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// 配置默认值常量
const (
	defaultConfigPath     = "web.yaml"
	defaultServerPort     = ":8080"
	defaultServerMode     = "debug"
	defaultLogLevel       = "info"
	defaultLogFile        = "logs/app.log"
	defaultLogMaxSize     = 10
	defaultLogMaxBackups  = 5
	defaultLogMaxAge      = 7
	defaultDBType         = "sqlite"
	defaultDBPath         = "./data/stats.db"
	defaultDBHost         = "localhost"
	defaultDBName         = "xrcuo_api"
	defaultDBMaxOpenConns = 10
	defaultDBMaxIdleConns = 5
	defaultRateLimitCap   = 500
	defaultRateLimitRate  = 10
	defaultSiteTitle      = "YVLPYY｜二次元の小窝"
	defaultSiteName       = "林熙"
	defaultSiteMotto      = "人海中遇见的人终将归还人海"
	defaultSiteAvatar     = "https://yilx.net/tx.jpg"
	defaultSiteICP        = "沪ICP备1234567890号-1"
	defaultSiteCopyright  = "© 2026 YVLPYY. All rights reserved."
	defaultSiteBlogLink   = "https://blog.yilx.net/"
	defaultSiteCDKLink    = "https://cdk.yilx.net"
	defaultSiteAPILink    = "https://api.yilx.net"
	defaultSiteAboutLink  = "https://blog.yilx.net/about.html"
	defaultSiteGitHub     = "https://github.com/"
	defaultSiteZhihu      = "https://www.zhihu.com/"
	defaultSiteWeibo      = "https://weibo.com/"
	defaultSiteEmail      = "https://mail.qq.com/"
	watchDebounceDuration = 100 * time.Millisecond
)

// Config 主配置结构
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Log       LogConfig       `yaml:"log"`
	Database  DatabaseConfig  `yaml:"database"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Site      SiteConfig      `yaml:"site"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port       string `yaml:"port"`
	Mode       string `yaml:"mode"`
	JSONFormat struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"json_format"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level            string `yaml:"level"`
	File             string `yaml:"file"`
	ConsoleOutput    bool   `yaml:"console_output"`
	RequestLog       bool   `yaml:"request_log"`
	MaxSize          int    `yaml:"max_size"`
	MaxBackups       int    `yaml:"max_backups"`
	MaxAge           int    `yaml:"max_age"`
	Compress         bool   `yaml:"compress"`
	NewFileOnStartup bool   `yaml:"new_file_on_startup"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Type         string `yaml:"type"`
	Path         string `yaml:"path"`
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	User         string `yaml:"user"`
	Password     string `yaml:"password"`
	DBName       string `yaml:"dbname"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Capacity float64 `yaml:"capacity"`
	Rate     float64 `yaml:"rate"`
}

// SiteConfig 网站配置
type SiteConfig struct {
	Title     string      `yaml:"title"`
	Name      string      `yaml:"name"`
	Motto     string      `yaml:"motto"`
	AvatarURL string      `yaml:"avatar_url"`
	ICP       string      `yaml:"icp"`
	Copyright string      `yaml:"copyright"`
	Links     SiteLinks   `yaml:"links"`
	Contact   SiteContact `yaml:"contact"`
}

// SiteLinks 网站链接
type SiteLinks struct {
	Blog  string `yaml:"blog"`
	CDK   string `yaml:"cdk"`
	API   string `yaml:"api"`
	About string `yaml:"about"`
}

// SiteContact 联系方式
type SiteContact struct {
	GitHub string `yaml:"github"`
	Zhihu  string `yaml:"zhihu"`
	Weibo  string `yaml:"weibo"`
	Email  string `yaml:"email"`
}

// ConfigUpdateCallback 配置更新回调函数类型
type ConfigUpdateCallback func(*Config)

// ConfigManager 配置管理器
type ConfigManager struct {
	config          *Config
	configPath      string
	mutex           sync.RWMutex
	watcher         *fsnotify.Watcher
	stopChan        chan struct{}
	isWatching      bool
	updateCallbacks []ConfigUpdateCallback
	callbacksMutex  sync.Mutex
	debounceTimer   *time.Timer
	debounceMutex   sync.Mutex
}
