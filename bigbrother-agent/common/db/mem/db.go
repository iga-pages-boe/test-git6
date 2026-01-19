package mem

import (
	"context"
	"errors"
	"sync"

	"code.byted.org/ti/bigbrother-server/types"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrStaleData = errors.New("stale data")
)

type MemKV struct {
	sync.RWMutex
	data map[string]map[string][]*types.Entry
}

func NewMemKV() *MemKV {
	return &MemKV{
		data: make(map[string]map[string][]*types.Entry),
	}
}

func (m *MemKV) Get(ctx context.Context, group, key string, fullHistory bool) ([]*types.Entry, error) {
	m.RLock()
	defer m.RUnlock()
	keyHist := m.data[group]
	if keyHist == nil {
		return nil, ErrNotFound
	}
	hists := keyHist[key]
	if len(hists) == 0 {
		return nil, ErrNotFound
	}
	if fullHistory {
		return hists, nil
	}
	return hists[:1], nil
}

func (m *MemKV) Put(ctx context.Context, group, key string, newEntry, lastEntry *types.Entry) error {
	var cLast *types.Entry
	m.RLock()
	if keys := m.data[group]; keys != nil {
		if his := keys[key]; len(his) > 0 {
			cLast = his[0]
		}
	}
	m.RUnlock()
	if lastEntry == nil {
		if cLast != nil {
			return ErrStaleData
		}
		if newEntry.Meta.Version != 1 {
			return errors.New("version starting with 1")
		}
		m.Lock()
		keys := m.data[group]
		if keys == nil {
			keys = make(map[string][]*types.Entry)
		}
		keys[key] = []*types.Entry{newEntry}
		m.data[group] = keys
		m.Unlock()
		return nil
	}

	if cLast == nil {
		return ErrStaleData
	}
	if lastEntry.Meta.Version != cLast.Meta.Version || lastEntry.Meta.UpdatedAt != cLast.Meta.UpdatedAt {
		return ErrStaleData
	}
	if lastEntry.Meta.Version != 0 {
		newEntry.Meta.Version = lastEntry.Meta.Version + 1
	}

	m.Lock()
	m.data[group][key] = append(m.data[group][key], newEntry)
	copy(m.data[group][key][1:], m.data[group][key])
	m.data[group][key][0] = newEntry
	m.Unlock()
	return nil
}
