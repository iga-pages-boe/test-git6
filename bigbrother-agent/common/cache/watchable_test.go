package cache

import (
	"fmt"
	"testing"
	"time"

	"github.com/kr/pretty"

	"code.byted.org/ti/bigbrother-server/types"
	"github.com/stretchr/testify/require"
)

func TestNonBlocking(t *testing.T) {
	require := require.New(t)
	cache := NewCache(DefaultExpire)
	group := "test-group"
	key := "test-key"
	i := 0
	for ; i < 1000; i++ {
		cache.Put(group, key, &types.Entry{
			Group: group,
			Key:   key,
			Value: fmt.Sprintf("value-%d", i),
			Meta: &types.Meta{
				Version: int64(i + 1),
			},
		})
	}
	e, expired := cache.Get(group, key)
	require.False(expired)
	require.Equal(fmt.Sprintf("value-%d", i-1), e.Value)
	require.Equal(int64(i), e.Meta.Version)

	var events []*types.Entry
	// 2 seconds is far enough for consuming 1000 entries
	tick := time.NewTimer(time.Second * 2)
loop:
	for {
		select {
		case e := <-cache.Watch():
			events = append(events, e)
		case <-tick.C:
			break loop
		}
	}
	pretty.Println(events)
	// 因为只能消费到最新的一个值
	require.Equal(1, len(events))
	require.Equal(fmt.Sprintf("value-%d", i-1), events[0].Value)
	require.Equal(int64(i), events[0].Meta.Version)
}
