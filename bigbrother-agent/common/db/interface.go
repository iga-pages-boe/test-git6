package db

import (
	"context"

	"code.byted.org/ti/bigbrother-server/types"
)

type Source interface {
	Get(ctx context.Context, group, key string, fullHistory bool) ([]*types.Entry, error)
	Put(ctx context.Context, group, key string, newEntry, lastEntry *types.Entry) error
}

type WatchableSource interface {
	Source
	Watch(ctx context.Context) chan *types.Entry
	UpdateWatchingKeys(ctx context.Context, keys map[string][]string)
}
