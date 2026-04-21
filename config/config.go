package config

import (
	_ "embed"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

//go:embed default_config.yaml
var defaultConfigYAML string

var (
	instance *ConfigManager
	once     sync.Once
)

// GetInstance 获取配置管理器单例
func GetInstance() *ConfigManager {
	once.Do(func() {
		instance = &ConfigManager{
			stopChan: make(chan struct{}, 1),
		}
	})
	return instance
}

// GetConfig 获取当前配置
func (cm *ConfigManager) GetConfig() *Config {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	return cm.config
}

// SetConfig 设置配置
func (cm *ConfigManager) SetConfig(config *Config) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.config = config
}

func (cm *ConfigManager) generateDefaultConfig() error {
	logrus.Debugf("正在生成配置文件: %s", cm.configPath)
	return os.WriteFile(cm.configPath, []byte(defaultConfigYAML), 0644)
}

func (cm *ConfigManager) getConfigFilePath() string {
	if path := os.Getenv("CONFIG_FILE_PATH"); path != "" {
		return path
	}
	return defaultConfigPath
}

// Parse 解析配置（全局入口）
func Parse() {
	GetInstance().ParseConfig()
}

// RegisterUpdateCallback 注册配置更新回调
func (cm *ConfigManager) RegisterUpdateCallback(callback ConfigUpdateCallback) {
	cm.callbacksMutex.Lock()
	defer cm.callbacksMutex.Unlock()
	cm.updateCallbacks = append(cm.updateCallbacks, callback)
}

func (cm *ConfigManager) executeUpdateCallbacks(config *Config) {
	cm.callbacksMutex.Lock()
	callbacks := make([]ConfigUpdateCallback, len(cm.updateCallbacks))
	copy(callbacks, cm.updateCallbacks)
	cm.callbacksMutex.Unlock()

	for _, callback := range callbacks {
		callback(config)
	}
}

// ParseConfig 解析配置文件
func (cm *ConfigManager) ParseConfig() {
	cm.configPath = cm.getConfigFilePath()
	logrus.Debugf("正在解析配置文件: %s", cm.configPath)

	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		if err := cm.generateDefaultConfig(); err != nil {
			logrus.Fatalf("无法生成设置文件: %s, 请确认是否给足系统权限", cm.configPath)
		}
		logrus.Warnf("未检测到 %s，已自动生成，请配置并重新启动", cm.configPath)
		logrus.Warn("将于 5 秒后退出...")
		os.Exit(1)
	}

	content, err := os.ReadFile(cm.configPath)
	if err != nil {
		logrus.Fatalf("读取配置文件失败: %v", err)
	}

	config := &Config{}
	if err := yaml.Unmarshal(content, config); err != nil {
		logrus.Fatal("解析配置文件失败，请检查格式、内容是否输入正确")
	}

	cm.validateAndSetDefaults(config)
	isUpdate := cm.config != nil
	cm.SetConfig(config)

	if isUpdate {
		logrus.Info("正在应用更新后的配置...")
		gin.SetMode(cm.GetConfig().Server.Mode)
		logrus.Infof("Gin模式已更新为: %s", cm.GetConfig().Server.Mode)
		cm.executeUpdateCallbacks(config)
		logrus.Info("配置更新应用完成")
	}
}

