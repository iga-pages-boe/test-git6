package main

import (
	"fmt"
	"regexp"
)

func main() {
	// 改成go reg支持的格式
	// 不使用否定前瞻的基础正则表达式，匹配所有以/开头的路径
	// 注意：需要在代码中添加额外逻辑来排除特定前缀的路径
	regStr := "^/.*$"
	reg, err := regexp.Compile(regStr)
	if err != nil {
		panic(err)
	}
	match := reg.MatchString("/game_pictures/123.png")
	fmt.Println(match)
}
