package server

import (
	"context"
	"sync"
	"time"

	"code.byted.org/gopkg/logs"
	"code.byted.org/ti/bigbrother-server/pb"

	"google.golang.org/grpc/metadata"
)

func (s *Server) Watch(stream pb.Watch_WatchServer) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	group := "-"
	if psms, ok := md["group"]; ok {
		group = psms[0]
	}
	ctx, cancel := context.WithCancel(stream.Context())
	ws := &watchStream{
		watchers:    make(map[int64]*watcher),
		server:      s,
		stream:      stream,
		watchedKeys: make(map[string]int),
		watchCh:     make(chan *pb.WatchResponse, 1),
		eventCh:     make(chan *pb.Entry, 1),
		eventChOut:  make(chan *pb.Entry, 1),
		reqChan:     make(chan *pb.WatchRequest, 1),
		ctx:         ctx,
		cancel:      cancel,
		meta:        map[string]string{"group": group},
	}
	logs.Info("new stream from %v", ws.meta)

	// post to stopc => terminate server stream; can't use a waitgroup
	// since all goroutines will only terminate after Watch() exits.
	stopc := make(chan struct{}, 4)
	go func() {
		defer func() { stopc <- struct{}{} }()
		ws.coreLoop()
	}()
	go func() {
		defer func() { stopc <- struct{}{} }()
		ws.recvLoop()
	}()
	go func() {
		defer func() { stopc <- struct{}{} }()
		ws.sendLoop()
	}()

	go func() {
		defer func() { stopc <- struct{}{} }()
		ws.eventLoop()
	}()

	s.sub <- ws.eventCh
	defer func() {
		logs.Info("stream %v exited", ws.meta)
		// must pass a copy of ws.watched keys here
		ws.mu.RLock()
		keys := make(map[string]int, len(ws.watchedKeys))
		for k, v := range ws.watchedKeys {
			keys[k] = v
		}
		ws.mu.RUnlock()
		ws.DecrKeys(keys)
		s.unsub <- ws.eventCh
	}()

	<-stopc
	cancel()
	return ctx.Err()
}

// watcher represents a watcher watcher
type watcher struct {
	req pb.WatchCreateRequest
	id  int64

	filter *filter

	// w is the parent.
	ws *watchStream
}

type watchStream struct {
	// mu protects watchers and nextWatcherID
	mu sync.RWMutex

	// watchers receive events from watch broadcast.
	watchers map[int64]*watcher
	// nextWatcherID is the id to assign the next watcher on this stream.
	nextWatcherID int64

	server      *Server
	stream      pb.Watch_WatchServer
	watchedKeys map[string]int

	// reqChan receives all requests from clients
	reqChan chan *pb.WatchRequest

	// watchCh receives watch responses and send to watchers
	watchCh chan *pb.WatchResponse

	// eventCh receives all events from system.
	eventCh chan *pb.Entry

	// eventChOut receives events ready for sending
	eventChOut chan *pb.Entry

	// buf buffers all pending events to send
	buf []*pb.Entry

	// context metadata
	meta map[string]string

	ctx    context.Context
	cancel context.CancelFunc
}

func (ws *watchStream) IncrKeys(keys map[string]int) {
	logs.Info("stream %v increasing keys %v", ws.meta, keys)
	ws.mu.Lock()
	for k, inc := range keys {
		c := ws.watchedKeys[k]
		ws.watchedKeys[k] = c + inc
	}
	ws.mu.Unlock()
	ws.server.IncrKeys(keys)
}

func (ws *watchStream) DecrKeys(keys map[string]int) {
	logs.Info("stream %v decreasing keys %v", ws.meta, keys)
	ws.mu.Lock()
	for k, dec := range keys {
		c := ws.watchedKeys[k]
		if c < dec {
			logs.Warn("UNEXPECTED! decreased too many keys")
		} else if c == dec {
			delete(ws.watchedKeys, k)
		} else {
			ws.watchedKeys[k] = c - dec
		}
	}
	ws.mu.Unlock()
	ws.server.DecrKeys(keys)
}

