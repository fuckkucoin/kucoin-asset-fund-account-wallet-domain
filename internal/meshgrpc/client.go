package meshgrpc

import (
	"context"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var pool struct {
	mu    sync.Mutex
	conns map[string]*grpc.ClientConn
}

// Invoke 按 target 调用对端 Mesh.Call。方法、路径、正文与 HTTP 调用相同。
func Invoke(ctx context.Context, target, method, path string, body []byte) (int, []byte, error) {
	conn, err := connOf(target)
	if err != nil {
		return 0, nil, err
	}
	out := new(CallResponse)
	err = conn.Invoke(ctx, fullMethod, &CallRequest{Method: method, Path: path, Body: body}, out, grpc.CallContentSubtype(codecName))
	if err != nil {
		return 0, nil, err
	}
	return int(out.Status), out.Body, nil
}

func connOf(target string) (*grpc.ClientConn, error) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if pool.conns == nil {
		pool.conns = map[string]*grpc.ClientConn{}
	}
	if c := pool.conns[target]; c != nil {
		return c, nil
	}
	c, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	pool.conns[target] = c
	return c, nil
}
