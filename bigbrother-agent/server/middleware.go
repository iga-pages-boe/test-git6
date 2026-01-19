package server

import (
	"context"
	"strconv"
	"strings"
	"time"

	"code.byted.org/gopkg/metrics"
	"code.byted.org/ti/bigbrother-agent/common/mmetrics"
	"code.byted.org/ti/bigbrother-server/pb"

	"code.byted.org/bytedoc/mongo-go-driver/bson"

	"code.byted.org/gopkg/ctxvalues"
	"code.byted.org/gopkg/logid"
	"google.golang.org/grpc/metadata"

	"code.byted.org/gopkg/logs"
	"google.golang.org/grpc"
)

func accessInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()
	method := "-"
	host := "-"
	psm := "-"
	code := "500"
	defer func() {
		mmetrics.EmitThroughput(
			metrics.Tag("method", method),
			metrics.Tag("psm", psm),
			metrics.Tag("host", host),
			metrics.Tag("code", code))
		mmetrics.EmitLatency(start,
			metrics.Tag("method", method),
			metrics.Tag("psm", psm),
			metrics.Tag("host", host),
			metrics.Tag("code", code))
	}()

	md, ok := metadata.FromIncomingContext(ctx)
	logidKey := strings.ToLower(ctxvalues.CTXKeyLogID)
	if ok && len(md[logidKey]) > 0 {
		ctx = ctxvalues.SetLogID(ctx, md[logidKey][0])
	} else {
		ctx = ctxvalues.SetLogID(ctx, logid.GenLogID())
	}
	method = getMethodName(info.FullMethod)
	if hosts, ok := md["host"]; ok {
		host = hosts[0]
	}
	if psms, ok := md["psm"]; ok {
		psm = psms[0]
	}
	logs.CtxInfo(ctx, "new request %s, host: %s, psm: %s, request %v", method, host, psm, req)

	// Calls the handler
	rsp, err := handler(ctx, req)
	if err != nil {
		logs.CtxError(ctx, "failed to do request, %v", err)
		code = "500"
	} else {
		code = getCodeFromResponse(ctx, rsp)
	}

	logs.CtxInfo(ctx, "response %v, code %s", rsp, code)
	return rsp, err
}

func getMethodName(fullName string) string {
	ss := strings.Split(fullName, "/")
	return ss[len(ss)-1]
}

type base struct {
	Base *pb.Base
}

func getCodeFromResponse(ctx context.Context, rsp interface{}) string {
	ret := "5000"
	data, err := bson.Marshal(rsp)
	if err != nil {
		logs.CtxError(ctx, "failed to marshal response, %v", err)
		return ret
	}
	b := base{}
	err = bson.Unmarshal(data, &b)
	if err != nil {
		logs.CtxError(ctx, "failed to marshal response back, %v", err)
		return ret
	}
	ret = strconv.FormatInt(int64(b.Base.Code), 10)
	return ret
}
