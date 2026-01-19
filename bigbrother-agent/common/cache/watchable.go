package cache

import (
	"sync"
	"time"

	"code.byted.org/ti/bigbrother-server/types"
)

type Item struct {
	Entry *types.Entry
	T     time.Time
}

type Cache struct {
	sync.RWMutex
	cache     map[string]*Item
	expire    time.Duration
	events    chan *types.Entry
	eventsOut chan *types.Entry
	buf       []*types.Entry
}

const DefaultExpire = time.Minute

func NewCache(expire time.Duration) *Cache {
	if expire <= 0 {
		expire = DefaultExpire
	}

	c := &Cache{
		expire:    expire,
		cache:     make(map[string]*Item),
		events:    make(chan *types.Entry, 1),
		eventsOut: make(chan *types.Entry),
	}
	go c.serverEvents()
	return c
}

func (c *Cache) serverEvents() {
	emptyEntry := &types.Entry{}
	for {
		curEntry := emptyEntry
		outc := c.eventsOut
		if len(c.buf) > 0 {
			curEntry = c.buf[0]
		} else {
			outc = nil
		}
		select {
		case outc <- curEntry:
			c.buf[0] = nil // free
			c.buf = c.buf[1:]
		case e, ok := <-c.events:
			if !ok {
				// shutdown
				return
			}
			// remove staled changes
			i := 0
			for ; i < len(c.buf); i++ {
				if c.buf[i].Group == e.Group && c.buf[i].Key == e.Key {
					break
				}
			}
			if i < len(c.buf) { // found staled one
				copy(c.buf[i:], c.buf[i+1:])
				c.buf[len(c.buf)-1] = e
			} else {
				c.buf = append(c.buf, e)
			}
		}
	}

}

// Watch returns a channel of updating eventing in this cache
// Note: the returned channel is closable. If it is closed, cache will also be
// shutdown.
func (c *Cache) Watch() chan *types.Entry {
	return c.eventsOut
}

func (c *Cache) Dump() map[string]*Item {
	c.RLock()
	defer c.RUnlock()
	ret := make(map[string]*Item, len(c.cache))
	for k, v := range c.cache {
		ret[k] = v
	}
	return ret
}

func (c *Cache) Get(group, key string) (entry *types.Entry, expired bool) {
	c.RLock()
	e, ok := c.cache[group+"/"+key]
	c.RUnlock()
	if !ok {
		return nil, false
	}
	return e.Entry, time.Now().Sub(e.T) > c.expire
}

func (c *Cache) Put(group, key string, entry *types.Entry) {
	if entry == nil || group == "" || key == "" {
		return
	}
	var old *types.Entry
	c.Lock()
	e := c.cache[group+"/"+key]
	c.cache[group+"/"+key] = &Item{
		Entry: entry,
		T:     time.Now(),
	}
	c.Unlock()
	if e != nil {
		old = e.Entry
	}
	if entry.NewerThan(old) {
		c.events <- entry
	}
}
