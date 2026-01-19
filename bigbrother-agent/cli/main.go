package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"code.byted.org/ti/bigbrother-server/typeutil"

	"code.byted.org/ti/bigbrother-server/pb"
	"code.byted.org/ti/bigbrother-server/pb/errcode"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var usage = `Usage:
cli list
	-- 显示agent当前缓存的所有键值
cli get <group>/<key>
	-- 显示agent缓存的值
cli get-remote <group>/<key>
    -- 模拟SDK的Get行为
`

func printHelpAndExit() {
	fmt.Println(usage)
	os.Exit(1)
}

const AgentAddr = "127.0.0.1:2310"

func getPBCli() pb.KVServiceClient {
	conn, err := grpc.Dial(AgentAddr, grpc.WithInsecure())
	if err != nil { // unlikely
		fmt.Println("failed to connect to agent,", err)
		os.Exit(1)
	}
	cli := pb.NewKVServiceClient(conn)
	return cli
}

func main() {
	args := os.Args
	if len(args) < 2 {
		printHelpAndExit()
	}
	switch args[1] {
	case "list":
		if len(args) != 2 {
			printHelpAndExit()
		}
		ctx := metadata.AppendToOutgoingContext(context.Background(), "from_agent_cli", "true")
		rsp, err := getPBCli().ListKeys(ctx, &pb.ListKeysRequest{})
		if err != nil {
			fmt.Println("failed to do grpc request,", err)
			os.Exit(1)
		}
		err = errcode.GetError(rsp.Base)
		if err != errcode.OK {
			fmt.Println(err)
			os.Exit(1)
		}
		sort.Slice(rsp.Entries, func(i, j int) bool {
			if rsp.Entries[i].Group < rsp.Entries[j].Group {
				return true
			}
			if rsp.Entries[i].Group == rsp.Entries[j].Group && rsp.Entries[i].Key < rsp.Entries[j].Key {
				return true
			}
			return false
		})
		for i := range rsp.Entries {
			fmt.Printf("%s/%s:%s\n", rsp.Entries[i].Group, rsp.Entries[i].Key, rsp.Entries[i].Value)
		}
	case "get", "get-remote":
		if len(args) != 3 {
			printHelpAndExit()
		}
		ctx := context.Background()
		if args[1] == "get" {
			ctx = metadata.AppendToOutgoingContext(ctx, "from_agent_cli", "true")
		}
		key := args[2]
		ss := strings.Split(key, "/")
		if len(ss) != 2 {
			printHelpAndExit()
		}
		group := ss[0]
		key = ss[1]
		rsp, err := getPBCli().Get(ctx, &pb.GetRequest{
			Group: group,
			Key:   key,
		})
		if err != nil {
			fmt.Println("failed to do grpc request,", err)
			os.Exit(1)
		}
		err = errcode.GetError(rsp.Base)
		if err != errcode.OK {
			fmt.Println(err)
			os.Exit(1)
		}
		if len(rsp.Entries) != 1 {
			fmt.Println("server internal error,", rsp)
			os.Exit(1)
		}
		e := typeutil.ConvertPbEntryToTypes(rsp.Entries[0])
		version := fmt.Sprintf("%d", e.Meta.Version)
		if e.Meta.UrgentUpdate {
			version = "urgent_" + formatTime(e.Meta.UpdatedAt)
		}
		fmt.Printf("%s/%s\t(version: %s)\n%s\n", e.Group, e.Key, version, e.Value)
	}
}

func formatTime(t time.Time) string {
	return fmt.Sprintf("%02d_%02dT%02d_%02d_%02d",
		t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second())
}