type filter struct {
	keys []string
}

func (f *filter) match(e *pb.Entry) bool {
	eKey := e.Group + "/" + e.Key
	for i := range f.keys {
		if eKey == f.keys[i] {
			return true
		}
	}
	return false
}

func filterFromRequest(req *pb.WatchCreateRequest) *filter {
	return &filter{
		keys: []string{req.Group + "/" + req.Key},
	}
}

func (ws *watchStream) coreLoop() {
	for {
		select {
		case req, ok := <-ws.reqChan:
			if !ok {
				return
			}
			switch uv := req.RequestUnion.(type) {
			case *pb.WatchRequest_CreateRequest:
				cr := uv.CreateRequest

				w := &watcher{
					id:     ws.nextWatcherID,
					ws:     ws,
					filter: filterFromRequest(cr),
				}
				ws.nextWatcherID++
				ws.watchers[w.id] = w
				ws.IncrKeys(map[string]int{w.filter.keys[0]: 1})
				ws.watchCh <- &pb.WatchResponse{
					WatchId: w.id,
					Created: true,
				}
			case *pb.WatchRequest_CancelRequest:
				if w := ws.watchers[uv.CancelRequest.WatchId]; w != nil {
					ws.DecrKeys(map[string]int{w.filter.keys[0]: 1})
					delete(ws.watchers, uv.CancelRequest.WatchId)
				}
			default:
				panic("not implemented")
			}
		case event := <-ws.eventChOut:
			logs.Info("%v subscriber dispatching event, %v", ws.meta, event)
			for id, w := range ws.watchers {
				if w.filter.match(event) {
					ws.watchCh <- &pb.WatchResponse{
						WatchId: id,
						Entry:   event,
					}
				}
			}
		case <-ws.ctx.Done():
			return
		}
	}
}

func (ws *watchStream) recvLoop() {
	for {
		if ws.ctx.Err() != nil {
			return
		}
		req, err := ws.stream.Recv()
		if err != nil {
			logs.Error("failed to receive %v", err)
			return
		}
		ws.reqChan <- req
	}
}

func (ws *watchStream) sendLoop() {
	for {
		select {
		case wresp, ok := <-ws.watchCh:
			if !ok {
				return
			}
			if wresp.Entry != nil {
				logs.Info("%v subscriber sending to watcher, %v", ws.meta, wresp.Entry)
			}
			// emulate blocking on one watcher
			if ws.meta["psm"] == "toblock" {
				time.Sleep(time.Second * 5)
			}
			// start := time.Now()
			if err := ws.stream.Send(wresp); err != nil {
				logs.Error("failed to send to %v, %v", ws.meta, err)
				return
			}
			if wresp.Entry != nil {
				logs.Info("%v subscriber had sent to watcher, %v", ws.meta, wresp.Entry)
			}
		case <-ws.ctx.Done():
			return
		}
	}
}

func (ws *watchStream) eventLoop() {
	emptyEntry := &pb.Entry{}
	for {
		curEntry := emptyEntry
		outc := ws.eventChOut

		if len(ws.buf) > 0 {
			curEntry = ws.buf[0]
		} else {
			outc = nil
		}
		select {
		case outc <- curEntry:
			ws.buf[0] = nil
			ws.buf = ws.buf[1:]
		case event, ok := <-ws.eventCh:
			if !ok {
				return
			}
			logs.Info("%v subscriber received event, %v", ws.meta, event)
			ws.buf = append(ws.buf, event)
			//logs.Info("%v subscriber buffer size %d", ws.meta, len(ws.buf))
			if l := len(ws.buf); l > 1 {
				logs.Error("%v subscriber overloaded, buf size: %d", ws.meta, l)
			}
		case <-ws.ctx.Done():
			return
		}
	}
}
