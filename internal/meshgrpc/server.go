package meshgrpc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	serviceName = "funding.mesh.v1.Mesh"
	methodName  = "Call"
	fullMethod  = "/" + serviceName + "/" + methodName
	maxBody     = 1 << 20
)

// Server 把内部 gRPC 调用交回本进程已有的 HTTP 处理函数。
type Server interface {
	Call(ctx context.Context, in *CallRequest) (*CallResponse, error)
}

type handlerServer struct {
	h http.Handler
}

func (s handlerServer) Call(_ context.Context, in *CallRequest) (*CallResponse, error) {
	if in == nil {
		return nil, status.Error(codes.InvalidArgument, "empty call")
	}
	code, body, err := Dispatch(s.h, in.Method, in.Path, in.Body)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	return &CallResponse{Status: int32(code), Body: body}, nil
}

// Dispatch 用原方法、路径和正文调用本进程 HTTP。对外路由名和参数不变。
func Dispatch(h http.Handler, method, path string, body []byte) (int, []byte, error) {
	if h == nil {
		return 0, nil, errors.New("http handler unavailable")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	if path == "" {
		path = "/"
	}
	if len(body) > maxBody {
		return http.StatusRequestEntityTooLarge, nil, nil
	}
	var rdr io.Reader = http.NoBody
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, path, rdr)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	raw, _ := io.ReadAll(io.LimitReader(rec.Body, maxBody))
	return rec.Code, raw, nil
}

// NewServer 将 HTTP 处理函数挂到 gRPC Mesh Call 服务（进程内转发）。
func NewServer(h http.Handler) *grpc.Server {
	s := grpc.NewServer()
	mount(s, h)
	return s
}

func mount(s *grpc.Server, h http.Handler) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: serviceName,
		HandlerType: (*Server)(nil),
		Methods: []grpc.MethodDesc{{
			MethodName: methodName,
			Handler:    callHandler,
		}},
	}, handlerServer{h: h})
}

func callHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(CallRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	call := srv.(Server).Call
	if interceptor == nil {
		return call(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: fullMethod}
	out, err := interceptor(ctx, in, info, func(ctx context.Context, req any) (any, error) {
		return call(ctx, req.(*CallRequest))
	})
	return out, err
}
