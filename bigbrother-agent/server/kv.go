package server

import (
	"context"
	"errors"

	"code.byted.org/gopkg/logs"
	"code.byted.org/ti/bigbrother-agent/common/servers"
	"code.byted.org/ti/bigbrother-server/merr"
	"code.byted.org/ti/bigbrother-server/pb"
	"code.byted.org/ti/bigbrother-server/pb/errcode"
	"code.byted.org/ti/bigbrother-server/typeutil"

	"google.golang.org/grpc/metadata"
)

func (s *Server) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	logs.Info("get request %s", req)
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md["from_agent_cli"]) > 0 {
		entry, _ := s.cache.Get(req.Group, req.Key)
		if entry != nil {
			pbEntries := []*pb.Entry{typeutil.ConvertEntryToPb(entry)}
			return &pb.GetResponse{
				Base:    merr.OK.ToPb(),
				Entries: pbEntries,
			}, nil
		} else {
			return &pb.GetResponse{
				Base:    merr.NotFound.ToPb(),
				Entries: nil,
			}, nil
		}
	}
	// 请求远端
	servers.PolluteCacheFromServers(ctx, s.cache, req.Group, req.Key)
	e, expired := s.cache.Get(req.Group, req.Key)
	if e == nil || expired {
		return &pb.GetResponse{Base: merr.NotFound.ToPb()}, nil
	}
	if e != nil {
		pbEntries := []*pb.Entry{typeutil.ConvertEntryToPb(e)}
		return &pb.GetResponse{
			Base:    merr.OK.ToPb(),
			Entries: pbEntries,
		}, nil
	} else {
		return &pb.GetResponse{
			Base:    merr.NotFound.ToPb(),
			Entries: nil,
		}, nil
	}
}

func (s *Server) ListKeys(ctx context.Context, req *pb.ListKeysRequest) (*pb.ListKeysResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md["from_agent_cli"]) > 0 {
		dump := s.cache.Dump()
		pbEntries := make([]*pb.Entry, 0, len(dump))
		for _, e := range dump {
			pbEntries = append(pbEntries, typeutil.ConvertEntryToPb(e.Entry))
		}
		return &pb.ListKeysResponse{
			Base:    errcode.OK.ToPb(),
			Entries: pbEntries,
		}, nil
	}
	return nil, errors.New("not implemented")
}

func (s *Server) Put(ctx context.Context, req *pb.PutRequest) (*pb.PutResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *Server) CreateGroup(ctx context.Context, request *pb.CreateGroupRequest) (*pb.CreateGroupResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *Server) DeleteGroup(ctx context.Context, request *pb.DeleteGroupRequest) (*pb.DeleteGroupResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *Server) ListGroups(ctx context.Context, request *pb.ListGroupsRequest) (*pb.ListGroupsResponse, error) {
	return nil, errors.New("not implemented")
}
