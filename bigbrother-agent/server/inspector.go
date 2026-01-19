package server

import (
	"sync"

	"code.byted.org/ti/bigbrother-server/ctxkey"
	"code.byted.org/ti/bigbrother-server/typeutil"

	"code.byted.org/gopkg/env"
	"code.byted.org/gopkg/logs"
	"code.byted.org/ti/bigbrother-agent/common/cache"
	"code.byted.org/ti/bigbrother-agent/common/servers"
	pwatcher "code.byted.org/ti/bigbrother-agent/common/watcher"
)

// TODO: add Close method
type Inspector struct {
	sync.RWMutex
	cache       *cache.Cache
	watchedKeys map[string]struct{}
	watchers    []*pwatcher.Watcher
}

func NewInspector(c *cache.Cache) *Inspector {
	return &Inspector{
		cache:       c,
		watchedKeys: make(map[string]struct{}),
	}
}

func (i *Inspector) Watch(group, key string) {
	startWatcher := false
	i.Lock()
	if len(i.watchers) == 0 {
		logs.Info("creating watchers")
		metakv := []string{ctxkey.MetadataAgentHost, env.HostIP()}
		for idc, ss := range servers.Servers() {
			if idc == "RD" {
				metakv = append(metakv, "X-TT-ENV", "boe_test")
			}
			logs.Info("watching %s with %v using metadata %v", idc, ss, metakv)
			w := pwatcher.NewWatcher(ss, pwatcher.WithMeta(metakv...), pwatcher.WithPingpong())
			i.watchers = append(i.watchers, w)
		}
	}

	if _, in := i.watchedKeys[group+"/"+key]; !in {
		startWatcher = true
		i.watchedKeys[group+"/"+key] = struct{}{}
	}
	i.Unlock()
	if startWatcher {
		go func() {
			for _, w := range i.watchers {
				logs.Info("watching %s/%s through %s", group, key, w)
				go func(w *pwatcher.Watcher) {
					for e := range w.Watch(group, key) {
						logs.Info("received event from watcher %s, event %v", w, e)
						i.cache.Put(group, key, typeutil.ConvertPbEntryToTypes(e.Event))
					}
				}(w)
			}
		}()
	}
}
