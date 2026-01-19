package cmd

import (
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"code.byted.org/ti/bigbrother-agent/config"
	"code.byted.org/ti/bigbrother-agent/server"

	"code.byted.org/gopkg/logs"
	"github.com/urfave/cli/v2"
)

const (
	AppName = "dsa_bigbrother_agent"
)

var (
	GitSHA    string
	BuildTime string
)

var (
	command = cli.Command{
		Name:   "dsa_bigbrother_agent",
		Usage:  "通用边缘降级平台",
		Flags:  GlobalFlags,
		Action: Main,
	}
)

var GlobalFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "conf-dir",
		Value: "conf",
		Usage: "path of config file, default is config.yaml",
	},
	&cli.StringFlag{
		Name:  "psm",
		Value: "data.ti.bigbrother_agent",
		Usage: "psm",
	},
	&cli.StringFlag{
		Name:  "log-dir",
		Value: "/opt/dsa/bigbrother_agent/log",
		Usage: "logdir",
	},
}

func Main(ctx *cli.Context) error {
	config.SetPSM(ctx.String("psm"))
	config.SetLogDir(ctx.String("log-dir"))
	InitLogger()
	defer func() {
		if e := recover(); e != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			logs.Error("RecoveryCatcher error=%s panic stack info=%s", e, string(buf))
		}
		logs.Stop()
	}()
	c := SetupSignalHandler()

	s, err := server.NewServer()
	if err != nil {
		logs.Fatal("failed to create agent")
		os.Exit(1)
	}
	err = s.Start(c)
	if err != nil {
		logs.Error("failed to start agent %v", err)
	}
	return err
}

func NewApp() *cli.App {
	app := cli.NewApp()
	app.Name = AppName
	app.Version = GitSHA + "-" + BuildTime
	app.Flags = GlobalFlags

	commands := []*cli.Command{&command}
	app.Commands = commands
	return app
}

func SetupSignalHandler() (stopCh <-chan struct{}) {
	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-c
		close(stop)
		<-c
		os.Exit(1) // second signal. Exit directly.
	}()
	return stop
}
