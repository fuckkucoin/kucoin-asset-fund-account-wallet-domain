// @FileName: remote.go
// @Summary: 网关经注册中心拨号 account / ledger / kyt 的真实跨进程路径。
// @Author: TianpengLan
// @Created: 2021.06.30

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/svcdial"
	"account-assets.kucoin.net/funding/internal/svcreg"
	"github.com/gin-gonic/gin"
)

// Remote 网关侧跨服务客户端。拨号失败结构化日志 + 退避，不把失败写成 200。
type Remote struct {
	dial *svcdial.Client
}

   func NewRemote(rt *app.Runtime) *Remote {
	   if rt == nil || rt.Dial == nil {
		   return &Remote{}
	  }
	   return &Remote{dial: rt.Dial}
  }

type remoteOverview struct {
	UserID   string          `json:"user_id"`
	Accounts json.RawMessage `json:"accounts,omitempty"`
	Bills  json.RawMessage `json:"bills,omitempty"`
	Source string          `json:"source"`
}

// OverviewRemote 先打 account，再打 ledger；任一拨号失败记错误并返回可诊断载荷，不伪装成功。

  func (r *Remote) OverviewRemote(ctx context.Context, userID string) (remoteOverview, int, error) {
	  out := remoteOverview{UserID: userID, Source: "remote"}
	   if r == nil || r.dial == nil {
		  return out, 0, svcreg.ErrClosed
	   }
	   codeA, rawA, errA := r.dial.GetAccounts(ctx, userID)
	  if errA != nil {
		   return out, codeA, errA
	  }
	   out.Accounts = rawA
	   codeL, rawL, errL := r.dial.GetLedgerBills(ctx, userID, 50)
	   if errL != nil {
		  return out, codeL, errL
	  }
	  out.Bills = rawL
	  if codeA >= 500 {
		  return out, codeA, nil
	   }
	   return out, codeL, nil
   }

// ScreenKYTRemote 出金/入金旁路筛查；KYT 拨号失败返回错误由调用方决定拦还是记审计。

   func (r *Remote) ScreenKYTRemote(ctx context.Context, payload []byte) (int, []byte, error) {
	  if r == nil || r.dial == nil {
		   return 0, nil, svcreg.ErrClosed
	  }
	   return r.dial.ScreenKYT(ctx,
		   payload)
  }

// RegisterRemote 挂网关跨服务探测与聚合路由。
   func RegisterRemote(v1 *gin.RouterGroup, rt *app.Runtime) {
	   if v1 == nil {
		  return
	  }
	  rem := NewRemote(rt)
	   v1.GET("/remote/overview", func(c *gin.Context) {
		   user := c.Query("user_id")
		   if user == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
			   return
		   }
		  ov, code, err := rem.OverviewRemote(c.Request.Context(), user)
		   if err != nil {
			   if rt != nil {
				   rt.Log.Error().Err(err).Str("user_id", user).Msg("gateway remote overview")
			   }
			   c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "user_id": user})
			   return
		   }
		  if code == 0 {
			  code = http.StatusOK
		  }
		   c.JSON(code, ov)
	   })
	  v1.POST("/remote/kyt/screen", func(c *gin.Context) {
		  body, err := c.GetRawData()
		  if err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "body required"})
			  return
		   }
		   code, raw, err := rem.ScreenKYTRemote(c.Request.Context(), body)
		   if err != nil {
			  if rt != nil {
				   rt.Log.Error().Err(err).Msg("gateway remote kyt")
			   }
			  c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			  return
		  }
		   if code == 0 {
			  code = http.StatusOK
		   }
		  c.Data(code, "application/json", raw)
	   })
	   v1.GET("/remote/health/:service", func(c *gin.Context) {
		  svc := c.Param("service")
		   if rem.dial == nil {
			  c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dial unavailable"})
			  return
		  }
		  code, raw, err := rem.dial.HealthOf(c.Request.Context(), svc)
		   if err != nil {
			  if rt != nil {
				   rt.Log.Error().Err(err).Str("service", svc).Msg("gateway remote health")
			   }
			   c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "service": svc})
			  return
		   }
		  c.Data(code, "application/json", raw)
	   })
  }
