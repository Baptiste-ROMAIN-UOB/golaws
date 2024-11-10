server:
cd server
go build
./server

Worker nodes:
cd worker
go run worker.go port IPv4
go run worker.go port IPv4
go run worker.go port IPv4

For example, local:
cd worker
go run worker.go 8081 localhost
go run worker.go 8082 localhost
go run worker.go 8083 localhost

go run main.go

Receive data and graphs
