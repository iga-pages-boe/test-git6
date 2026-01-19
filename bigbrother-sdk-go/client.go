package sdk

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"code.byted.org/gopkg/metrics"
	"code.byted.org/ti/bigbrother-sdk-go/mmetrics"

	"code.byted.org/ti/bigbrother-proxy/servers"

	"code.byted.org/ti/bigbrother-server/types"

	"code.byted.org/ti/bigbrother-proxy/watcher"

	"code.byted.org/gopkg/logs"
	"code.byted.org/ti/bigbrother-proxy/cache"
	"code.byted.org/ti/bigbrother-server/pb"
	"code.byted.org/ti/bigbrother-server/pb/errcode"
	"code.byted.org/ti/bigbrother-server/typeutil"
	"google.golang.org/grpc"
)

// 缓存默认过期时间，不建议修改。
// 当服务正常工作时，最新的变更会通过agent的长链接推送过来.
// agent到所有机房都有长链，并且会自动重连。
// 只有当agent未启动或者无法启动时，此缓存时间才有意义。此时，这个缓存值会影响SDK直连server的缓存时间
// 不可设置过短的时间，如果一定要设置，建议>10s
const DefaultExpire = time.Minute

const AgentAddr = "127.0.0.1:2310"

type Client struct {
	sync.RWMutex
	group     string
	watcher   *watcher.Watcher
	expire    time.Duration
	cache     *cache.Cache
	events    chan *types.Entry
	listeners map[string]Callback

	versionMapChan chan map[string]string
	latencyMapChan chan map[string]string
}

type Opt func(*Client)

func WithCacheExpire(expire time.Duration) Opt {
	return func(c *Client) {
		c.expire = expire
	}
}

var metricsSwitch int64
var pullIntervalNormal time.Duration
var pullIntervalAgentDown time.Duration

func init() {
	c, _ := NewClient("bigbrother_sdk")
	c.AddListenerWithValue("metrics", func(value string) CallbackRet {
		if value == "on" {
			atomic.StoreInt64(&metricsSwitch, 1)
			logs.Info("[bigbrother] metrics on")
		} else {
			atomic.StoreInt64(&metricsSwitch, 0)
			logs.Info("[bigbrother] metrics off")
		}
		return CallbackRet{}
	}, "off")
	c.AddListenerWithValue("pullintervalnormal", func(value string) CallbackRet {
		logs.Info("pullintervalnormal setting to %s", value)
		d, err := time.ParseDuration(value)
		if err != nil {
			logs.Error("value %s was not a valid time duration", value)
			return CallbackRet{}
		}
		atomic.StoreInt64((*int64)(&pullIntervalNormal), int64(d))
		return CallbackRet{}
	}, "1m")
	c.AddListenerWithValue("pullintervalagentdown", func(value string) CallbackRet {
		logs.Info("pullintervalagentdown setting to %s", value)
		d, err := time.ParseDuration(value)
		if err != nil {
			logs.Error("value %s was not a valid time duration", value)
			return CallbackRet{}
		}
		atomic.StoreInt64((*int64)(&pullIntervalAgentDown), int64(d))
		return CallbackRet{}
	}, "10s")
}

func NewClient(group string, opts ...Opt) (*Client, error) {
	c := &Client{
		group:          group,
		expire:         DefaultExpire,
		listeners:      make(map[string]Callback),
		watcher:        watcher.NewWatcher([]string{AgentAddr}, watcher.WithMeta("group", group)),
		versionMapChan: make(chan map[string]string, 1),
		latencyMapChan: make(chan map[string]string, 1),
	}
	c.cache = cache.NewCache(c.expire)
	c.events = c.cache.Watch()
	return c, nil
}

