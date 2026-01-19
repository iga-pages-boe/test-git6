#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import json
import os

# 设置文件路径
json_file_path = '/Users/bytedance/Desktop/code/iga(2025-05-01)20250924194241.json'
output_file_path = '/Users/bytedance/Desktop/code/iga_with_condition_routes.json'

# 读取JSON文件
def read_json_file(file_path):
    with open(file_path, 'r', encoding='utf-8') as f:
        return json.load(f)

# 保存JSON文件
def save_json_file(file_path, data):
    with open(file_path, 'w', encoding='utf-8') as f:
        json.dump(data, f, ensure_ascii=False, indent=4)

# 查找ListOriginGroup操作并获取ConditionRoutes模板
def get_condition_route_template(api_list):
    for api in api_list:
        if api.get('Action') == 'ListOriginGroup':
            routes = api.get('Route', [])
            if routes and len(routes) > 0:
                condition_routes = routes[0].get('ConditionRoutes', [])
                if condition_routes and len(condition_routes) > 0:
                    return condition_routes[0]  # 返回第一个条件路由作为模板
    return None

# 为所有其他操作添加条件路由
def add_condition_routes_to_all_apis(api_list, template):
    if not template:
        print("未找到ListOriginGroup的ConditionRoutes模板")
        return api_list
    
    processed_count = 0
    for api in api_list:
        action = api.get('Action')
        # 跳过ListOriginGroup自身
        if action == 'ListOriginGroup':
            continue
        
        routes = api.get('Route', [])
        if routes and len(routes) > 0:
            route = routes[0]
            path = route.get('Path')
            if path and 'ConditionRoutes' in route and len(route['ConditionRoutes']) == 0:
                # 创建新的条件路由，替换Path
                new_condition_route = template.copy()
                new_condition_route['Path'] = path
                
                # 添加到ConditionRoutes数组
                route['ConditionRoutes'] = [new_condition_route]
                processed_count += 1
                print(f"已为 {action} 添加条件路由，Path: {path}")
    
    print(f"总共处理了 {processed_count} 个操作")
    return api_list

# 主函数
def main():
    print("开始处理JSON文件...")
    
    # 读取JSON数据
    data = read_json_file(json_file_path)
    api_list = data.get('ApiList', [])
    
    if not api_list:
        print("JSON文件中未找到ApiList数组")
        return
    
    print(f"找到 {len(api_list)} 个API操作")
    
    # 获取条件路由模板
    template = get_condition_route_template(api_list)
    if not template:
        print("未找到有效的条件路由模板")
        return
    
    print(f"找到条件路由模板: {template}")
    
    # 为所有操作添加条件路由
    updated_api_list = add_condition_routes_to_all_apis(api_list, template)
    data['ApiList'] = updated_api_list
    
    # 保存更新后的JSON文件
    save_json_file(output_file_path, data)
    print(f"已保存更新后的JSON文件到: {output_file_path}")

if __name__ == "__main__":
    main()