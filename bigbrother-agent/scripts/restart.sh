#!/bin/bash

readonly module_name=$1 # 组件名称
readonly version=$2 # 组件版本
readonly deploy_path=$3 # 组件部署目录
readonly unzip_path=$4 # 组件新版本的当前解压目录

echo "begin do ${module_name} version ${version} restart action"
service $module_name restart
if [ $? -ne 0 ]; then
    echo "do ${module_name} restart failed"
   exit 1
fi
echo "do ${module_name} version ${version} restart success"