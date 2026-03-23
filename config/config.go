package config

import (
	"log"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port       string `yaml:"port"`
		Mode       string `yaml:"mode"`
		JSONFormat struct {
			Enabled bool `yaml:"enabled"`
		} `yaml:"json_format"`
	} `yaml:"server"`

	Database struct {
		Path         string `yaml:"path"`
		MaxOpenConns int    `yaml:"max_open_conns"`
		MaxIdleConns int    `yaml:"max_idle_conns"`
	} `yaml:"database"`

	IP2Region struct {
		V4DBPath string `yaml:"v4_db_path"`
		V6DBPath string `yaml:"v6_db_path"`
	} `yaml:"ip2region"`

	Log struct {
		Level         string `yaml:"level"`
		File          string `yaml:"file"`
		ConsoleOutput bool   `yaml:"console_output"`
		RequestLog    bool   `yaml:"request_log"`
		MaxSize       int    `yaml:"max_size"`
		MaxBackups    int    `yaml:"max_backups"`
		MaxAge        int    `yaml:"max_age"`
	} `yaml:"log"`

	RandomImage struct {
		LocalEnabled bool   `yaml:"local_enabled"`
		LocalPath    string `yaml:"local_path"`
	} `yaml:"random_image"`

	Download struct {
		Path string `yaml:"path"`
	} `yaml:"download"`
}

type ConfigUpdateCallback func(*Config)

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

var (
	instance *ConfigManager
	once     sync.Once
)

func GetInstance() *ConfigManager {
	once.Do(func() {
		instance = &ConfigManager{
			stopChan: make(chan struct{}),
		}
	})
	return instance
}

func (cm *ConfigManager) GetConfig() *Config {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	return cm.config
}

func (cm *ConfigManager) SetConfig(config *Config) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.config = config
}

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

func (cm *ConfigManager) ParseConfig() {
	cm.configPath = cm.getConfigPath()

	logrus.Debugf("Parsing config file: %s", cm.configPath)

	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		if err = cm.genConfig(); err != nil {
			logrus.Fatalf("Failed to generate config file: %s", cm.configPath)
		}
		logrus.Warnf("Config file not found, generated default: %s", cm.configPath)
		logrus.Warn("Exiting in 5 seconds...")
		os.Exit(1)
	}

	content, err := os.ReadFile(cm.configPath)
	if err != nil {
		logrus.Fatalf("Failed to read config file: %v", err)
	}

	config := &Config{}
	err = yaml.Unmarshal(content, config)
	if err != nil {
		logrus.Fatal("Failed to parse config.yaml")
	}

	cm.validateConfig(config)

	isUpdate := cm.config != nil

	cm.SetConfig(config)

	if isUpdate {
		logrus.Info("Applying updated config...")
		cm.executeUpdateCallbacks(config)
		log.Println("Config update applied")
	}
}

func (cm *ConfigManager) validateConfig(config *Config) {
	validModes := map[string]bool{"debug": true, "release": true, "test": true}
	if !validModes[config.Server.Mode] {
		logrus.Warnf("Invalid Gin mode: %s, using default: debug", config.Server.Mode)
		config.Server.Mode = "debug"
	}

	if config.IP2Region.V4DBPath == "" && config.IP2Region.V6DBPath == "" {
		logrus.Warn("Using default IP2Region config")
		config.IP2Region.V4DBPath = "./ip2region_v4.xdb"
		config.IP2Region.V6DBPath = "./ip2region_v6.xdb"
	}

	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true, "fatal": true, "panic": true}
	if !validLogLevels[config.Log.Level] {
		logrus.Warnf("Invalid log level: %s, using default: info", config.Log.Level)
		config.Log.Level = "info"
	}

	if config.Log.MaxSize <= 0 {
		config.Log.MaxSize = 10
	}
	if config.Log.MaxBackups <= 0 {
		config.Log.MaxBackups = 5
	}
	if config.Log.MaxAge <= 0 {
		config.Log.MaxAge = 7
	}

	logrus.Debug("Config validation complete")
}