func (cm *ConfigManager) validateAndSetDefaults(config *Config) {
	validModes := map[string]bool{"debug": true, "release": true, "test": true}
	if !validModes[config.Server.Mode] {
		logrus.Warnf("无效的Gin模式: %s, 使用默认模式: %s", config.Server.Mode, defaultServerMode)
		config.Server.Mode = defaultServerMode
	}

	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true, "fatal": true, "panic": true}
	if !validLogLevels[config.Log.Level] {
		logrus.Warnf("无效的日志级别: %s, 使用默认级别: %s", config.Log.Level, defaultLogLevel)
		config.Log.Level = defaultLogLevel
	}

	setStringDefault(&config.Log.File, defaultLogFile)

	if config.Log.MaxSize <= 0 {
		logrus.Warnf("无效的日志文件大小: %d, 使用默认值: %d MB", config.Log.MaxSize, defaultLogMaxSize)
		config.Log.MaxSize = defaultLogMaxSize
	}
	if config.Log.MaxBackups <= 0 {
		logrus.Warnf("无效的日志文件保留数量: %d, 使用默认值: %d", config.Log.MaxBackups, defaultLogMaxBackups)
		config.Log.MaxBackups = defaultLogMaxBackups
	}
	if config.Log.MaxAge <= 0 {
		logrus.Warnf("无效的日志文件保留天数: %d, 使用默认值: %d", config.Log.MaxAge, defaultLogMaxAge)
		config.Log.MaxAge = defaultLogMaxAge
	}

	if config.RateLimit.Capacity <= 0 {
		logrus.Warnf("无效的速率限制容量: %f, 使用默认值: %d", config.RateLimit.Capacity, defaultRateLimitCap)
		config.RateLimit.Capacity = defaultRateLimitCap
	}
	if config.RateLimit.Rate <= 0 {
		logrus.Warnf("无效的速率限制速率: %f, 使用默认值: %d", config.RateLimit.Rate, defaultRateLimitRate)
		config.RateLimit.Rate = defaultRateLimitRate
	}

	setStringDefault(&config.Site.Title, defaultSiteTitle)
	setStringDefault(&config.Site.Name, defaultSiteName)
	setStringDefault(&config.Site.Motto, defaultSiteMotto)
	setStringDefault(&config.Site.AvatarURL, defaultSiteAvatar)
	setStringDefault(&config.Site.ICP, defaultSiteICP)
	setStringDefault(&config.Site.Copyright, defaultSiteCopyright)
	setStringDefault(&config.Site.Links.Blog, defaultSiteBlogLink)
	setStringDefault(&config.Site.Links.CDK, defaultSiteCDKLink)
	setStringDefault(&config.Site.Links.API, defaultSiteAPILink)
	setStringDefault(&config.Site.Links.About, defaultSiteAboutLink)
	setStringDefault(&config.Site.Contact.GitHub, defaultSiteGitHub)
	setStringDefault(&config.Site.Contact.Zhihu, defaultSiteZhihu)
	setStringDefault(&config.Site.Contact.Weibo, defaultSiteWeibo)
	setStringDefault(&config.Site.Contact.Email, defaultSiteEmail)

	logrus.Debug("配置验证完成")
}

func setStringDefault(field *string, defaultValue string) {
	if *field == "" {
		*field = defaultValue
	}
}

// WatchConfig 监听配置文件变化
func (cm *ConfigManager) WatchConfig() {
	if cm.isWatching {
		return
	}

	var err error
	cm.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		logrus.Errorf("创建配置文件监听器失败: %v", err)
		return
	}

	if err := cm.watcher.Add(cm.configPath); err != nil {
		logrus.Errorf("添加配置文件到监听器失败: %v", err)
		cm.watcher.Close()
		return
	}

	cm.isWatching = true

	go cm.watchLoop()
	logrus.Info("配置文件监听已启动")
}

func (cm *ConfigManager) watchLoop() {
	defer func() {
		cm.watcher.Close()
		cm.isWatching = false
		cm.debounceMutex.Lock()
		if cm.debounceTimer != nil {
			cm.debounceTimer.Stop()
		}
		cm.debounceMutex.Unlock()
	}()

	for {
		select {
		case event, ok := <-cm.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				logrus.Info("配置文件发生变化，重新加载配置")
				cm.debounceMutex.Lock()
				if cm.debounceTimer != nil {
					cm.debounceTimer.Stop()
				}
				cm.debounceTimer = time.AfterFunc(watchDebounceDuration, func() {
					cm.ParseConfig()
				})
				cm.debounceMutex.Unlock()
			}
		case err, ok := <-cm.watcher.Errors:
			if !ok {
				return
			}
			logrus.Errorf("配置文件监听错误: %v", err)
		case <-cm.stopChan:
			return
		}
	}
}

