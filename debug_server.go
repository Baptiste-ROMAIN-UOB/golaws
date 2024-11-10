// server.go
package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
)

// Worker结构体定义
type Worker struct {
	Client *rpc.Client
}

var (
	workers            = make([]*Worker, 0) // worker列表
	workerMutex        sync.Mutex           // worker访问互斥锁
	currentWorkerIndex int                  // 用于轮询的索引
)

// 游戏参数结构体
type Params struct {
	Width  int
	Height int
}

// 段计算请求结构体
type SegmentRequest struct {
	Start  int
	End    int
	World  [][]byte
	Params Params
}

// 段计算响应结构体
type SegmentResponse struct {
	NewSegment [][]byte
}

// 分布式任务请求
type DistributedTask struct {
	World  [][]byte
	Params struct {
		Width  int
		Height int
	}
}

// 引擎结构体
type Engine struct{}

// 获取可用worker
func getAvailableWorker() (*Worker, error) {
	workerMutex.Lock()
	defer workerMutex.Unlock()

	numWorkers := len(workers)
	if numWorkers == 0 {
		return nil, fmt.Errorf("没有可用的worker")
	}

	// 使用轮询方式选择worker
	worker := workers[currentWorkerIndex]
	currentWorkerIndex = (currentWorkerIndex + 1) % numWorkers

	fmt.Printf("[调试] 选择worker %d / %d\n", currentWorkerIndex, numWorkers)
	return worker, nil
}

// 计算单个段的状态
func (e *Engine) State(req DistributedTask, res *[][]byte) error {
	fmt.Printf("[调试] Server收到计算请求：网格大小%dx%d\n",
		req.Params.Width, req.Params.Height)

	// 验证输入
	if req.World == nil {
		return fmt.Errorf("输入世界为nil")
	}

	if len(req.World) != req.Params.Height {
		return fmt.Errorf("世界高度不匹配: 期望%d, 实际%d",
			req.Params.Height, len(req.World))
	}

	for i, row := range req.World {
		if len(row) != req.Params.Width {
			return fmt.Errorf("第%d行宽度不匹配: 期望%d, 实际%d",
				i, req.Params.Width, len(row))
		}
	}

	workerMutex.Lock()
	numWorkers := len(workers)
	workerMutex.Unlock()

	if numWorkers == 0 {
		return fmt.Errorf("没有可用的worker")
	}

	fmt.Printf("[调试] 准备分配任务给%d个worker处理\n", numWorkers)

	// 计算每个worker应该处理的行数
	segmentHeight := req.Params.Height / numWorkers
	if segmentHeight < 1 {
		segmentHeight = 1
	}

	// 创建结果数组
	*res = make([][]byte, req.Params.Height)
	for i := range *res {
		(*res)[i] = make([]byte, req.Params.Width)
	}

	// 使用channel来收集错误和完成的段
	errChan := make(chan error, numWorkers)
	segmentChan := make(chan struct {
		startY  int
		endY    int
		segment [][]byte
	}, numWorkers)

	numSegments := 0
	// 创建分段并分配工作
	for start := 0; start < req.Params.Height; start += segmentHeight {
		end := start + segmentHeight
		if end > req.Params.Height {
			end = req.Params.Height
		}
		numSegments++

		go func(startY, endY int) {
			fmt.Printf("[调试] Server准备处理段[%d-%d]\n", startY, endY)

			segReq := SegmentRequest{
				Start: startY,
				End:   endY,
				World: req.World,
				Params: Params{
					Width:  req.Params.Width,
					Height: req.Params.Height,
				},
			}

			worker, err := getAvailableWorker()
			if err != nil {
				fmt.Printf("[错误] 获取worker失败: %v\n", err)
				errChan <- err
				return
			}

			fmt.Printf("[调试] 将段[%d-%d]发送给worker处理\n", startY, endY)
			var response SegmentResponse
			err = worker.Client.Call("Worker.State", segReq, &response)
			if err != nil {
				fmt.Printf("[错误] Worker处理段[%d-%d]失败: %v\n", startY, endY, err)
				errChan <- err
				return
			}

			fmt.Printf("[调试] Worker成功完成段[%d-%d]的计算\n", startY, endY)
			segmentChan <- struct {
				startY  int
				endY    int
				segment [][]byte
			}{startY, endY, response.NewSegment}

		}(start, end)
	}

	fmt.Printf("[调试] 等待%d个段完成处理\n", numSegments)

	// 收集结果
	for i := 0; i < numSegments; i++ {
		select {
		case err := <-errChan:
			fmt.Printf("[错误] 段处理失败: %v\n", err)
			return err
		case segment := <-segmentChan:
			fmt.Printf("[调试] 接收到段[%d-%d]的计算结果\n", segment.startY, segment.endY)
			// 复制段结果到最终结果
			for y := segment.startY; y < segment.endY; y++ {
				copy((*res)[y], segment.segment[y-segment.startY])
			}
		}
	}

	fmt.Println("[调试] 所有段计算完成，返回最终结果")
	return nil
}

