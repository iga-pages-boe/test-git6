package db

import (
	"context"
	"sync"
	"time"

	"code.byted.org/gopkg/logs"
	"code.byted.org/ti/bigbrother-agent/common/cache"
	"code.byted.org/ti/bigbrother-server/types"
)

type WrappedSource struct {
	sync.RWMutex
	source Source
	keys   map[string][]string
	events chan *types.Entry
	cache  *cache.Cache
}

func (w *WrappedSource) Get(ctx context.Context, group, key string, fullHistory bool) ([]*types.Entry, error) {
	if !fullHistory {
		e, expired := w.cache.Get(group, key)
		if e != nil && !expired {
			return []*types.Entry{e}, nil
		}
	}
	entries, err := w.source.Get(ctx, group, key, fullHistory)
	if err != nil {
		return entries, err
	}
	if len(entries) > 0 {
		w.cache.Put(group, key, entries[0])
	}
	return entries, err
}

func (w *WrappedSource) Put(ctx context.Context, group, key string, newEntry, lastEntry *types.Entry) error {
	return w.source.Put(ctx, group, key, newEntry, lastEntry)
}

func (w *WrappedSource) Watch(ctx context.Context) chan *types.Entry {
	return w.events
}

func (w *WrappedSource) UpdateWatchingKeys(ctx context.Context, keys map[string][]string) {
	logs.Trace("db updating watching keys to %v", keys)
	w.Lock()
	defer w.Unlock()
	w.keys = keys
}

func (w *WrappedSource) asyncUpdate() {
	//inverval := time.Second * 60
	inverval := time.Second

	cacheEvents := w.cache.Watch()
	go func() {
		for e := range cacheEvents {
			w.events <- e
		}
	}()
	for range time.Tick(inverval) {
		var keyDump map[string][]string
		w.RLock()
		keyDump = w.keys
		w.RUnlock()
		logs.Trace("async update, %v", keyDump)
		for group, ks := range keyDump {
			for i := range ks {
				key := ks[i]
				logs.Trace("refreshing %s/%s", group, key)
				w.Get(context.Background(), group, key, false)
			}
		}
	}
}

func WrapAsWatchable(db Source) WatchableSource {
	ws := &WrappedSource{
		source: db,
		events: make(chan *types.Entry, 1),
		cache:  cache.NewCache(0),
	}
	go ws.asyncUpdate()
	return ws
}
