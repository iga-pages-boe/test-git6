package sdk

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVersionFormat(t *testing.T) {
	fmt.Println("urgent_" + formatTime(time.Now()))
}

func TestGet(t *testing.T) {
	require := require.New(t)
	client, err := NewClient("golangtest")
	require.Nil(err)
	value, err := client.Get(context.TODO(), "foo")
	fmt.Println(value, err)
}

func TestListen(t *testing.T) {
	require := require.New(t)
	client, err := NewClient("golangtest")
	require.Nil(err)
	err = client.AddListenerWithValue("foo", func(value string) CallbackRet {
		fmt.Println(value)
		return CallbackRet{}
	}, "test")
	require.Nil(err)
	time.Sleep(time.Hour)
}
