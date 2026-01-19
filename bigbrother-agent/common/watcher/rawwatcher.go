package watcher

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"code.byted.org/gopkg/logs"
	"code.byted.org/ti/bigbrother-agent/common/consul"
	"code.byted.org/ti/bigbrother-server/pb"
)

const maxBackoff = time.Second * 30

type RawWatchResponse struct {
	Event *pb.Entry

	// Created is used to indicate the creation of the Watcher.
	Created bool
}

func (r *RawWatchResponse) String() string {
	if r.Event == nil {
		return fmt.Sprintf("empty event(created: %s)", r.Created)
	}
	return fmt.Sprintf("%v", r.Event)
}

type WatchChan <-chan RawWatchResponse

// Watcher implements the Watcher interface
type Watcher struct {
	mu          sync.RWMutex
	stream      *watchGrpcStream
	servers     []string
	serverIndex int
	md          metadata.MD
	connected   int64
	pingpong    bool
}

func (w *Watcher) String() string {
	return fmt.Sprintf("watcher for %v", w.servers)
}

type WatcherOpt func(w *Watcher)

func WithMeta(kv ...string) WatcherOpt {
	return func(w *Watcher) {
		md := metadata.Pairs(kv...)
		w.md = md
	}
}

func WithPingpong() WatcherOpt {
	return func(w *Watcher) {
		w.pingpong = true
	}
}

func NewWatcher(servers []string, opts ...WatcherOpt) *Watcher {
	sscopy := make([]string, len(servers))
	for i := range servers {
		sscopy[i] = servers[i]
	}
	w := &Watcher{
		mu:      sync.RWMutex{},
		servers: sscopy,
		md:      nil,
	}
	for i := range opts {
		opts[i](w)
	}
	return w
}

func (w *Watcher) Connected() bool {
	return atomic.LoadInt64(&w.connected) == 1
}

func (w *Watcher) setConnected(connected bool) {
	if connected {
		atomic.StoreInt64(&w.connected, 1)
		return
	}
	atomic.StoreInt64(&w.connected, 0)
}

func (w *Watcher) nextServer() string {
	// don't need a lock to protect
	s := w.servers[w.serverIndex]
	w.serverIndex++
	if w.serverIndex >= len(w.servers) {
		w.serverIndex = 0
	}
	return s
}

// watchGrpcStream tracks all watch resources attached to a single grpc stream.
type watchGrpcStream struct {
	watcher *Watcher
	conn    *grpc.ClientConn

	// ctx controls internal remote.Watch requests
	ctx context.Context

	// substreams holds all active watchers on this grpc stream
	substreams map[int64]*watcherStream
	// resuming holds all resuming watchers on this grpc stream
	resuming []*watcherStream

	// reqc sends a watch request from Watch() to the main goroutine
	reqc chan *watchRequest

	// respc receives data from the watch watcher
	respc chan *pb.WatchResponse

	// resumec closes to signal that all substreams should begin resuming
	resumec chan struct{}

	// wg is Done when all substream goroutines have exited
	wg sync.WaitGroup

	// errc transmits errors from grpc Recv to the watch stream reconnect logic
	errc chan error
}

// watcherStream represents a registered Watcher
type watcherStream struct {
	// initReq is the request that initiated this request
	initReq watchRequest

	// outc publishes watch responses to subscriber
	outc chan RawWatchResponse
	// recvc buffers watch responses before publishing
	recvc chan *RawWatchResponse
	// donec closes when the watcherStream goroutine stops.
	donec chan struct{}

	// id is the registered watch id on the grpc stream
	id int64

	// buf holds all events received from server but not yet consumed by the watcher
	buf []*RawWatchResponse
}

type watchRequest struct {
	ctx   context.Context
	group string
	key   string

	// send Created notification event if this field is true
	createdNotify bool

	// retc receives a chan RawWatchResponse once the Watcher is established
	retc chan chan RawWatchResponse
}

func (w *watchRequest) toPB() *pb.WatchRequest {
	return &pb.WatchRequest{RequestUnion: &pb.WatchRequest_CreateRequest{
		CreateRequest: &pb.WatchCreateRequest{
			Group: w.group,
			Key:   w.key,
		},
	}}
}

func (w *Watcher) newWatcherGrpcStream() *watchGrpcStream {
	wgs := &watchGrpcStream{
		watcher:    w,
		ctx:        context.Background(),
		substreams: make(map[int64]*watcherStream),
		respc:      make(chan *pb.WatchResponse),
		reqc:       make(chan *watchRequest),
		errc:       make(chan error, 1),
		resumec:    make(chan struct{}),
	}
	go wgs.run()
	return wgs
}

