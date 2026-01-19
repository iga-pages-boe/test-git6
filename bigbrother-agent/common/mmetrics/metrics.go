package mmetrics

import (
	"time"

	"code.byted.org/gopkg/env"
	"code.byted.org/ti/dsaenv"

	"code.byted.org/gopkg/logs"
	"code.byted.org/gopkg/metrics"
)

var metricsClient *metrics.MetricsClientV2

const myPSM = "data.ti.bigbrother_agent"

var menvTag = metrics.T{"dsaenv", "-"}

func init() {
	metricsClient = metrics.NewDefaultMetricsClientV2(myPSM, true)
	region := dsaenv.Region()
	menv := env.Env()
	if region == "BOE" {
		if menv == "test" { // RD
			menv = "RD"
		} else { // QA
			menv = "QA"
		}
	} else if region != "" && region != "-" { // online
		menv = "Online"
	} else { // DEV
		menv = "-"
	}
	menvTag = metrics.T{"dsaenv", menv}
}

func EmitThroughput(tags ...metrics.T) {
	tags = append(tags, menvTag)
	if err := metricsClient.EmitCounter("throughput", 1, tags...); err != nil {
		logs.Warn("EmitCounter Error: %v", err.Error())
	}
}

func EmitLatency(start time.Time, tags ...metrics.T) {
	tags = append(tags, menvTag)
	cost := time.Since(start).Nanoseconds() / 1000
	if err := metricsClient.EmitTimer("latency.us", cost, tags...); err != nil {
		logs.Warn("EmitTimer Error: %v", err.Error())
	}
}

func EmitStore(value interface{}, tags ...metrics.T) {
	tags = append(tags, menvTag)
	if err := metricsClient.EmitStore("store", value, tags...); err != nil {
		logs.Warn("EmitStore Error: %v", err)
	}
}

func EmitError(name string, tags ...metrics.T) {
	tags = append(tags, menvTag)
	if err := metricsClient.EmitCounter(name+".error", 1, tags...); err != nil {
		logs.Warn("EmitCounter %s Error: %v", name, err)
	}
}