// StopWatching 停止监听配置文件
func (cm *ConfigManager) StopWatching() {
	if !cm.isWatching {
		return
	}
	select {
	case cm.stopChan <- struct{}{}:
	default:
	}
	cm.isWatching = false
	logrus.Info("配置文件监听已停止")
}

// 便捷访问函数
func GetServerPort() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil && c.Server.Port != "" {
		return c.Server.Port
	}
	return defaultServerPort
}

func GetServerMode() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil && c.Server.Mode != "" {
		return c.Server.Mode
	}
	return defaultServerMode
}

func IsJSONFormatEnabled() bool {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil {
		return c.Server.JSONFormat.Enabled
	}
	return false
}

func GetDatabaseType() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil && c.Database.Type != "" {
		return c.Database.Type
	}
	return defaultDBType
}

func GetDatabasePath() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil && c.Database.Path != "" {
		return c.Database.Path
	}
	return defaultDBPath
}

func GetDatabaseHost() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil && c.Database.Host != "" {
		return c.Database.Host
	}
	return defaultDBHost
}

func GetDatabasePort() int {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil && c.Database.Port > 0 {
		return c.Database.Port
	}
	switch GetDatabaseType() {
	case "mysql":
		return 3306
	case "postgresql":
		return 5432
	}
	return 0
}

func GetDatabaseUser() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil {
		return c.Database.User
	}
	return ""
}

func GetDatabasePassword() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil {
		return c.Database.Password
	}
	return ""
}

func GetDatabaseName() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil && c.Database.DBName != "" {
		return c.Database.DBName
	}
	return defaultDBName
}

func GetMaxOpenConns() int {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil && c.Database.MaxOpenConns > 0 {
		return c.Database.MaxOpenConns
	}
	return defaultDBMaxOpenConns
}

func GetMaxIdleConns() int {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil && c.Database.MaxIdleConns > 0 {
		return c.Database.MaxIdleConns
	}
	return defaultDBMaxIdleConns
}

func GetLogLevel() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil && c.Log.Level != "" {
		return c.Log.Level
	}
	return defaultLogLevel
}

func GetLogFile() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil && c.Log.File != "" {
		return c.Log.File
	}
	return defaultLogFile
}

func GetRateLimitCapacity() float64 {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil && c.RateLimit.Capacity > 0 {
		return c.RateLimit.Capacity
	}
	return defaultRateLimitCap
}

func GetRateLimitRate() float64 {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil && c.RateLimit.Rate > 0 {
		return c.RateLimit.Rate
	}
	return defaultRateLimitRate
}

func GetSiteTitle() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil {
		return c.Site.Title
	}
	return defaultSiteTitle
}

func GetSiteName() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil {
		return c.Site.Name
	}
	return defaultSiteName
}

func GetSiteMotto() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil {
		return c.Site.Motto
	}
	return defaultSiteMotto
}

func GetSiteAvatarURL() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil {
		return c.Site.AvatarURL
	}
	return defaultSiteAvatar
}

func GetSiteICP() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil {
		return c.Site.ICP
	}
	return defaultSiteICP
}

func GetSiteCopyright() string {
	cm := GetInstance()
	if c := cm.GetConfig(); c != nil {
		return c.Site.Copyright
	}
	return defaultSiteCopyright
}

func GetSiteLinks() SiteLinks {
	cm := GetInstance()
	defaults := SiteLinks{
		Blog:  defaultSiteBlogLink,
		CDK:   defaultSiteCDKLink,
		API:   defaultSiteAPILink,
		About: defaultSiteAboutLink,
	}
	if c := cm.GetConfig(); c != nil {
		return c.Site.Links
	}
	return defaults
}

func GetSiteContact() SiteContact {
	cm := GetInstance()
	defaults := SiteContact{
		GitHub: defaultSiteGitHub,
		Zhihu:  defaultSiteZhihu,
		Weibo:  defaultSiteWeibo,
		Email:  defaultSiteEmail,
	}
	if c := cm.GetConfig(); c != nil {
		return c.Site.Contact
	}
	return defaults
}
