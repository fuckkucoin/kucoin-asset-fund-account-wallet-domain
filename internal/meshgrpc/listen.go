package meshgrpc

import (
	"context"
	"net"
	"net/http"

	"google.golang.org/grpc"
)

// Listen 在 addr 上接受内部 gRPC，并交给 h。h 仍是本进程原来的 HTTP 路由。
func Listen(ctx context.Context, addr string, h http.Handler) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s := grpc.NewServer()
	mount(s, h)
	go func() {
		<-ctx.Done()
		s.GracefulStop()
	}()
	return s.Serve(lis)
}

// Mount 把内部调用挂到已有的 gRPC 服务上。钱包等既有服务接着留在同一进程。
func Mount(s *grpc.Server, h http.Handler) {
	if s == nil {
		return
	}
	mount(s, h)
}
