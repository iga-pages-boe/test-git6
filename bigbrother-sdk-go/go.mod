module code.byted.org/ti/bigbrother-sdk-go

go 1.14

require (
	code.byted.org/gopkg/env v1.3.4
	code.byted.org/gopkg/logs v1.1.11
	code.byted.org/gopkg/metrics v1.4.5
	code.byted.org/ti/bigbrother-proxy v0.0.0-20210107122111-cac5f102e8d4
	code.byted.org/ti/bigbrother-server v0.0.0-20210107122745-b467e66ff632
	code.byted.org/ti/dsaenv v0.0.0-20201012083150-2ce9af2fc342
	github.com/stretchr/testify v1.6.1
	google.golang.org/grpc v1.34.0
)

//replace code.byted.org/ti/bigbrother-proxy => /Users/vicxiao/workspace/go/ti/bigbrother-proxy
//replace code.byted.org/ti/bigbrother-server => /Users/vicxiao/workspace/go/ti/bigbrother-server