// 本地计算段（当worker不可用时的备选方案）
func (e *Engine) calculateSegmentLocally(req SegmentRequest, res *SegmentResponse) {
	fmt.Printf("[调试] 本地计算段 [%d-%d]\n", req.Start, req.End)

	// 创建新的段
	res.NewSegment = make([][]byte, req.End-req.Start)
	for i := range res.NewSegment {
		res.NewSegment[i] = make([]byte, req.Params.Width)
	}

	// 为每个细胞计算下一个状态
	for y := req.Start; y < req.End; y++ {
		for x := 0; x < req.Params.Width; x++ {
			aliveNeighbors := countAliveNeighbors(req.World, x, y)
			segY := y - req.Start

			if req.World[y][x] == 255 {
				// 活细胞规则
				if aliveNeighbors == 2 || aliveNeighbors == 3 {
					res.NewSegment[segY][x] = 255
				}
			} else {
				// 死细胞规则
				if aliveNeighbors == 3 {
					res.NewSegment[segY][x] = 255
				}
			}
		}
	}
}

// 计算活细胞邻居数量
func countAliveNeighbors(world [][]byte, x, y int) int {
	aliveCount := 0
	height := len(world)
	width := len(world[0])

	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			neighborX := (x + i + width) % width
			neighborY := (y + j + height) % height
			if world[neighborY][neighborX] == 255 {
				aliveCount++
			}
		}
	}
	return aliveCount
}

// 注册worker
func (e *Engine) RegisterWorker(workerAddr string, success *bool) error {
	worker := &Worker{
		Client: nil,
	}

	// 尝试连接到worker
	client, err := rpc.Dial("tcp", workerAddr)
	if err != nil {
		return fmt.Errorf("连接worker失败: %v", err)
	}

	// 测试连接是否正常
	var pingRes bool
	err = client.Call("Worker.Ping", true, &pingRes)
	if err != nil {
		client.Close()
		return fmt.Errorf("Worker ping测试失败: %v", err)
	}

	worker.Client = client

	workerMutex.Lock()
	workers = append(workers, worker)
	workerMutex.Unlock()

	*success = true
	fmt.Printf("[调试] Worker注册成功: %s (当前worker数量: %d)\n", workerAddr, len(workers))
	return nil
}

// 启动RPC服务器
func startServer() {
	engine := new(Engine)
	rpc.Register(engine)

	fmt.Println("[调试] 正在启动RPC服务器，端口：8080")
	fmt.Println("[调试] 已注册的RPC方法:")
	fmt.Println(" - Engine.State")
	fmt.Println(" - Engine.RegisterWorker")

	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("启动服务器失败: %v", err)
	}
	defer listener.Close()

	fmt.Println("服务器已启动，等待连接...")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("接受连接时出错: %v\n", err)
			continue
		}
		fmt.Printf("[debug] 接受来自 %v 的连接\n", conn.RemoteAddr())
		go func(c net.Conn) {
			fmt.Printf("[调试] 开始处理来自 %v 的RPC连接\n", c.RemoteAddr())
			rpc.ServeConn(c)
			fmt.Printf("[调试] 结束处理来自 %v 的RPC连接\n", c.RemoteAddr())
		}(conn)
	}
}

func main() {
	startServer()
}
