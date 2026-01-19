package server

import (
	"runtime"
	"strconv"

	"code.byted.org/gopkg/logs"
	"code.byted.org/ti/bigbrother-agent/common/sdk"
)

const MaxProcsKey = "maxproc"
const DefaultProcCount = "4"

func WatchMaxProc() {
	cli, _ := sdk.NewClient("bigbrother_agent")
	cli.AddListenerWithValue(MaxProcsKey, func(value string) sdk.CallbackRet {
		logs.Warn("GOMAXPROCS set to %s", value)
		ret := sdk.CallbackRet{}
		c, err := strconv.Atoi(value)
		if err != nil {
			logs.Error("%s is not an integer, %v", value, err)
			return ret
		}
		runtime.GOMAXPROCS(c)
		return ret
	}, DefaultProcCount)
}
