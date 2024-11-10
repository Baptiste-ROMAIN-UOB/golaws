// worker.go
package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"time"
)

// Params 包含游戏参数
type Params struct {
	Width  int
	Height int
}

// SegmentRequest 包含段计算请求信息
type SegmentRequest struct {
	Start  int
	End    int
	World  [][]byte
	Params Params
}

// SegmentResponse 包含段计算响应信息
type SegmentResponse struct {
	NewSegment [][]byte
}

// Worker 处理计算任务
type Worker struct{}

// Ping 检查 Worker 是否在线
func (w *Worker) Ping(request bool, response *bool) error {
	*response = true
	return nil
}

// countAliveNeighbors 计算一个细胞周围的活细胞数量
func countAliveNeighbors(world [][]byte, x, y int) int {
	aliveCount := 0
	directions := []struct{ dx, dy int }{
		{-1, -1}, {-1, 0}, {-1, 1},
		{0, -1}, {0, 1},
		{1, -1}, {1, 0}, {1, 1},
	}

	for _, dir := range directions {
		neighborX := (x + dir.dx + len(world[0])) % len(world[0])
		neighborY := (y + dir.dy + len(world)) % len(world)
		if world[neighborY][neighborX] == 255 {
			aliveCount++
		}
	}
	return aliveCount
}

// State 计算细胞的新状态
func (w *Worker) State(req SegmentRequest, res *SegmentResponse) error {
	log.Printf("[调试] Worker 收到计算请求：段 [%d-%d]，网格大小：%dx%d", req.Start, req.End, req.Params.Width, req.Params.Height)

	newSegment := make([][]byte, req.End-req.Start)
	for i := range newSegment {
		newSegment[i] = make([]byte, req.Params.Width)
	}

	for y := req.Start; y < req.End; y++ {
		for x := 0; x < req.Params.Width; x++ {
			aliveNeighbors := countAliveNeighbors(req.World, x, y)
			segY := y - req.Start

			if req.World[y][x] == 255 {
				if aliveNeighbors == 2 || aliveNeighbors == 3 {
					newSegment[segY][x] = 255
				}
			} else if aliveNeighbors == 3 {
				newSegment[segY][x] = 255
			}
		}
	}

	log.Printf("[调试] Worker 完成段 [%d-%d] 的计算", req.Start, req.End)
	res.NewSegment = newSegment
	return nil
}

// registerWithServer 尝试向服务器注册 Worker
func registerWithServer(serverAddr, workerAddr string) error {
	client, err := net.DialTimeout("tcp", serverAddr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("连接服务器失败: %v", err)
	}
	defer client.Close()

	rpcClient := rpc.NewClient(client)
	var success bool
	if err := rpcClient.Call("Engine.RegisterWorker", workerAddr, &success); err != nil || !success {
		return fmt.Errorf("Worker 注册失败: %v", err)
	}
	log.Printf("[调试] Worker 注册成功：%s", workerAddr)
	return nil
}

// startWorkerServer 启动 Worker 的 RPC 服务器
func startWorkerServer(port string, worker *Worker) error {
	if err := rpc.Register(worker); err != nil {
		return fmt.Errorf("Worker 注册失败: %v", err)
	}

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("启动 Worker 监听失败: %v", err)
	}
	defer listener.Close()

	log.Printf("Worker 已启动，监听端口：%s", port)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("接受连接时出错: %v", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}

// retryWithBackoff 尝试注册到服务器，带有指数退避
func retryWithBackoff(serverAddr, workerAddr string) {
	const maxRetries = 5
	for i := 0; i < maxRetries; i++ {
		if err := registerWithServer(serverAddr, workerAddr); err == nil {
			return
		}
		backoff := time.Duration(i+1) * 5 * time.Second
		log.Printf("注册失败，%v 后重试 (%d/%d)", backoff, i+1, maxRetries)
		time.Sleep(backoff)
	}
	log.Fatalf("注册失败，已达到最大重试次数")
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("用法: go run worker.go <worker端口> <服务器IP>")
		os.Exit(1)
	}
	workerPort := os.Args[1]
	serverIP := os.Args[2]

	localIP, err := getLocalIP()
	if err != nil {
		log.Fatalf("获取本机 IP 地址失败: %v", err)
	}
	workerAddr := net.JoinHostPort(localIP, workerPort)
	serverAddr := net.JoinHostPort(serverIP, "8080")

	worker := new(Worker)
	go func() {
		if err := startWorkerServer(workerPort, worker); err != nil {
			log.Fatalf("Worker 服务器启动失败: %v", err)
		}
	}()
	retryWithBackoff(serverAddr, workerAddr)

	select {}
}

// getLocalIP 获取本地非环回 IP 地址
func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String(), nil
		}
	}
	return "localhost", nil
}
