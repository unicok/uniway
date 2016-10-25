# utest
unit testing utilities for Go

测试API
=======

这是一个单元测试工具库，用来降低Go语言项目单元测试的代码复杂度。

utest支持以下几种测试检查：

```go
// 自定义条件的断言，断言失败时测试立即终止
// 支持变长参数的数据打印，测试失败时这些数据将被打印出来
utest.Assert(t, condition, args...)

// 同Assert，区别是Check失败时测试不会立即终止执行
utest.Check(t, condition, args...)

// 断言v必须为nil，否则测试失败
utest.IsNil(t, v)
// 同IsNil，失败时测试立即终止
utest.IsNilNow(t, v)

// 断言v不能为nil，否则测试失败
utest.NotNil(t, v)
utest.NotNilNow(t, v)

//
// 断言a和b必须相等，此函数只支持以下数据类型：
//   int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, rune, byte, string, []byte
//   []int, []int8, []int16, []int32, []int64, []uint, []uint8, []uint16, []uint32, []uint64, []rune
// 或者实现了utest.Equals接口的自定义类型。
// 当为简单类型时，a和b必须是同类型，只有当b为int类型时，函数内部才会尝试做类型转换，这个设计是为了适应常量值的用法：
//   utest.Equal(t, GetUint64(), 123)
//
utest.Equal(t, a, b)
utest.EqualNow(t, a, b)

// 当Equal无法满足测试需要时可以使用次函数，但是请记得次函数开销较大
utest.DeepEqual(t, a, b)
utest.DeepEqualNow(t, a, b)
```

进程监控
=======

此外，在进行一些复杂的多线程单元测试的时候，可能出现死锁的情况，或者进行benchmark的时候，需要知道过程中内存的增长情况和GC情况。

utest为这些情况提供了一个统一的监控功能，在单元测试运行目录下使用以下方法可以获取到单元测试过程中的信息：

```shell
echo 'lookup goroutine' > utest.cmd
```

以上shell脚本将使utest自动输出goroutine堆栈跟踪信息到`utest.goroutine`文件。

utest支持以下几种监控命令：

```
lookup goroutine  -  获取当前所有goroutine的堆栈跟踪信息，输出到utest.goroutine文件，用于排查死锁等情况
lookup heap       -  获取当前内存状态信息，输出到utest.heap文件，包含内存分配情况和GC暂停时间等
lookup threadcreate - 获取当前线程创建信息，输出到utest.thread文件，通常用来排查CGO的线程使用情况
```

此外你还可以通过注册`utest.CommandHandler`回调来添加自己的监控命令支持。