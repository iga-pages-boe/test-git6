# 动态加速降级服务SDK

## 设计文档

see https://bytedance.feishu.cn/docs/doccnUG8R3rnxe97787DO206wwd#

## 推荐用法

1. 首先在平台上注册group，比如route_agent
2. 在自己的group下发布需要的key及其值
3. 创建client, 使用1中的group创建client

  ```
   client, err := NewClient("route_agent")
  ```

4. 调用`TryGet`尝试获取key的配置，如果失败模块可以准备兜底的值
5. 调用`AddListenerWithValue`来监听key的变化，其中传入的value，应该是TryGet返回的值、或者模块兜底配置

## 注意

1. AddListener 有每隔5分钟一次的低频轮询补偿来保证极端情况下长链的数据丢失，使用方不需要再自己轮询补偿
2. 具有版本去重的功能，同一个版本（无论是来自轮询还是长链）只会触发一次Listener
3. SDK具备对agent的容灾能力，当agent不可用是会回退成每一分钟轮询一次所有中心服务，直到agent恢复
4. 如果消费堆积，对于同一个key，会丢弃中间变更的值，只保留最新值。例如，在消费堆积的时候，key连续变化产生了三个版本v1、v2、v3，SDK将自动丢弃过时的v1,v2只保留最新的v3等待消费。