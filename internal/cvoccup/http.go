package cvoccup

import (
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// Handler HTTP 处理器。
type Handler struct {
	svc        *Service
	rollbackFn RollbackTriggerFn
}

// NewHandler 构造 HTTP 处理器。

  func NewHandler(svc *Service, rollbackFn RollbackTriggerFn) *Handler {
	   return &Handler{svc: svc,
		  rollbackFn: rollbackFn}
   }

// Register 注册路由到 RouterGroup（建议挂载于 /ops/cv-occup）。
  func (h *Handler) Register(rg *gin.RouterGroup) {
	   rg.GET("/occ/:id", h.getOcc)
	  rg.GET("/occ/order/:order_id", h.getOccByOrder)
	   rg.GET("/occs", h.listOccs)
	  rg.POST("/occ/:id/rollback", h.forceRollback)
	   rg.GET("/breaches", h.listBreaches)
	  rg.POST("/breaches/:id/resolve", h.resolveBreach)
	  rg.GET("/summary", h.summary)
   }

// getOcc 按 ID 查询路径占用。

  func (h *Handler) getOcc(c *gin.Context) {
	  id := c.Param("id")
	   occ, err := h.svc.GetOcc(c.Request.Context(), id)
	  if err != nil {
		  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		  return
	  }
	  c.JSON(http.StatusOK, occ)
   }

// getOccByOrder 按订单 ID 查询路径占用。
   func (h *Handler) getOccByOrder(c *gin.Context) {
	  orderID := c.Param("order_id")
	   occ,
		   err := h.svc.GetOccByOrder(c.Request.Context(), orderID)
	  if err != nil {
		   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		  return
	  }
	  c.JSON(http.StatusOK, occ)
   }

// listOccs 列出用户路径占用。
   func (h *Handler) listOccs(c *gin.Context) {
	   userID := c.Query("user_id")
	   activeOnly := c.Query("active_only") == "1" || c.Query("active_only") == "true"
	   limit := 50
	   if lq := c.Query("limit"); lq != "" {
		   if n, err := strconv.Atoi(lq); err == nil && n > 0 {
			  limit = n
		  }
	   }
	   occs, err := h.svc.ListOccByUser(c.Request.Context(), userID, activeOnly, limit)
	   if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		   return
	   }
	  c.JSON(http.StatusOK, gin.H{"occs": occs, "count": len(occs)})
  }

// forceRollback 强制回滚路径占用（ops desk）。
  func (h *Handler) forceRollback(c *gin.Context) {
	  id := c.Param("id")
	  var body struct {
		  Reason string `json:"reason"`
	  }
	   _ = c.ShouldBindJSON(&body)
	   if body.Reason == "" {
		   body.Reason = "MANUAL"
	   }
	  occ, err := h.svc.GetOcc(c.Request.Context(), id)
	  if err != nil {
		   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		   return
	  }
	   if occ.IsTerminal() {
		   c.JSON(http.StatusConflict, gin.H{"error": "occ already terminal", "status": string(occ.Status)})
		   return
	   }
	  if h.rollbackFn != nil {
		  remain := occ.RemainAmount
		   if remain == "" {
			  remain = "0"
		  }
		  if err := h.rollbackFn(c.Request.Context(), occ.ID, occ.OrderID, occ.UserID, "MANUAL", remain); err != nil {
			  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			  return
		   }
	  } else {
		  if _, err := h.svc.Release(c.Request.Context(), id, body.Reason); err != nil {
			   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			   return
		   }
	   }
	  occ, _ = h.svc.GetOcc(c.Request.Context(), id)
	  c.JSON(http.StatusOK, gin.H{"ok": true, "occ": occ})
  }

// listBreaches 列出违约事件（违约板）。

  func (h *Handler) listBreaches(c *gin.Context) {
	   unresolvedOnly := c.Query("unresolved_only") == "1" || c.Query("unresolved_only") == "true"
	   limit := 50
	   if lq := c.Query("limit"); lq != "" {
		  if n, err := strconv.Atoi(lq); err == nil && n > 0 {
			  limit = n
		   }
	   }
	   breaches, err := h.svc.ListBreaches(c.Request.Context(), unresolvedOnly, limit)
	  if err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	  }
	  c.JSON(http.StatusOK, gin.H{"breaches": breaches, "count": len(breaches)})
   }

// resolveBreach 标记违约已解决。

   func (h *Handler) resolveBreach(c *gin.Context) {
	  id := c.Param("id")
	  if err := h.svc.ResolveBreach(c.Request.Context(), id); err != nil {
		  c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	  }
	   c.JSON(http.StatusOK, gin.H{"ok": true})
  }

// occSummaryRow 占用汇总行（ops 视图）。

type occSummaryRow struct {
	UserID   string `json:"user_id"`
	Asset    string `json:"asset"`
	TotalOcc string `json:"total_occ"`
	ActivePaths int `json:"active_paths"`
}

// summary 占用汇总视图。
   func (h *Handler) summary(c *gin.Context) {
	   userID := c.Query("user_id")
	   asset := c.Query("asset")
	   occs, err := h.svc.store.ListActiveOccsByAsset(c.Request.Context(), userID, NormalizeAsset(asset))
	   if err != nil {
		   c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		  return
	  }
	  type aggKey struct{ uid, asset string }
	  byKey := map[aggKey]*occSummaryRow{}
	  for _, o := range occs {
		   k := aggKey{o.UserID, NormalizeAsset(o.FromAsset)}
		  r := byKey[k]
		   if r == nil {
			  r = &occSummaryRow{UserID: o.UserID, Asset: NormalizeAsset(o.FromAsset), TotalOcc: "0"}
			  byKey[k] = r
		  }
		  r.ActivePaths++
		   if ra, err := decimal.NewFromString(o.RemainAmount); err == nil {
			   total, _ := decimal.NewFromString(r.TotalOcc)
			  r.TotalOcc = total.Add(ra).String()
		   }
	   }
	  rows := make([]*occSummaryRow, 0, len(byKey))
	  for _, r := range byKey {
		  rows = append(rows, r)
	   }
	   c.JSON(http.StatusOK, gin.H{"rows": rows, "total": len(rows)})
  }
