// @FileName: outbound_http.go
// @Summary: 出站提交 HTTP 双入口。
// 重点：POST 与 PUT 都进 SubmitWithdraw；读面不扣减。
// 风险点：幂等键只从规范化路径进入引擎。
// @Author: Cason
// @Created: 2021.06.14

package fundmesh

import (
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func attachOutbound(v1 *gin.RouterGroup, h *wireHandler) {
	   if v1 == nil || h == nil {
		   return
	   }
	   v1.POST("/funding/withdrawals", h.submitOutbound)
	   v1.PUT("/funding/withdrawals", h.submitOutbound)
	   v1.POST("/funding/withdraw/submit", h.submitOutbound)
	   v1.GET("/funding/withdrawals/:id/outbound", h.getOutbound)
	  v1.GET("/funding/mesh/outbound-proofs", h.outboundProofs)
	   v1.GET("/funding/mesh/outbound-inspect", h.outboundInspect)
   }

   func (h *wireHandler) submitOutbound(c *gin.Context) {
	   if err := perm.FromRequest(c.Request, "").Check(perm.ActWithdrawSubmit); err != nil {
		   c.JSON(http.StatusForbidden, gin.H{"error": err.Error(), "code": CodeDenied})
		  return
	   }
	   uid, err := callerid.Require(c, h.id)
	   if err != nil {
		  mw.WriteProblem(c, err)
		   return
	   }
	   var body struct {
		   Asset string `json:"asset"`
		  Amount    string `json:"amount"`
		   Chain     string `json:"chain"`
		   Dest      string `json:"dest"`
		   ToAddress string `json:"to_address"`
		  Memo string `json:"memo"`
		  Tag  string `json:"tag"`
		   IdempotencyKey string `json:"idempotency_key"`
		  WithdrawID string `json:"withdraw_id"`
	   }
	  if c.Request != nil && c.Request.ContentLength != 0 {
		  if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": CodeInput})
			   return
		   }
	  }
	   dest := strings.TrimSpace(body.Dest)
	   if dest == "" {
		   dest = strings.TrimSpace(body.ToAddress)
	  }
	  headerKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	  if headerKey == "" {
		   headerKey = strings.TrimSpace(c.GetHeader("X-Idempotency-Key"))
	   }
	  req := OutboundRequest{
		   UserID: uid,
		   Actor:  strings.TrimSpace(perm.FromRequest(c.Request, uid).ID),
		  Asset:  body.Asset,
		   Amount: body.Amount,
		  Chain: body.Chain,
		  Dest:  dest,
		   Memo:  body.Memo,
		   Tag:   body.Tag,
		   IdempotencyKey: body.IdempotencyKey,
		   HeaderKey:  headerKey,
		  WithdrawID: body.WithdrawID,
		   Method:     c.Request.Method,
		   Path:       c.FullPath(),
		  Entry: EntryPOST,
	  }
	   res,
		   err := h.eng.SubmitWithdraw(c.Request.Context(), req)
	   if err != nil {
		  c.JSON(HTTPStatusOf(err), gin.H{"error": err.Error(), "code": res.Code, "result": res})
		  return
	   }
	  c.JSON(http.StatusOK, gin.H{"result": res, "host": "query-gateway.kucoin.net"})
  }

  func (h *wireHandler) getOutbound(c *gin.Context) {
	   id := strings.TrimSpace(c.Param("id"))
	  key := strings.TrimSpace(c.Query("idempotency_key"))
	   rec, err := h.eng.GetOutbound(c.Request.Context(), key, id)
	  if err != nil {
		   c.JSON(HTTPStatusOf(err), gin.H{"error": err.Error(), "code": CodeOf(err)})
		  return
	   }
	   c.JSON(http.StatusOK, gin.H{"record": resultFromOutbound(rec), "host": "query-gateway.kucoin.net"})
   }

   func (h *wireHandler) outboundProofs(c *gin.Context) {
	   c.JSON(http.StatusOK, h.eng.ExportOutboundProofs())
  }

  func (h *wireHandler) outboundInspect(c *gin.Context) {
	   c.JSON(http.StatusOK, InspectOutbound())
  }
