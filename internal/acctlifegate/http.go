package acctlifegate

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes 挂载全部 ops API 到指定路由组。

   func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	   g := rg.Group("/acct-life-gate")
	  // ── 世代查询 ─────────────────────────────────────────────────────────────
	  g.GET("/gen", func(c *gin.Context) {
		   userID, book := c.Query("user_id"), c.Query("book")
		  v, err := svc.LifeGenViewFor(c.Request.Context(), userID, book)
		  if err != nil {
			   writeErr(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, v)
	  })
	  // ── 租约 ─────────────────────────────────────────────────────────────────
	   g.POST("/lease/acquire", func(c *gin.Context) {
		  var req AcquireLeaseRequest
		   if !bindJSON(c, &req) {
			   return
		  }
		   lease, err := svc.AcquireLease(c.Request.Context(), req)
		   if err != nil {
			   writeErr(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, lease)
	  })
	  g.GET("/lease/:lease_id", func(c *gin.Context) {
		  lease, err := svc.GetLease(c.Request.Context(), c.Param("lease_id"))
		  if err != nil {
			  writeErr(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, lease)
	   })
	   // ── 在途 Move ─────────────────────────────────────────────────────────────
	  g.POST("/move/bind", func(c *gin.Context) {
		   var req BindMoveRequest
		  if !bindJSON(c, &req) {
			   return
		  }
		  bind, err := svc.BindMove(c.Request.Context(), req)
		  if err != nil {
			   writeErr(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, bind)
	   })
	  g.POST("/move/release", func(c *gin.Context) {
		  var req ReleaseMoveRequest
		  if !bindJSON(c, &req) {
			  return
		  }
		  if err := svc.ReleaseMove(c.Request.Context(), req); err != nil {
			   writeErr(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"ok": true})
	   })
	   g.GET("/move/pending", func(c *gin.Context) {
		  userID, book := c.Query("user_id"), c.Query("book")
		   binds, err := svc.ListPendingMoveBinds(c.Request.Context(), userID, book)
		   if err != nil {
			  writeErr(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"binds": binds, "count": len(binds)})
	  })
	   // ── 漂移案件 ──────────────────────────────────────────────────────────────
	   g.GET("/drift/summary", func(c *gin.Context) {
		   userID, book := c.Query("user_id"), c.Query("book")
		  summary, err := svc.BookDriftSummaryFor(c.Request.Context(), userID, book)
		   if err != nil {
			   writeErr(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, summary)
	  })
	   g.POST("/drift/report", func(c *gin.Context) {
		  var req ReportDriftRequest
		   if !bindJSON(c, &req) {
			   return
		   }
		   dc, err := svc.ReportDrift(c.Request.Context(), req)
		  if err != nil {
			  writeErr(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, dc)
	  })
	   g.POST("/drift/prove", func(c *gin.Context) {
		  var req ProveDriftRequest
		  if !bindJSON(c, &req) {
			  return
		  }
		  dc, err := svc.ProveDrift(c.Request.Context(), req)
		  if err != nil {
			  writeErr(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, dc)
	  })
	   g.POST("/drift/close", func(c *gin.Context) {
		   var req CloseDriftRequest
		  if !bindJSON(c, &req) {
			  return
		  }
		   dc, err := svc.CloseDrift(c.Request.Context(), req)
		   if err != nil {
			  writeErr(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, dc)
	  })
	  g.GET("/drift/:case_id", func(c *gin.Context) {
		   dc, err := svc.GetDriftCase(c.Request.Context(), c.Param("case_id"))
		  if err != nil {
			  writeErr(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, dc)
	   })
	  // ── 生命周期操作（含世代联动）────────────────────────────────────────────
	  g.POST("/lifecycle/freeze", func(c *gin.Context) {
		  var req FreezeRequest
		   if !bindJSON(c, &req) {
			  return
		   }
		  gen, err := svc.FreezeBook(c.Request.Context(), req)
		  if err != nil {
			   writeErr(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gen)
	  })
	   g.POST("/lifecycle/close", func(c *gin.Context) {
		  var req CloseRequest
		   if !bindJSON(c, &req) {
			   return
		   }
		  gen, err := svc.CloseBook(c.Request.Context(), req)
		  if err != nil {
			  writeErr(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gen)
	  })
	   g.POST("/lifecycle/open", func(c *gin.Context) {
		  var req OpenRequest
		   if !bindJSON(c, &req) {
			  return
		   }
		   gen, err := svc.OpenBook(c.Request.Context(), req)
		  if err != nil {
			   writeErr(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gen)
	  })
	   // ── 借记屏蔽检查 ──────────────────────────────────────────────────────────
	   g.GET("/debit/check", func(c *gin.Context) {
		   userID, book := c.Query("user_id"), c.Query("book")
		   if err := svc.CheckDebitAllowed(c.Request.Context(), userID, book); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"allowed": false, "reason": err.Error()})
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{"allowed": true})
	   })
   }

// ── 辅助函数 ─────────────────────────────────────────────────────────────────

  func bindJSON(c *gin.Context, v any) bool {
	   if err := c.ShouldBindJSON(v); err != nil {
		  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		  return false
	  }
	   return true
   }

  func writeErr(c *gin.Context, err error) {
	  code := http.StatusInternalServerError
	  msg := err.Error()
	  switch {
	   case isNotFound(err):
		   code = http.StatusNotFound
	  case isConflict(err):
		  code = http.StatusConflict
	  case isForbidden(err):
		  code = http.StatusForbidden
	  case isBadRequest(err):
		   code = http.StatusBadRequest
	  }
	  c.JSON(code, gin.H{"error": msg})
  }

   func isNotFound(err error) bool {
	  s := err.Error()
	   return strings.Contains(s, "not found")
  }

   func isConflict(err error) bool {
	   s := err.Error()
	  return strings.Contains(s, "in-flight") ||
		   strings.Contains(s, "mismatch") ||
		   strings.Contains(s, "already exists")
   }

   func isForbidden(err error) bool {
	   s := err.Error()
	  return strings.Contains(s, "blocked") ||
		  strings.Contains(s, "revoked") ||
		  strings.Contains(s, "expired") ||
		   strings.Contains(s, "cannot debit")
  }

   func isBadRequest(err error) bool {
	  s := err.Error()
	   return strings.Contains(s, "invalid") ||
		  strings.Contains(s, "required") ||
		  strings.Contains(s, "must differ")
  }

// parseOptInt 解析查询参数整数，提供默认值。

   func parseOptInt(s string, def int) int {
	   if s == "" {
		  return def
	  }
	  v, err := strconv.Atoi(s)
	  if err != nil {
		  return def
	   }
	  return v
   }

// parseOptTime 解析查询参数 RFC3339 时间，提供默认值。

   func parseOptTime(s string, def time.Time) time.Time {
	  if s == "" {
		  return def
	   }
	   t,
		  err := time.Parse(time.RFC3339, s)
	  if err != nil {
		   return def
	   }
	  return t
   }

// 确保辅助函数被使用（避免编译器警告）。
var _ = parseOptInt
var _ = parseOptTime
