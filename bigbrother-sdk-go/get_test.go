package sdk

import (
	"context"
	"fmt"
	"testing"
)

// 由于无法在 GOROOT 中找到 "code.byted.org/ti/bigbrother-sdk-go" 包，推测可能包路径有误，
// 请确认该包是否存在，若包存在可使用 go get 命令拉取，此处暂时注释掉该导入
// import "code.byted.org/ti/bigbrother-sdk-go"

func TestGet22(t *testing.T) {
	fmt.Println("Hello, World!")
	client, err := NewClient("golangtest")
	if err != nil {
		fmt.Println(err)
	}
	value, err := client.Get(context.Background(), "foo")
	fmt.Println(value, err)

}