func (w *watchGrpcStream) reconnect(ctx context.Context) (pb.Watch_WatchClient, error) {
	if w.conn != nil {
		w.conn.Close()
	}
	var addr string
	server := w.watcher.nextServer()
	if strings.Contains(server, ":") {
		addr = server
	} else {
		// TODO: 容器化之后支持容器的服务发现
		enpoints, err := consul.Lookup(server)
		if err != nil {
			return nil, err
		}
		if len(enpoints) == 0 {
			return nil, errors.New("no consul endpoint")
		}
		e := enpoints[rand.Intn(len(enpoints))]
		addr = e.Addr
	}

	logs.Info("[bigbother-proxy]connecting to %s", addr)
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
	if err != nil {
		return nil, err
	}
	w.conn = conn
	ctx = metadata.NewOutgoingContext(ctx, w.watcher.md)
	return pb.NewWatchClient(conn).Watch(ctx)
}

// openWatchClient retries opening a watch watcher until success
func (w *watchGrpcStream) openWatchClient() (ws pb.Watch_WatchClient, err error) {
	backoff := time.Millisecond
	for {
		select {
		case <-w.ctx.Done():
			if err == nil {
				return nil, w.ctx.Err()
			}
			return nil, err
		default:
		}
		if ws, err = w.reconnect(w.ctx); ws != nil && err == nil {
			w.watcher.setConnected(true)
			break
		}
		w.watcher.setConnected(false)
		if err != nil {
			logs.Error("[bigbrother-proxy] failed to open watch watcher, %v", err)
			// retry, but backoff
			if backoff < maxBackoff {
				// 25% backoff factor
				backoff = backoff + backoff/4
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			time.Sleep(backoff)
		}
	}
	return ws, nil
}

// serveSubstream forwards watch responses from run() to the subscriber
func (w *watchGrpcStream) serveSubstream(ws *watcherStream, resumec chan struct{}) {
	defer func() {
		close(ws.donec)
		w.wg.Done()
	}()

	emptyWr := &RawWatchResponse{}
	for {
		curWr := emptyWr
		outc := ws.outc

		if len(ws.buf) > 0 {
			curWr = ws.buf[0]
		} else {
			outc = nil
		}
		select {
		case outc <- *curWr:
			ws.buf[0] = nil
			ws.buf = ws.buf[1:]
		case wr, ok := <-ws.recvc:
			if !ok {
				// shutdown from closeSubstream
				return
			}

			if wr.Created {
				if ws.initReq.retc != nil {
					ws.initReq.retc <- ws.outc
					// to prevent next write from taking the slot in buffered channel
					// and posting duplicate create events
					ws.initReq.retc = nil

					// send first creation event only if requested
					if ws.initReq.createdNotify {
						ws.outc <- *wr
					}
				}
			}

			// Created event is already sent above,
			// Watcher should not post duplicate events
			if wr.Created {
				continue
			}

			// TODO pause channel if buffer gets too large
			logs.Info("[bigbrother-proxy] put response in buffer(len: %d), %s", len(ws.buf), wr)
			ws.buf = append(ws.buf, wr)
		case <-w.ctx.Done():
			panic("watch cancellation not supported yet")
			return
		case <-ws.initReq.ctx.Done():
			panic("watch cancellation not supported yet")
			return
		case <-resumec:
			logs.Info("[bigbrother-proxy] substream %d resuming", ws.id)
			return
		}
	}
	// lazily send cancel message if events on missing id
}

// joinSubstreams waits for all substream goroutines to complete.
func (w *watchGrpcStream) joinSubstreams() {
	for _, ws := range w.substreams {
		<-ws.donec
	}
	for _, ws := range w.resuming {
		if ws != nil {
			<-ws.donec
		}
	}
}

// serveWatchClient forwards messages from the grpc stream to run()
func (w *watchGrpcStream) serveWatchClient(wc pb.Watch_WatchClient) {
	for {
		resp, err := wc.Recv()
		if err != nil {
			w.errc <- err
			return
		}
		w.respc <- resp
	}
}

func (w *watchGrpcStream) newWatchClient() (pb.Watch_WatchClient, error) {
	// mark all substreams as resuming
	close(w.resumec)
	w.resumec = make(chan struct{})
	w.joinSubstreams()
	for _, ws := range w.substreams {
		ws.id = -1
		w.resuming = append(w.resuming, ws)
	}
	// strip out nils, if any
	var resuming []*watcherStream
	for _, ws := range w.resuming {
		if ws != nil {
			resuming = append(resuming, ws)
		}
	}
	w.resuming = resuming
	w.substreams = make(map[int64]*watcherStream)

	// connect to grpc stream
	wc, err := w.openWatchClient()

	// serve all non-closing streams, even if there's a watcher error
	// so that the teardown path can shutdown the streams as expected.
	for _, ws := range w.resuming {
		ws.donec = make(chan struct{})
		w.wg.Add(1)
		go w.serveSubstream(ws, w.resumec)
	}

	if err != nil {
		return nil, err
	}

	// receive data from new grpc stream
	go w.serveWatchClient(wc)
	return wc, nil
}

func (w *watchGrpcStream) addSubstream(resp *pb.WatchResponse, ws *watcherStream) {
	ws.id = resp.WatchId
	w.substreams[ws.id] = ws
}

// dispatchEvent sends a RawWatchResponse to the appropriate Watcher stream
func (w *watchGrpcStream) dispatchEvent(pbresp *pb.WatchResponse) bool {
	wr := &RawWatchResponse{
		Event:   pbresp.Entry,
		Created: pbresp.Created,
	}

	return w.unicastResponse(wr, pbresp.WatchId)
}

// nextResume chooses the next resuming to register with the grpc stream. Abandoned
// streams are marked as nil in the queue since the head must wait for its inflight registration.
func (w *watchGrpcStream) nextResume() *watcherStream {
	for len(w.resuming) != 0 {
		if w.resuming[0] != nil {
			return w.resuming[0]
		}
		w.resuming = w.resuming[1:len(w.resuming)]
	}
	return nil
}

// unicastResponse sends a watch response to a specific watch substream.
func (w *watchGrpcStream) unicastResponse(wr *RawWatchResponse, watchId int64) bool {
	ws, ok := w.substreams[watchId]
	if !ok {
		return false
	}
	select {
	case ws.recvc <- wr:
	case <-ws.donec:
		return false
	}
	return true
}

// run is the root of the goroutines for managing a Watcher watcher
func (w *watchGrpcStream) run() {
	var wc pb.Watch_WatchClient
	var closeErr error

	if wc, closeErr = w.newWatchClient(); closeErr != nil {
		return
	}

	cancelSet := make(map[int64]struct{})

	// Adhoc: TLB 1 分钟会断开一次grpc长链，通过30秒一次的心跳来保活
	var pingpongChan <-chan time.Time
	var ppReq = &pb.WatchRequest{RequestUnion: &pb.WatchRequest_PingpongRequest{}}
	if w.watcher.pingpong {
		pingpongChan = time.NewTicker(time.Second * 30).C
	}

	for {
		select {
		case <-pingpongChan:
			//logs.Info("sending pingpong")
			wc.Send(ppReq)
		// Watch() requested
		case wreq := <-w.reqc:
			outc := make(chan RawWatchResponse, 1)
			// TODO: pass custom watch ID?
			ws := &watcherStream{
				initReq: *wreq,
				id:      -1,
				outc:    outc,
				// unbuffered so resumes won't cause repeat events
				recvc: make(chan *RawWatchResponse),
			}

			ws.donec = make(chan struct{})
			w.wg.Add(1)
			go w.serveSubstream(ws, w.resumec)

			// queue up for Watcher creation/resume
			w.resuming = append(w.resuming, ws)
			if len(w.resuming) == 1 {
				// head of resume queue, can register a new Watcher
				wc.Send(ws.initReq.toPB())
			}
		// new events from the watch watcher
		case pbresp := <-w.respc:
			logs.Info("[bigbrother-proxy] (%s) received event, watcher-id %d, %v", w.watcher, pbresp.WatchId, pbresp.Entry)
			switch {
			case pbresp.Created:
				// response to head of queue creation
				if ws := w.resuming[0]; ws != nil {
					w.addSubstream(pbresp, ws)
					w.dispatchEvent(pbresp)
					w.resuming[0] = nil
				}

				if ws := w.nextResume(); ws != nil {
					wc.Send(ws.initReq.toPB())
				}

			default:
				// dispatch to appropriate watch stream
				ok := w.dispatchEvent(pbresp)

				if ok {
					break
				}

				// watch response on unexpected watch id; cancel id
				if _, ok := cancelSet[pbresp.WatchId]; ok {
					break
				}

				cancelSet[pbresp.WatchId] = struct{}{}
				cr := &pb.WatchRequest_CancelRequest{
					CancelRequest: &pb.WatchCancelRequest{
						WatchId: pbresp.WatchId,
					},
				}
				req := &pb.WatchRequest{RequestUnion: cr}
				wc.Send(req)
			}

		// watch watcher failed on Recv; spawn another if possible
		case err := <-w.errc:
			logs.Error("[bigbrother] failed to recv, %v", err)
			if wc, closeErr = w.newWatchClient(); closeErr != nil {
				return
			}
			if ws := w.nextResume(); ws != nil {
				wc.Send(ws.initReq.toPB())
			}
			cancelSet = make(map[int64]struct{})

		case <-w.ctx.Done():
			return
		}
	}
}

// Watch posts a watch request to run() and waits for a new Watcher channel
func (w *Watcher) Watch(group, key string) WatchChan {
	wr := &watchRequest{
		// TODO: support cancellation
		ctx:   context.Background(),
		group: group,
		key:   key,
		retc:  make(chan chan RawWatchResponse, 1),
	}

	ok := false

	w.mu.Lock()
	if w.stream == nil {
		w.stream = w.newWatcherGrpcStream()
	}
	wgs := w.stream
	reqc := wgs.reqc
	w.mu.Unlock()

	// couldn't create channel; return closed channel
	closeCh := make(chan RawWatchResponse, 1)

	// submit request
	select {
	case reqc <- wr:
		ok = true
	case <-wr.ctx.Done():
	}

	// receive channel
	if ok {
		return <-wr.retc
	}

	close(closeCh)
	return closeCh
}
