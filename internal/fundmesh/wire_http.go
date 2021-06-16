// @FileName: wire_http.go
// @Summary: mesh 取消双入口与观察组路由。
// 重点：DELETE 与 POST 都进 CancelWithdraw；ping 不写资金。
// 风险点：旁路调试接口未注册名必须失败。
// @Author: Bronson
// @Created: 2021.06.15

package fundmesh

import (
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// RegisterWire 挂取消双入口、观察 ping 与旁路拨号。
   func RegisterWire(v1 *gin.RouterGroup, eng *Engine, id callerid.Opt) {
	   if v1 == nil || eng == nil {
		   return
	   }
	  h := &wireHandler{eng: eng, id: id}
	  v1.DELETE("/funding/withdrawals/:id", h.cancelPath)
	  v1.POST("/funding/withdrawals/:id/cancel", h.cancelPath)
	   v1.POST("/funding/withdraw/cancel", h.cancelBody)
	   obs := v1.Group("/observe")
	   obs.GET("/ping", h.observePing)
	   obs.POST("/ping", h.observePing)
	   v1.POST("/funding/sidecall/:name", h.sideCall)
	  v1.GET("/funding/mesh/drift", h.drift)
	  v1.GET("/funding/mesh/fallback-proofs", h.fallbacks)
	   attachOutbound(v1, h)
   }

type wireHandler struct {
	eng *Engine
	id callerid.Opt
}

   func (h *wireHandler) cancelPath(c *gin.Context) {
	   h.runCancel(c, strings.TrimSpace(c.Param("id")), "")
  }

   func (h *wireHandler) cancelBody(c *gin.Context) {
	  var body struct {
		  WithdrawID string `json:"withdrawId"`
		  WithdrawalID   string `json:"withdrawalId"`
		   Reason         string `json:"reason"`
		  IdempotencyKey string `json:"idempotency_key"`
	  }
	  if c.Request != nil && c.Request.ContentLength != 0 {
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": CodeInput})
			  return
		  }
	   }
	   id := strings.TrimSpace(body.WithdrawID)
	  if id == "" {
		   id = strings.TrimSpace(body.WithdrawalID)
	  }
	  h.runCancel(c, id, body.Reason)
  }

  func (h *wireHandler) runCancel(c *gin.Context, id, reason string) {
	  if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error(), "code": CodeDenied})
		   return
	  }
	   uid, err := callerid.Require(c, h.id)
	  if err != nil {
		   mw.WriteProblem(c, err)
		  return
	   }
	   var extra struct {
		  Reason         string `json:"reason"`
		   IdempotencyKey string `json:"idempotency_key"`
	   }
	   _ = c.ShouldBindJSON(&extra)
	  if reason == "" {
		   reason = extra.Reason
	   }
	  req := CancelRequest{
		   WithdrawID: id,
		   UserID: uid,
		   Actor:  strings.TrimSpace(perm.FromRequest(c.Request, uid).ID),
		  Reason: reason,
		   IdempotencyKey: extra.IdempotencyKey,
		  Method:         c.Request.Method,
		  Path: c.FullPath(),
	   }
	   if req.Method == http.MethodDelete {
		   req.Entry = EntryDELETE
	  } else {
		   req.Entry = EntryPOST
	  }
	  res, err := h.eng.CancelWithdraw(c.Request.Context(), req)
	   if err != nil {
		   c.JSON(HTTPStatusOf(err), gin.H{"error": err.Error(), "code": res.Code, "result": res})
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"result": res, "host": "query-gateway.kucoin.net"})
  }

   func (h *wireHandler) observePing(c *gin.Context) {
	   before := h.eng.ObserveWrites()
	   ping := h.eng.PingObserve()
	  after := h.eng.ObserveWrites()
	  if after != before {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": ErrObserveWriteForbidden.Error(), "code": CodeDenied})
		   return
	   }
	  c.JSON(http.StatusOK, ping)
   }

   func (h *wireHandler) sideCall(c *gin.Context) {
	  name := c.Param("name")
	  code, err := h.eng.Sides().Call(c.Request.Context(), name)
	  if err != nil {
		   c.JSON(HTTPStatusOf(err), gin.H{"error": err.Error(), "code": CodeOf(err), "name": name})
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"name": name, "status": code})
   }

   func (h *wireHandler) drift(c *gin.Context) {
	   p,
		   err := h.eng.cfg.Validate(c.Request.Context())
	  if err != nil {
		  c.JSON(HTTPStatusOf(err), gin.H{"error": err.Error(), "proof": p})
		   return
	  }
	  c.JSON(http.StatusOK, p)
  }

  func (h *wireHandler) fallbacks(c *gin.Context) {
	  c.JSON(http.StatusOK, gin.H{"proofs": h.eng.Proofs().ListFallback()})
   }
