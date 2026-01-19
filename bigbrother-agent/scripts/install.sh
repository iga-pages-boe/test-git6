#!/bin/bash

readonly module_name=$1 # 组件名称
readonly version=$2 # 组件版本
readonly deploy_path=$3 # 组件部署目录
readonly unzip_path=$4 # 组件新版本的当前解压目录
readonly auto_start_flag=$5 # 组件是否开机自启动，如果为1，则自启动
readonly target_version_file="${deploy_path}current_revision"
readonly service_file="${unzip_path}service"

test_dir(){
    [ -d $deploy_path ] || mkdir -p $deploy_path
    if [ $? -ne 0 ]; then
       echo "test ${deploy_path} failed"
       return 1
    fi
    return 0
}

rsync_dir(){
    rsync -r $unzip_path $deploy_path --links --safe-links
    if [ $? -ne 0 ]; then
        echo "rsync ${unzip_path} to ${deploy_path} failed"
        return 2
    fi

    rsync $service_file "/etc/systemd/system/${module_name}.service"
    if [ $? -ne 0 ]; then
        echo "rsync ${service_file} to /etc/systemd/system/${module_name}.service failed"
        return 2
    fi

    systemctl daemon-reload
    if [ $? -ne 0 ]; then
        echo "systemctl daemon-reload failed"
        return 2
    fi
    return 0
}

check_target_version(){
    grep $version $target_version_file
    if [ $? -ne 0 ]; then
        echo "check ${module_name} ${version} file failed"
        return 3
    fi
    return 0
}

auto_start_cmd(){
  if [ "$auto_start_flag"x = "1"x ]; then
    systemctl enable $module_name
    if [ $? -ne 0 ]; then
        echo "systemctl enable ${module_name} failed"
        return 4
    fi
  fi
  return 0
}

echo "begin do ${module_name} version ${version} install action"
test_dir
if [ $? -ne 0 ]; then
   exit 1
fi

rsync_dir
if [ $? -ne 0 ]; then
   exit 2
fi

check_target_version
if [ $? -ne 0 ]; then
   exit 3
fi

auto_start_cmd
if [ $? -ne 0 ]; then
   exit 4
fi
echo "do ${module_name} version ${version} install success"