func (c *Client) reportVersions() {
	var versionMap, letancyMap map[string]string
	ticker := time.Tick(time.Second * 5)
	for {
		select {
		case <-ticker:
			if atomic.LoadInt64(&metricsSwitch) == 0 {
				continue
			}
			for k, v := range versionMap {
				mmetrics.EmitStore(k, 1, metrics.T{"version", v})
			}
			for k, v := range letancyMap {
				mmetrics.EmitStore(k, 1, metrics.T{"latency", v})
			}
		case vm := <-c.versionMapChan:
			versionMap = vm
		case lm := <-c.latencyMapChan:
			letancyMap = lm

		}
	}
}

// Get 获取最新的值。对于临时错误（网络故障、agent尚未启动）会持续重试直到请求成功。
// returns 最新的值、或者错误。错误可能是ctx取消或者NotFound
func (c *Client) Get(ctx context.Context, key string) (value string, err error) {
	for {
		v, err := c.TryGet(ctx, key)
		if err == errcode.NotFound {
			return v, err
		}
		if err := ctx.Err(); err != nil {
			return v, err
		}

		// 链接agent出错, 再尝试2次, 一次快速尝试，一次延迟1s
		for i := 0; i < 2; i++ {
			tick := time.NewTimer(time.Duration(i) * time.Second)
			select {
			case <-tick.C:
			//pass
			case <-ctx.Done():
				tick.Stop()
				return "", ctx.Err()
			}
			v, err = c.TryGet(ctx, key)
			if err == errcode.NotFound {
				return "", err
			}
			if err == nil {
				return v, nil
			}
		}

		if err := ctx.Err(); err != nil {
			return "", err
		}

		noSuchKey := servers.PolluteCacheFromServers(ctx, c.cache, c.group, key)
		if noSuchKey {
			return "", errcode.NotFound
		}

		e, expired := c.cache.Get(c.group, key)
		if e == nil {
			continue
		}
		if !expired {
			return e.Value, nil
		}
	}
}

// TryGet 只会尝试链接agent，如果agent尚未启动则报错
func (c *Client) TryGet(ctx context.Context, key string) (value string, err error) {
	entry, expired := c.cache.Get(c.group, key)
	if !expired && entry != nil {
		return entry.Value, nil
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}
	conn, err := grpc.Dial(AgentAddr, grpc.WithInsecure())
	if conn != nil {
		defer conn.Close()
	}
	if err != nil { // unlikely
		logs.CtxError(ctx, "[bigbrother] failed to connect to agent, %v", err)
		return "", err
	}
	cli := pb.NewKVServiceClient(conn)

	rsp, err := cli.Get(ctx, &pb.GetRequest{
		Group:      c.group,
		Key:        key,
		AllHistory: false,
	})
	if err != nil {
		logs.CtxError(ctx, "[bigbrother] failed to do grpc request, %v", err)
		return "", err
	}
	err = errcode.GetError(rsp.Base)
	if err != errcode.OK {
		return "", err
	}

	if len(rsp.Entries) == 0 {
		return "", errors.New("server internal error")
	}

	c.cache.Put(c.group, key, typeutil.ConvertPbEntryToTypes(rsp.Entries[0]))
	e, _ := c.cache.Get(c.group, key)
	return e.Value, nil
}

type CallbackRet struct{}
type Callback func(value string) CallbackRet

// Listen 监听key的变化。为key添加Listener时，会同步调用一次Get函数，可以使用Get的返回值进行Listener
// 初始化工作。因为Get在临时错误情况下会不断重试，如果不希望在极端情况被阻塞，可以使用AddListenerWithValue
// 来进行初始化。
// Note:
// 1. 不同的key在一个后台goroutine中处理
// 2. 一个key只能有一个listener，多次注册只有最后一个生效
// 3. 不保证不同key之间的处理顺序。比如中心数据库配置先变更了key_1,再变更key_2，但是
//    key_1的Listener可能在key_2的listener之后被调用
// 4. 可能丢弃中间值。比如key_1的值在极短时间内经历了多次变化 v1->v2->v3->v4，只保证
//    v4一定会传递给listener，中间的值可能会被主动丢弃。这是一个增强的feature，因为
//    中间值v2,v3在已经出现了更新值v4的情况下，再下发没有意义。
func (c *Client) AddListener(key string, cb Callback) error {
	v, err := c.Get(context.Background(), key)
	if err != nil {
		return err
	}
	return c.AddListenerWithValue(key, cb, v)
}

