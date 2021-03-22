package clearing

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// RegisterSuspense 注册挂账处置路由到 gin 路由组。

   func RegisterSuspense(v1 *gin.RouterGroup, sb *SuspenseBook) {
	  if sb == nil {
		   sb = NewSuspenseBook()
	   }
	   // 新建挂账分录
	   v1.POST("/clearing/suspense", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   BatchID string `json:"batch_id"`
			   ItemID string `json:"item_id"`
			  ExceptionID string `json:"exception_id"`
			  ReconDiffID string `json:"recon_diff_id"`
			  UserID      string `json:"user_id"`
			   Asset       string `json:"asset"`
			  Amount      string `json:"amount"`
			   Direction   string `json:"direction"`
			   Reason      string `json:"reason"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  actor := perm.FromRequest(c.Request, "").ID
		   entry, err := sb.Park(
			   body.BatchID, body.ItemID, body.ExceptionID, body.ReconDiffID,
			   body.UserID, body.Asset, body.Amount, body.Direction, body.Reason, actor,
		   )
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"suspense": entry})
	  })
	   // 处置：已解决
	   v1.POST("/clearing/suspense/:id/resolve", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  Epoch int64  `json:"epoch"`
			   Token string `json:"token"`
			  Note  string `json:"note"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   actor := perm.FromRequest(c.Request, "").ID
		  entry, err := sb.Resolve(c.Param("id"), actor, body.Note, body.Epoch, body.Token)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"suspense": entry})
	  })
	  // 处置：已拒绝
	  v1.POST("/clearing/suspense/:id/reject", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  Epoch int64 `json:"epoch"`
			  Token string `json:"token"`
			   Note  string `json:"note"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  actor := perm.FromRequest(c.Request, "").ID
		   entry, err := sb.Reject(c.Param("id"), actor, body.Note, body.Epoch, body.Token)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"suspense": entry})
	   })
	   // 查询单条
	  v1.GET("/clearing/suspense/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   entry, err := sb.Get(c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, gin.H{"suspense": entry})
	   })
	  // 列表（可按 batch_id 过滤）
	   v1.GET("/clearing/suspense", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  batchID := c.Query("batch_id")
		   var entries []SuspenseEntry
		   if batchID != "" {
			  entries = sb.ListByBatch(batchID)
		   } else {
			   entries = sb.ListOpen()
		  }
		   c.JSON(http.StatusOK, gin.H{"suspense_entries": entries, "count": len(entries)})
	  })
	  // open 挂账列表
	   v1.GET("/clearing/suspense/open", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		   entries := sb.ListOpen()
		  c.JSON(http.StatusOK, gin.H{"open_suspense": entries, "count": len(entries)})
	   })
   }

// RegisterExceptLink 注册异常关联路由到 gin 路由组。

  func RegisterExceptLink(v1 *gin.RouterGroup, store *ExceptLinkStore) {
	   if store == nil {
		   store = NewExceptLinkStore()
	   }
	   // 建立关联
	   v1.POST("/clearing/except-links", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   ExceptionID string `json:"exception_id"`
			  ReconDiffID string `json:"recon_diff_id"`
			   Note string `json:"note"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  actor := perm.FromRequest(c.Request, "").ID
		  lnk, err := store.Link(body.ExceptionID, body.ReconDiffID, body.Note, actor)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"except_link": lnk})
	   })
	  // 按异常 ID 查关联
	   v1.GET("/clearing/except-links", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  exID := c.Query("exception_id")
		  links := store.ByException(exID)
		  c.JSON(http.StatusOK, gin.H{"except_links": links, "count": len(links)})
	  })
	  // 查询单条
	   v1.GET("/clearing/except-links/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  lnk, err := store.Get(c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{"except_link": lnk})
	  })
	   // 移除关联
	  v1.DELETE("/clearing/except-links/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		  actor := perm.FromRequest(c.Request, "").ID
		   if err := store.Unlink(c.Param("id"), actor); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"ok": true})
	  })
  }
