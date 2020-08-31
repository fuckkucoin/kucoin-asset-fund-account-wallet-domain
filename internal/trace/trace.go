// 请求追踪。
// 作者: King
// 日期: 2020.08.30
package trace

import (
	"context"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"account-assets.kucoin.net/funding/internal/metrics"
	"google.golang.org/grpc/metadata"
)

type ctxKey struct{}

type Span struct {
	TraceID string
	Name    string
	Start time.Time
}

   func NewTraceID() string { return uuid.NewString() }

  func WithSpan(ctx context.Context, name string) (context.Context, *Span) {
	  id := ID(ctx)
	  if id == "" {
		  id = NewTraceID()
	   }
	   sp := &Span{TraceID: id, Name: name, Start: time.Now()}
	  return context.WithValue(ctx, ctxKey{}, sp), sp
   }

  func ID(ctx context.Context) string {
	   if sp, ok := ctx.Value(ctxKey{}).(*Span); ok && sp != nil {
		  return sp.TraceID
	  }
	  return ""
  }

  func (sp *Span) End(log zerolog.Logger, err error) {
	   if sp == nil {
		  return
	   }
	  metrics.SpanSeconds.WithLabelValues(sp.Name).Observe(time.Since(sp.Start).Seconds())
	   ev := log.Info().Str("trace_id", sp.TraceID).Str("span", sp.Name).Dur("latency", time.Since(sp.Start))
	  if err != nil {
		   ev.Err(err).Msg("span")
		  return
	   }
	  ev.Msg("span")
   }

// HTTP injects X-Trace-Id and logs a span for each request (OTLP hook point).

  func HTTP(log zerolog.Logger) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   tid := c.GetHeader("X-Trace-Id")
		   if tid == "" {
			   tid = NewTraceID()
		  }
		  sp := &Span{TraceID: tid, Name: c.Request.Method + " " + c.FullPath(), Start: time.Now()}
		  ctx := context.WithValue(c.Request.Context(), ctxKey{}, sp)
		   c.Request = c.Request.WithContext(ctx)
		   c.Writer.Header().Set("X-Trace-Id", tid)
		   c.Next()
		   sp.End(log, nil)
	  }
  }

   func Unary(log zerolog.Logger) grpc.UnaryServerInterceptor {
	  return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		   tid := ""
		  if md, ok := metadata.FromIncomingContext(ctx); ok {
			  if v := md.Get("x-trace-id"); len(v) > 0 {
				   tid = v[0]
			  }
		   }
		  if tid == "" {
			   tid = NewTraceID()
		  }
		   sp := &Span{TraceID: tid, Name: info.FullMethod, Start: time.Now()}
		  ctx = context.WithValue(ctx, ctxKey{}, sp)
		   resp, err := handler(ctx, req)
		  sp.End(log, err)
		   return resp, err
	  }
   }
