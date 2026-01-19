#!/bin/bash

set +x
CUR_DIR=`pwd`
RUN_NAME="dsa_bigbrother_agent"
Name="bigbrother_agent" # 组件名称，注意，组件名称允许与二进制名不相同
Version=$BUILD_VERSION #编译的版本号
WorkingDirectory="/opt/dsa/bigbrother_agent/"

mkdir -p output/bin output/log output/scripts
cp -r scripts/* output/scripts
chmod +x output/scripts/*
eval "cat <<EOF
$( < script/service )
EOF
" > output/service

eval "cat <<EOF
$( < script/service )
EOF
"

go build -o output/bin/${RUN_NAME}
go build -o output/bin/cli ./cli

# 生成md5校验数据
cd output
md5sum bin/${RUN_NAME} > check.md5
cd $CUR_DIR