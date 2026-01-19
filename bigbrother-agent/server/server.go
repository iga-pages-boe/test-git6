package server

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"code.byted.org/gopkg/logs"
	"code.byted.org/ti/bigbrother-agent/common/cache"
	"code.byted.org/ti/bigbrother-server/pb"
	"code.byted.org/ti/bigbrother-server/types"
	"code.byted.org/ti/bigbrother-server/typeutil"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/fsnotify/fsnotify"
)

const (
	LocalKeyGroup    = "local_key"
	LocalKeyDir      = "/etc/default"
	LocalKeyFileName = "dsa_env"
)

type Server struct {
	sync.RWMutex

	cache  *cache.Cache
	events chan *pb.Entry
	sub    chan chan *pb.Entry
	unsub  chan chan *pb.Entry
	subs   map[chan *pb.Entry]struct{}

	watchedKeys map[string]int
	keyChanged  chan struct{}
	inspector   *Inspector

	metrics ServerMetrics
}

type ServerMetrics struct {
	clientCount int64
}

func NewServer() (*Server, error) {
	s := &Server{
		cache:       cache.NewCache(cache.DefaultExpire),
		events:      make(chan *pb.Entry, 1),
		sub:         make(chan chan *pb.Entry),
		unsub:       make(chan chan *pb.Entry),
		subs:        make(map[chan *pb.Entry]struct{}),
		watchedKeys: make(map[string]int),
		keyChanged:  make(chan struct{}, 1),
	}
	s.inspector = NewInspector(s.cache)
	return s, nil
}

func (s *Server) IncrKeys(keys map[string]int) {
	logs.Info("server increasing keys %v", keys)
	s.Lock()
	for k, inc := range keys {
		if inc <= 0 {
			logs.Warn("unexpected key count, %s, %d", k, inc)
			continue
		}
		c := s.watchedKeys[k]
		ss := strings.Split(k, "/")
		s.inspector.Watch(ss[0], ss[1])
		s.watchedKeys[k] = c + inc
	}
	s.Unlock()
	s.keyChanged <- struct{}{}
}

func (s *Server) DecrKeys(keys map[string]int) {
	logs.Info("server decreasing keys %v", keys)
	s.Lock()
	for k, dec := range keys {
		c := s.watchedKeys[k]
		if c < dec {
			logs.Warn("UNEXPECTED! decreased too many keys")
		} else if c == dec {
			delete(s.watchedKeys, k)
		} else {
			s.watchedKeys[k] = c - dec
		}
	}
	s.Unlock()
	s.keyChanged <- struct{}{}
}

func (s *Server) broadcast(e *pb.Entry) {
	logs.Info("broadcasting entry: %v", e)
	s.events <- e
}

func (s *Server) Start(stopCh <-chan struct{}) error {
	address := "127.0.0.1:2310"
	logs.Info("listening on %s", address)
	lis, err := net.Listen("tcp", address)
	if err != nil {
		panic(err)
	}

	recovery := func(p interface{}) (err error) {
		const size = 64 << 10
		buf := make([]byte, size)
		buf = buf[:runtime.Stack(buf, false)]
		logs.Error("panic recovered, error=%s panic stack info=\n%s", p, string(buf))
		return status.Errorf(codes.Unknown, "panic triggered: %v", p)
	}
	opts := []grpc_recovery.Option{
		grpc_recovery.WithRecoveryHandler(recovery),
	}

	grpcServer := grpc.NewServer(
		grpc_middleware.WithUnaryServerChain(
			grpc_recovery.UnaryServerInterceptor(opts...),
			accessInterceptor,
		),
		grpc_middleware.WithStreamServerChain(
			grpc_recovery.StreamServerInterceptor(opts...),
		))
	pb.RegisterWatchServer(grpcServer, s)
	pb.RegisterKVServiceServer(grpcServer, s)
	go s.Run(stopCh)
	go WatchMaxProc()
	logs.Info("starting server grpc server")
	return grpcServer.Serve(lis)
}

func (s *Server) Run(stopCh <-chan struct{}) {
	logs.Info("agent starting")

	go func() {
		for range s.keyChanged {
			// just log
			var watchedKeys map[string]int
			s.RLock()
			watchedKeys = make(map[string]int, len(s.watchedKeys))
			for k, v := range s.watchedKeys {
				watchedKeys[k] = v
			}
			s.RUnlock()
			logs.Info("key changed to %v", watchedKeys)
		}
	}()

	go func() {
		entries := s.cache.Watch()
		for e := range entries {
			logs.Info("received new event from cache, %v", e)
			s.events <- typeutil.ConvertEntryToPb(e)
		}
	}()

	go s.FileWatcher(LocalKeyDir, LocalKeyFileName, LocalKeyGroup)

	for {
		select {
		// subscribe a new watcher
		case l := <-s.sub:
			s.subs[l] = struct{}{}
			atomic.AddInt64(&s.metrics.clientCount, 1)
		// unsubscribe a watcher
		case l := <-s.unsub:
			delete(s.subs, l)
			atomic.AddInt64(&s.metrics.clientCount, -1)
		// broadcast new events to all watchers
		case e := <-s.events:
			logs.Info("broadcasting to all subscribers(%d), %v",
				len(s.subs), e)
			for c, _ := range s.subs {
				c <- e
			}
		case <-stopCh:
			logs.Warn("received stop signal, exiting")
			logs.Stop()
			os.Exit(0)
		}
	}
}

func (s *Server) HandleText(configFile, group string) error {
	file, err := os.Open(configFile)
	if err != nil {
		logs.Error("Cannot open text file: %s, err: [%v]", configFile, err)
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		res := strings.Split(line, "=")
		if len(res) != 2 {
			continue
		}
		res[0] = strings.TrimSpace(res[0])
		res[1] = strings.TrimSpace(res[1])
		oldEntry, _ := s.cache.Get(group, res[0])
		if oldEntry != nil && oldEntry.Value == res[1] {
			logs.Debug("group %s, key %s, value %s do not change", group, res[0], res[1])
			continue
		}
		newEntry := &types.Entry{
			Meta: &types.Meta{
				Version:      1,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
				Creator:      "bigbrother",
				UrgentUpdate: false,
			},
			Group: group,
			Key:   res[0],
			Value: res[1],
		}
		s.cache.Put(group, res[0], newEntry)
		logs.Debug("group %s update key %s = %s\n", group, res[0], res[1])
	}

	if err := scanner.Err(); err != nil {
		logs.Error("Cannot scanner text file: %s, err: [%v]", configFile, err)
		return err
	}

	return nil
}

func (s *Server) FileWatcher(directory, fileName, group string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logs.Error("%v", err)
		return
	}
	defer watcher.Close()

	path := filepath.Join(directory, fileName)
	err = s.HandleText(path, group)
	if err != nil {
		logs.Error("first handle file error: %v", err)
	}

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					if match, _ := regexp.Match(fileName, []byte(event.Name)); match {
						logs.Debug("new write event from path: %s", path)
						err := s.HandleText(path, group)
						if err != nil {
							logs.Error("handle config file error: %v", err)
						}
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					logs.Error("error: %v", err)
					return
				}
			}
		}
	}()

	err = watcher.Add(directory)
	if err != nil {
		logs.Error("error: %v", err)
		return
	}
	<-done
}
