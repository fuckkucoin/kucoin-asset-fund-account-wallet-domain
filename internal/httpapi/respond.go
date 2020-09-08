// 错误到 HTTP 状态的映射。
// 作者: Span
// 日期: 2020.08.26
package httpapi

import (
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/opsfc"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/trace"
	"bytes"
	"context"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strconv"
	"time"
)

// writeErr 按错误类型映射 HTTP 状态，禁止 handler 自己拼 4xx/5xx。
func writeErr(c *gin.Context, err error) {
	c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
}

// writeEntity 写业务体；有错仍带回实体（幂等冲突时返回已有单）。

func writeEntity(c *gin.Context, key string, v any, err error) {
	if err != nil && v == nil {
		writeErr(c, err)
		return
	}
	status := http.StatusOK
	if err != nil {
		status = apierr.HTTPStatus(err)
	}
	body := gin.H{key: v}
	if err != nil {
		body["error"] = err.Error()
	}
	c.JSON(status, body)
}

func writeTimeout(c *gin.Context, rt *app.Runtime) (context.Context, context.CancelFunc) {
	d := rt.Cfg.HTTPWriteTimeout
	if d <= 0 {
		d = 8 * time.Second
	}
	return context.WithTimeout(c.Request.Context(), d)
}

func queryLimit(c *gin.Context, def, max int) int {
	n, _ := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(def)))
	if n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func publicTransfer(ctx context.Context, _ *app.Runtime, tr *ledger.Transfer) any {
	if tr == nil {
		return nil
	}
	return ledger.ViewTransfer(tr, trace.ID(ctx))
}

func persistKind(rt *app.Runtime) string {
	if rt.Mem {
		return "memory"
	}
	return "mysql+redis"
}

func accessLog(rt *app.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		metrics.HTTPRequests.WithLabelValues(c.FullPath(), c.Request.Method, strconv.Itoa(c.Writer.Status())).Inc()
		metrics.HTTPDuration.WithLabelValues(c.FullPath(), c.Request.Method).Observe(time.Since(start).Seconds())
		rt.Log.Info().
			Str("trace_id", trace.ID(c.Request.Context())).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Dur("latency", time.Since(start)).
			Msg("http")
	}
}

func requireAct(c *gin.Context, act perm.Action) bool {
	if opsfc.IsOpsPath(c.Request.URL.Path) {
		if _, code, err := opsfc.Check(c.Request, act); err != nil {
			c.JSON(code, gin.H{"error": err.Error()})
			return false
		}
		return true
	}
	if err := perm.FromRequest(c.Request, "").Check(act); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return false
	}
	return true
}

// requireAuth IAM JWT 或（内存装配）Bearer；健康检查和 metrics 不走这里。
// 认证关闭时写方法默认拒绝；仅非档且显式 allow_insecure 才放行。
func requireAuth(rt *app.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		kind, profile := "", ""
		if rt != nil && rt.Cfg != nil {
			kind, profile = rt.Cfg.Auth.Kind, rt.Cfg.Profile
		}
		var g *authn.Guard
		if rt != nil {
			g = rt.Auth
		}
		skip, deny := g.Gate(c.Request.Method, authn.ProductionProfile(kind, profile))
		if deny {
			metrics.AuthRejected.Inc()
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": authn.ErrUnauthorized.Error()})
			return
		}
		if skip {
			c.Next()
			return
		}
		body,
			err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": authn.ErrUnauthorized.Error()})
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		if err := rt.Auth.Authenticate(c.Request, body); err != nil {
			metrics.AuthRejected.Inc()
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.Next()
	}
}
