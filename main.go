package main

import (
	"flag"      // 用于处理命令行参数
	"fmt"       // 用于打印信息
	"os"        // 用于操作系统功能，如接收退出信号
	"os/signal" // 用于处理系统信号
	"runtime"   // 用于管理程序的运行时行为
	"sync"      // 用于实现并发的同步操作
	"syscall"   // 用于处理操作系统系统调用

	"uk.ac.bris.cs/gameoflife/gol" // 引入游戏逻辑模块
	"uk.ac.bris.cs/gameoflife/sdl" // 引入SDL模块（图形界面）
)

// parseFlags 解析命令行参数，返回游戏参数和是否为无窗口模式的标志
func parseFlags() (gol.Params, bool) {
	var params gol.Params // 定义一个 gol.Params 类型的变量来存储游戏参数

	// 设置命令行参数 -t，用于指定线程数量，默认为 8
	flag.IntVar(&params.Threads, "t", 8, "指定线程数，默认为 8。")

	// 设置命令行参数 -w，用于指定图像的宽度，默认为 512
	flag.IntVar(&params.ImageWidth, "w", 512, "指定图像宽度，默认为 512。")

	// 设置命令行参数 -h，用于指定图像的高度，默认为 512
	flag.IntVar(&params.ImageHeight, "h", 512, "指定图像高度，默认为 512。")

	// 设置命令行参数 -turns，用于指定游戏的回合数，默认为 1000
	flag.IntVar(&params.Turns, "turns", 1000, "指定游戏回合数，默认为 1000。")

	// 设置命令行参数 -headless，用于无窗口模式，默认为 false（显示窗口）
	headless := flag.Bool("headless", false, "无窗口模式，如果启用此选项，将禁用 SDL 窗口。")

	// 解析命令行输入的参数
	flag.Parse()

	// 打印参数设置，供用户查看
	fmt.Printf("%-10v %v\n", "Threads", params.Threads)
	fmt.Printf("%-10v %v\n", "Width", params.ImageWidth)
	fmt.Printf("%-10v %v\n", "Height", params.ImageHeight)
	fmt.Printf("%-10v %v\n", "Turns", params.Turns)

	// 返回解析后的游戏参数和无窗口模式标志
	return params, *headless
}

// setupSignalHandler 设置信号处理器，监听退出信号，并在捕捉到退出信号时向 keyPresses 通道发送 'q'
func setupSignalHandler(keyPresses chan<- rune) {
	var once sync.Once                                      // sync.Once 确保信号处理只执行一次
	sigterm := make(chan os.Signal, 1)                      // 定义一个通道来接收系统信号
	signal.Notify(sigterm, syscall.SIGTERM, syscall.SIGINT) // 注册监听 SIGTERM 和 SIGINT 信号

	// 开启一个 goroutine 来处理信号
	go func() {
		once.Do(func() { // 确保以下代码只执行一次
			<-sigterm         // 等待信号传入
			keyPresses <- 'q' // 捕捉到信号时，向 keyPresses 通道发送 'q'，表示退出
		})
	}()
}

// 主函数，程序入口
func main() {
	fmt.Println("程序启动")            // 打印启动信息
	runtime.LockOSThread()         // 锁定主线程，防止操作系统在多线程下自动切换线程
	defer runtime.UnlockOSThread() // 确保程序退出时解锁线程

	params, headless := parseFlags() // 解析命令行参数，获得游戏参数和无窗口模式标志

	keyPresses := make(chan rune, 10)    // 创建一个通道，用于处理键盘输入事件
	events := make(chan gol.Event, 1000) // 创建一个通道，用于处理游戏事件

	setupSignalHandler(keyPresses) // 设置信号处理器，处理系统信号

	go gol.Run(params, events, keyPresses) // 启动游戏逻辑，使用 goroutine 运行游戏

	// 根据是否为无窗口模式，选择启动 SDL 图形界面或无窗口模式
	if headless {
		sdl.RunHeadless(events) // 如果是无窗口模式，则运行无窗口版本的 SDL
	} else {
		sdl.Run(params, events, keyPresses) // 否则运行有图形界面的 SDL
	}
}