// AddListenerWithValue 除了不同步调用Get之外，其它同AddListener. 可以提供value给callback进行必要的初始化。
func (c *Client) AddListenerWithValue(key string, cb Callback, value string) error {
	needWatcher := false
	firstKey := false
	c.Lock()
	if len(c.listeners) == 0 {
		firstKey = true
	}
	_, existed := c.listeners[key]
	needWatcher = !existed
	c.listeners[key] = cb
	c.Unlock()
	if firstKey {
		go c.reportVersions()
		go c.serveEvents()
	}
	if needWatcher {
		go func() {
			WatchChan := c.watcher.Watch(c.group, key)
			for raw := range WatchChan {
				entry := typeutil.ConvertPbEntryToTypes(raw.Event)
				c.cache.Put(c.group, key, entry)
			}
		}()
	}
	cb(value)
	return nil
}

func (c *Client) serveEvents() {
	// 低频率的轮询补偿
	go func() {
		interval := time.Second
		var lastPull time.Time
		// 打散Get
		time.Sleep(time.Duration(rand.Int63n(int64(interval))))
		for range time.Tick(interval) {
			gap := time.Duration(atomic.LoadInt64((*int64)(&pullIntervalNormal)))
			if !c.watcher.Connected() {
				gap = time.Duration(atomic.LoadInt64((*int64)(&pullIntervalAgentDown)))
			}
			if time.Now().Sub(lastPull) < gap {
				continue
			}
			lastPull = time.Now()
			var keys []string
			c.RLock()
			keys = make([]string, len(c.listeners))
			i := 0
			for k, _ := range c.listeners {
				keys[i] = k
				i++
			}
			c.RUnlock()
			for i := range keys {
				c.Get(context.Background(), keys[i])
			}
		}
	}()

	versionMap := make(map[string]string)
	copyVersionMap := func() map[string]string {
		return copyMap(versionMap)
	}
	latencyMap := make(map[string]string)
	copyLatencyMap := func() map[string]string {
		return copyMap(latencyMap)
	}
	for e := range c.events {
		logs.Info("received event %v", e)
		if e.Group != c.group {
			logs.Warn("[bigbrother] sanity check failed, received unintended events %v, group: %s", e, c.group)
			continue
		}
		var cb Callback
		c.RLock()
		cb = c.listeners[e.Key]
		c.RUnlock()
		if cb == nil {
			continue
		}
		version := fmt.Sprintf("%d", e.Meta.Version)
		if e.Meta.UrgentUpdate {
			version = "urgent_" + formatTime(e.Meta.UpdatedAt)
		}
		versionKey := e.Group + "." + e.Key
		// only update report latency
		if versionMap[versionKey] != "" && versionMap[versionKey] != version && !e.Meta.UpdatedAt.IsZero() {
			latencyMap[versionKey] = latency(e.Meta.UpdatedAt)
			c.latencyMapChan <- copyLatencyMap()
		}
		versionMap[versionKey] = version

		cb(e.Value)
		c.versionMapChan <- copyVersionMap()
	}
}

func copyMap(s map[string]string) map[string]string {
	d := make(map[string]string, len(s))
	for k, v := range s {
		d[k] = v
	}
	return d
}

func latency(start time.Time) string {
	d := time.Now().Sub(start)
	if d < time.Millisecond*100 {
		return "100ms"
	}
	if d < time.Second {
		return "1s"
	}
	if d < time.Second*10 {
		return "10s"
	}
	if d < time.Minute {
		return "1min"
	}
	if d < time.Minute*2 {
		return "2min"
	}
	return "toolong"
}

func formatTime(t time.Time) string {
	return fmt.Sprintf("%02d_%02dT%02d_%02d_%02d",
		t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second())
}
