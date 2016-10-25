# uniway

[![Build Status](https://travis-ci.org/unicok/uniway.svg?branch=master)](https://travis-ci.org/unicok/uniway)
[![codecov](https://codecov.io/gh/unicok/uniway/branch/master/graph/badge.svg)](https://codecov.io/gh/unicok/uniway)

调用示例
=======

调用示例1 - 以客户端身份连接到网关：

```go
client := proto.NewClient(
	Conn, // 物理连接
	fastway.EndPointCfg {
		MsgFormat,       // 消息序列化格式
		MemPool,         // 内存池
		MaxPacket,       // 包体积限制
		BufferSize,      // 预读所用的缓冲区大小
		SendChanSize,    // 物理连接异步发送用的chan缓冲区大小
		RecvChanSize,    // 虚拟连接异步接收用的chan缓冲区大小
		PingInterval,    // 发送Ping的时间间隔，必须小于网关的ClientIdleTimeout
		PingTimeout,     // 可选参数，当网关响应Ping超过此项设置，将调用TimeoutCallback
		TimeoutCallback, // 此项设置需要跟PingTimeout配套
	},
)
```

调用示例2 - 以服务端身份连接到网关：

```go
server, err := proto.DialServer(
	Conn, // 物理连接
	fastway.EndPointCfg {
		ServerID,        // 服务端ID
		AuthKey,         // 身份验证用的Key
		MsgFormat,       // 消息序列化格式
		MemPool,         // 内存池
		MaxPacket,       // 包体积限制
		BufferSize,      // 预读所用的缓冲区大小
		SendChanSize,    // 物理连接异步发送用的chan缓冲区大小
		RecvChanSize,    // 虚拟连接异步接收用的chan缓冲区大小
		PingInterval,    // 发送Ping的时间间隔，必须小于网关的ClientIdleTimeout
		PingTimeout,     // 可选参数，当网关响应Ping超过此项设置，将调用TimeoutCallback
		TimeoutCallback, // 此项设置需要跟PingTimeout配套
	},
)
```

调用示例3 - 创建一个虚拟连接：

```go
conn, err := client.Dial(ServerID)
```

调用示例4 - 接收一个虚拟连接：

```go
conn, err := server.Accept()
```

注意事项：

+ 网关开启[snet协议](https://github.com/unicok/snet)时，需要用[snet协议的连接](https://github.com/unicok/snet)来创建Endpoint
+ 使用snet协议时，可以利用ping超时机制来做到尽早的尝试重连，依赖于TCP重传失败或者IO超时，都比较难以控制超时时间。

网关
====

本网关提供了一个命令行程序用来对外提供服务，命令行支持以下参数：

网关相关参数：

| 参数名 | 说明 | 默认值 |
| --- | --- | --- |
| ReusePort | 是否开启reuseport特性 | false |
| MaxPacket | 最大的消息包体积 | 512K |
| MemPoolType | [slab内存池类型 (sync、atom或chan)](https://github.com/funny/slab) | atom |
| MemPoolFactor | slab内存池的Chunk递增指数 | 2 |
| MemPoolMaxChunk | slab内存池中最大的Chunk大小 | 64K |
| MemPoolMinChunk | slab内存池中最小的Chunk大小 | 64B |
| MemPoolPageSize | slab内存池的每个Slab内存大小 | 1M |

客户端相关参数：

| 参数名 | 说明 | 默认值 |
| --- | --- | --- |
| ClientAddr | 网关暴露给客户端的地址 | ":0" |
| ClientMaxConn | 每个客户端可以创建的最大虚拟连接数 | 16 |
| ClientBufferSize | 每个客户端连接使用的 bufio.Reader 缓冲区大小 | 2K |
| ClientIdleTimeout | 客户端连接空闲超过时长，客户端应在此时间范围内发送ping命令来保活 | 30秒 |
| ClientSendChanSize | 每个客户端连接异步发送消息用的chan缓冲区大小 | 1000 |
| ClientSnetEnable | [是否为客户端开启snet协议](https://github.com/funny/snet) | false |
| ClientSnetEncrypt | 是否为客户端开启snet加密 | false |
| ClientSnetBuffer | 每个客户端物理连接对应的snet重连缓冲区大小 | 32k |
| ClientSnetInitTimeout | 客户端snet握手超时时间 | 10秒 |
| ClientSnetWaitTimeout | 客户端snet等待重连超时时间 | 60秒 |

服务端相关参数：

| 参数名 | 说明 | 默认值 |
| --- | --- | --- |
| ServerAddr | 网关暴露给服务端的地址 | ":0" |
| ServerAuthPassword | 用于验证服务端合法性的秘钥 | 空 |
| ServerBufferSize | 每个服务端连接使用的 bufio.Reader 缓冲区大小 | 64K |
| ServerIdleTimeout | 服务端连接空闲超过时长，服务端应在此时间范围内发送ping命令来保活 | 30秒 |
| ServerSendChanSize | 每个服务端连接异步发送消息用的chan缓冲区大小 | 10万 |
| ServerSnetEnable | [是否为服务端开启snet协议](https://github.com/funny/snet) | false |
| ServerSnetEncrypt | 是否为服务端开启snet加密 | false |
| ServerSnetBuffer | 每个服务端物理连接对应的snet重连缓冲区大小 | 1M |
| ServerSnetInitTimeout | 服务端snet握手超时时间 | 10秒 |
| ServerSnetWaitTimeout | 服务端snet等待重连超时时间 | 60秒 |

命令行程序启动时会自动把当前进程ID记录到`fastway.pid`，并且监听`SIGTERM`和`SIGINT`。

当网关以前台模式运行时，可以用`Ctrl + C`安全退出网关进程。

网关可以使用`docker`、`nohup`、`start-stop-daemon`来做后台运行。

以`nohup`为例，可以这样启动网关，并把日志输入重定向到文件中：

```
nohup fastway 1>fastway.out 2>fastway.err &
```

网关的日志输出可以使用`logrotate`做自动分卷。

当网关以后台模式运行时，可以使用以下命令安全退出网关进程：

```
kill `cat fastway.pid`
```

此外，还可以对网关进程发送指令来获取网关的运行状态。

以开始[`cpuprof`](https://blog.golang.org/profiling-go-programs)为例，可以通过以下方式开启：

```
echo "cpuprof start" > fastway.cmd && kill -SIGUSR1 `cat fastway.pid`
```

当网关进程收到`SIGUSR1`信号时，将读取`fastway.cmd`文件的内容作为指令执行。

网关指令通过[`cmd`](https://github.com/funny/cmd)包实现，可以自行扩展自定义的指令。

网关支持的完整指令：

| 指令 | 说明 |
| --- | --- |
| cpuprof start | 开始cpuprof |
| cpuprof stop | 停止cpuprof并保存信息到 fastway.cpu.prof，可用`go tool pprof`分析|
| lookup block | 堆栈跟踪的方式查看导致阻塞的地方，保存到 fastway.block.prof |
| lookup gc | 以日志的形式输出gc汇总信息 |
| lookup goroutine | 获取当前所有goroutine的调用栈信息到 fastway.goroutine.prof |
| lookup heap | 获取堆内存分配信息到 fastway.heap.prof，可用`go tool pprof`分析 |
| lookup threadcreate | 堆栈跟踪的方式查看哪些地方在创建操作系统线程，保存到 fastway.threadcreate.prof |

通讯协议
=======

客户端和服务端均采用相同的协议格式与网关进行通讯，消息由4个字节包长度信息和变长的包体组成：

```
+--------+--------+
| Length | Packet |
+--------+--------+
  4 byte   Length
```

每个消息的包体可以再分解为4个字节的虚拟连接ID和变长的消息内容两部分：

```
+---------+-------------+
| Conn ID |   Message   |
+---------+-------------+
   4 byte    Length - 4
```

`Conn ID`是虚拟连接的唯一标识，网关通过识别`Conn ID`来转发消息。

当`Conn ID`为0时，`Message`的内容被网关进一步解析并作为指令进行处理。

网关指令固定以一个字节的指令类型字段开头，指令参数根据指令类型而定：

```
+--------+------------------+
|  Type  |       Args       |
+--------+------------------+
  1 byte    Length - 4 - 1
```

目前支持的指令如下：

| **Type** | **用途** | **Args** | **说明** |
| ---- | ---- | ---- | ---- |
| 0 | Dial | RemoteID | 创建虚拟连接 |
| 1 | Accept | ConnID + RemoteID | 由网关发送给虚拟连接创建者，告知虚拟连接创建成功 |
| 2 | Connect | ConnID + RemoteID | 由网关发送给虚拟连接的被连接方，告知有新的虚拟连接产生 |
| 3 | Refuse | RemoteID | 由网关发送给虚拟连接创建者，告知无法连接到远端 |
| 4 | Close | ConnID | 客户端、服务端、网关都有可能发送次消息 |
| 5 | Ping | 无 | 客户端和服务端主动发送，网关收到后回发同样消息 |

特殊说明：

+ 协议允许客户端主动连接服务端，也允许服务端主动连接客户端。
+ 新建虚拟连接的时候，网关会把虚拟连接信息发送给两端。
+ 客户端连接服务端时：
	+ 网关回发的`Accept`指令中`RemoteID`为服务端ID
	+ 发送给服务端的`Connect`指令中`RemoteID`为客户端ID。
+ 服务端连接客户端时：
	+ 网关回发的`Accept`指令中`RemoteID`为客户端ID
	+ 发送给客户端的`Connect`指令中`RemoteID`为服务端ID。

握手协议
=======

服务端在连接网关时需要先进行握手来验证服务端的合法性。

握手过程如下：

0. 合法的服务端应持有正确的网关秘钥。
1. 网关在接受到新的服务端连接之后，向新连接发送一个`uint64`范围内的随机数作为挑战码。
2. 服务端收到挑战码后，拿出秘钥，计算 `MD5(挑战码 + 秘钥)`，得到验证码。
3. 服务端将验证码和自身节点ID一起发送给网关。
4. 网关收到消息后同样计算 `MD5(挑战码 + 秘钥)`，跟收到的验证码比对是否一致。
5. 验证码比对一致，网关将新连接登记为对应节点ID的连接。

握手下行数据格式：

```
+----------------+
| Challenge Code |
+----------------+
      8 byte
```

握手上行数据格式：

```
+-----------+----------+
|    MD5    | ServerID |
+-----------+----------+
   16 byte     4 byte
```

协议示例
=======

客户端请求网关创建虚拟连接到服务端：

```
+------------+------------+---------+--------------+
| Length = 9 | ConnID = 0 | CMD = 0 | ServerID = 1 |
+------------+------------+---------+--------------+
    4 byte       4 byte      1 byte      4 byte
```

网关响应虚拟连接创建请求：

```
+-------------+------------+---------+---------------+--------------+
| Length = 13 | ConnID = 0 | CMD = 1 | ConnID = 9527 | ServerID = 1 |
+-------------+------------+---------+---------------+--------------+
    4 byte        4 byte      1 byte       4 byte         4 byte
```

网关告知服务端有新的虚拟连接：

```
+-------------+------------+---------+---------------+--------------+
| Length = 13 | ConnID = 0 | CMD = 2 | ConnID = 9527 | ServerID = 1 |
+-------------+------------+---------+---------------+--------------+
    4 byte        4 byte      1 byte       4 byte         4 byte
```

客户端通过虚拟连接发送"Hello"到服务端：

```
+------------+---------------+-------------------+
| Length = 9 | ConnID = 9527 | Message = "Hello" |
+------------+---------------+-------------------+
    4 byte        4 byte            5 byte
```

客户端关闭虚拟连接：

```
+------------+------------+---------+---------------+
| Length = 9 | ConnID = 0 | CMD = 5 | ConnID = 9527 |
+------------+------------+---------+---------------+
    4 byte       4 byte      1 byte      4 byte
```