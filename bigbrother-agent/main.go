package main

import (
	"os"

	"code.byted.org/ti/bigbrother-agent/cmd"

	"code.byted.org/gopkg/logs"
)

func main() {
	app := cmd.NewApp()

	logs.Error("failed to start agent: %v", app.Run(os.Args))
	logs.Stop()
}
