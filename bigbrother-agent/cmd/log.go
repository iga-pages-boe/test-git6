package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"code.byted.org/gopkg/logs"
	"code.byted.org/gopkg/logs/provider"
	"code.byted.org/ti/bigbrother-agent/config"
)

const (
	LogLevel = logs.LevelInfo
)

func InitLogger() {
	// 初始化logs
	logger := logs.NewLogger(1024)
	logger.SetLevel(LogLevel)
	logger.SetCallDepth(3)
	logger.SetPSM(config.PSM())
	logger.AddProvider(provider.NewLogAgentProvider(config.PSM()))
	consoleProvider := logs.NewConsoleProvider()
	//consoleProvider.SetLevel(logs.LevelError)
	consoleProvider.SetLevel(logs.LevelTrace)
	if err := logger.AddProvider(consoleProvider); err != nil {
		fmt.Fprintf(os.Stderr, "AddProvider consoleProvider error: %s\n", err)
	}

	// 初始化 日志打印到文件
	filePath := filepath.Join(config.LogDir(), config.PSM()+".log")
	fileProvider := logs.NewFileProvider(filePath, logs.HourDur, -1)
	fileProvider.SetLevel(logs.LevelTrace)
	fileProvider.SetKeepFiles(24 * 7)
	if err := logger.AddProvider(fileProvider); err != nil {
		fmt.Fprintf(os.Stderr, "AddProvider fileProvider error: %s\n", err)
	}

	logs.InitLogger(logger)
}
