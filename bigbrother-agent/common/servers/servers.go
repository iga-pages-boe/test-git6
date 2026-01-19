package servers

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"code.byted.org/gopkg/env"
	"code.byted.org/gopkg/logs"
	"code.byted.org/ti/bigbrother-agent/common/cache"
	"code.byted.org/ti/bigbrother-server/pb"
	"code.byted.org/ti/bigbrother-server/pb/errcode"
	"code.byted.org/ti/bigbrother-server/typeutil"
	"code.byted.org/ti/dsaenv"

	"github.com/kr/pretty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

var servers map[string][]string

func init() {
	region := dsaenv.Region()
	menv := env.Env()
	fmt.Println("region", region, "env", menv)
	if region == "BOE" {
		servers = map[string][]string{
			"tob": []string{"ti-bigbrother-tob.byted.org:443"},
		}
	} else if region == "LFTB" {
		servers = map[string][]string{
			"tob": []string{"ti-bigbrother-tob.byted.org:443"},
		}
	} else if region != "" && region != "-" { // online
		servers = map[string][]string{
			"tob": []string{"ti-bigbrother-tob.byted.org:443"},
		}
	} else { // DEV
		servers = map[string][]string{
			"tob": []string{"ti-bigbrother-tob.byted.org:443"},
		}
	}
	pretty.Println(servers)
}

func Servers() map[string][]string {
	return servers
}

func PolluteCacheFromServers(ctx context.Context, cache *cache.Cache, group, key string) (noSuchKey bool) {
	var wg sync.WaitGroup
	wg.Add(len(servers))
	var notFound int64
	for idc, ss := range servers {
		go func(idc string, servers []string) {
			defer wg.Done()
			for i := range servers {
				server := servers[i]
				logs.CtxWarn(ctx, "[bigbrother] fetching from %s, %s", idc, server)
				var opts []grpc.DialOption
				if strings.Contains(server, ":443") {
					cred := credentials.NewTLS(&tls.Config{
						InsecureSkipVerify: true,
					})
					opts = append(opts, grpc.WithTransportCredentials(cred))
				} else {
					opts = append(opts, grpc.WithInsecure())
				}
				conn, err := grpc.Dial(server, opts...)
				if conn != nil {
					defer conn.Close()
				}
				if err != nil {
					logs.CtxError(ctx, "[bigbrother] failed to connect to %s, %v", server, err)
					continue
				}
				pb.NewKVServiceClient(conn)
				cli := pb.NewKVServiceClient(conn)
				ctx := ctx // make of copy of original ctx
				if idc == "RD" {
					ctx = metadata.AppendToOutgoingContext(ctx, "X-TT-ENV", "boe_test")
				}
				rsp, err := cli.Get(ctx, &pb.GetRequest{
					Group:      group,
					Key:        key,
					AllHistory: false,
				})
				if err != nil {
					logs.CtxError(ctx, "[bigbrother] failed to do grpc request, server %s, %v", servers[i], err)
					continue
				}
				err = errcode.GetError(rsp.Base)
				if err == errcode.NotFound {
					atomic.AddInt64(&notFound, 1)
					return
				}
				if err != errcode.OK {
					logs.CtxError(ctx, "[bigbrother] server %s returned error %v", servers[i], err)
					continue
				}

				if len(rsp.Entries) == 0 {
					logs.CtxError(ctx, "[bigbrother] server %s internal error %v", servers[i], err)
					continue
				}

				cache.Put(group, key, typeutil.ConvertPbEntryToTypes(rsp.Entries[0]))
				return
			}
		}(idc, ss)
	}
	wg.Wait()
	return atomic.LoadInt64(&notFound) == int64(len(servers))
}
