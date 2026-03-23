package log

import (
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/xrcuo/xrcuo-lib/config"
)

func InitLogger() {
	cfg := config.GetInstance().GetConfig()

	level, err := logrus.ParseLevel(cfg.Log.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logrus.SetLevel(level)

	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	if cfg.Log.File != "" {
		logDir := filepath.Dir(cfg.Log.File)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			logrus.Warnf("Failed to create log directory: %v, logging to console only", err)
		} else {
			file, err := os.OpenFile(cfg.Log.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err == nil {
				logrus.SetOutput(file)
			}
		}
	}

	if cfg.Log.ConsoleOutput {
		logrus.SetOutput(os.Stdout)
	}
}

func GetLogger() *logrus.Logger {
	return logrus.StandardLogger()
}
