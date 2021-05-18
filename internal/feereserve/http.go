package feereserve

import (
	"net/http"
	"strconv"
	"strings"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
)

// Register 挂载所有 feereserve 路由。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := buildService(rt)
	  addRoutes(v1, svc)
   }

// addRoutes 供时直接注入 Service。
  func addRoutes(v1 *gin.RouterGroup, svc *Service) {
	   v1.POST("/feereserve/reserve", handleReserve(svc))
	  v1.POST("/feereserve/settle", handleSettle(svc))
	  v1.POST("/feereserve/release", handleRelease(svc))
	  v1.GET("/feereserve/withdraw/:wid", handleGetByWithdraw(svc))
	  v1.GET("/feereserve/delta", handleListDelta(svc))
	   v1.GET("/feereserve/user/:uid", handleListByUser(svc))
	   v1.GET("/feereserve/health", handleHealth(svc))
   }

  func buildService(rt *app.Runtime) *Service {
	   if rt == nil {
		   return NewMem()
	  }
	  if rt.SQL != nil {
		  return New(rt.SQL)
	   }
	   return NewMem()
  }

// --- handlers ---

   func handleReserve(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  var body ReserveRequest
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  if strings.TrimSpace(body.WithdrawID) == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "withdraw_id required"})
			   return
		   }
		   r, err := svc.Reserve(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, r)
	  }
   }

  func handleSettle(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   var body SettleRequest
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   if strings.TrimSpace(body.WithdrawID) == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "withdraw_id required"})
			   return
		  }
		  if strings.TrimSpace(body.ActualFee) == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "actual_fee required"})
			   return
		  }
		   res, err := svc.Settle(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, res)
	  }
   }

   func handleRelease(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  var body struct {
			   WithdrawID string `json:"withdraw_id"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   if strings.TrimSpace(body.WithdrawID) == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "withdraw_id required"})
			   return
		  }
		  r, err := svc.Release(c.Request.Context(), body.WithdrawID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, r)
	   }
   }

  func handleGetByWithdraw(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  wid := strings.TrimSpace(c.Param("wid"))
		   if wid == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "withdraw_id required"})
			   return
		   }
		  r, err := svc.GetByWithdraw(c.Request.Context(), wid)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, r)
	  }
  }

   func handleListDelta(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   limit := 100
		  if l := strings.TrimSpace(c.Query("limit")); l != "" {
			   if n, err := strconv.Atoi(l); err == nil && n > 0 {
				   limit = n
			   }
		   }
		   items, err := svc.ListPendingDeltas(c.Request.Context(), limit)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, gin.H{"deltas": items, "count": len(items)})
	  }
   }

   func handleListByUser(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  uid := strings.TrimSpace(c.Param("uid"))
		  limit := 50
		   if l := strings.TrimSpace(c.Query("limit")); l != "" {
			   if n, err := strconv.Atoi(l); err == nil && n > 0 {
				   limit = n
			  }
		   }
		  items, err := svc.ListByUser(c.Request.Context(), uid, limit)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"reserves": items, "count": len(items)})
	   }
   }

   func handleHealth(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  summary,
			  err := svc.HealthSummary(c.Request.Context())
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, summary)
	  }
   }