func (cm *ConfigManager) WatchConfig() {
	if cm.isWatching {
		return
	}

	var err error
	cm.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		logrus.Errorf("Failed to create config watcher: %v", err)
		return
	}

	if err := cm.watcher.Add(cm.configPath); err != nil {
		logrus.Errorf("Failed to add config file to watcher: %v", err)
		cm.watcher.Close()
		return
	}

	cm.isWatching = true

	go func() {
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
					logrus.Info("Config file changed, reloading...")

					cm.debounceMutex.Lock()
					if cm.debounceTimer != nil {
						cm.debounceTimer.Stop()
					}
					cm.debounceTimer = time.AfterFunc(100*time.Millisecond, func() {
						cm.ParseConfig()
					})
					cm.debounceMutex.Unlock()
				}
			case err, ok := <-cm.watcher.Errors:
				if !ok {
					return
				}
				logrus.Errorf("Config watcher error: %v", err)
			case <-cm.stopChan:
				return
			}
		}
	}()

	logrus.Info("Config watcher started")
}

func (cm *ConfigManager) StopWatching() {
	if !cm.isWatching {
		return
	}

	cm.stopChan <- struct{}{}
	cm.isWatching = false
	logrus.Info("Config watcher stopped")
}

func (cm *ConfigManager) getConfigPath() string {
	if path := os.Getenv("CONFIG_FILE_PATH"); path != "" {
		return path
	}
	return "config.yaml"
}

func (cm *ConfigManager) genConfig() error {
	logrus.Debugf("Generating config file: %s", cm.configPath)
	return os.WriteFile(cm.configPath, []byte(defConfig), 0644)
}

func Parse() {
	GetInstance().ParseConfig()
}

func GetServerPort() string {
	cm := GetInstance()
	config := cm.GetConfig()
	if config == nil || config.Server.Port == "" {
		return ":8080"
	}
	return config.Server.Port
}

func GetServerMode() string {
	cm := GetInstance()
	config := cm.GetConfig()
	if config == nil || config.Server.Mode == "" {
		return "debug"
	}
	return config.Server.Mode
}

func IsJSONFormatEnabled() bool {
	cm := GetInstance()
	config := cm.GetConfig()
	if config == nil {
		return false
	}
	return config.Server.JSONFormat.Enabled
}

func GetDatabasePath() string {
	cm := GetInstance()
	config := cm.GetConfig()
	if config == nil || config.Database.Path == "" {
		return "./stats.db"
	}
	return config.Database.Path
}

func GetMaxOpenConns() int {
	cm := GetInstance()
	config := cm.GetConfig()
	if config == nil || config.Database.MaxOpenConns <= 0 {
		return 10
	}
	return config.Database.MaxOpenConns
}

func GetMaxIdleConns() int {
	cm := GetInstance()
	config := cm.GetConfig()
	if config == nil || config.Database.MaxIdleConns <= 0 {
		return 5
	}
	return config.Database.MaxIdleConns
}

func GetIP2RegionV4DBPath() string {
	cm := GetInstance()
	config := cm.GetConfig()
	if config == nil || config.IP2Region.V4DBPath == "" {
		return "./ip2region_v4.xdb"
	}
	return config.IP2Region.V4DBPath
}

func GetIP2RegionV6DBPath() string {
	cm := GetInstance()
	config := cm.GetConfig()
	if config == nil || config.IP2Region.V6DBPath == "" {
		return "./ip2region_v6.xdb"
	}
	return config.IP2Region.V6DBPath
}

func GetLogLevel() string {
	cm := GetInstance()
	config := cm.GetConfig()
	if config == nil || config.Log.Level == "" {
		return "info"
	}
	return config.Log.Level
}

func GetDownloadPath() string {
	cm := GetInstance()
	config := cm.GetConfig()
	if config == nil || config.Download.Path == "" {
		return "./downloads"
	}
	downloadPath := config.Download.Path
	if err := os.MkdirAll(downloadPath, 0755); err != nil {
		logrus.Warnf("Failed to create download directory: %v, using default", err)
		return "./downloads"
	}
	return downloadPath
}
