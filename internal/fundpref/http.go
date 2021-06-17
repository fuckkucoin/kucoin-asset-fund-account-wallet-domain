package fundpref

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, book *Book, id callerid.Opt) {
	   if v1 == nil {
		  return
	   }
	  if book == nil {
		  book = New()
	  }
	   v1.GET("/assets/preferences", func(c *gin.Context) {
		  uid, err := callerid.Require(c, id)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   p,
			   err := book.Get(c.Request.Context(), uid)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"preference": p, "host": "query-gateway.kucoin.net"})
	  })
	   v1.PUT("/assets/preferences", func(c *gin.Context) {
		   uid, err := callerid.Require(c, id)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  req, ok := BindWrite(c)
		  if !ok {
			  return
		   }
		   actor := strings.TrimSpace(perm.FromRequest(c.Request, uid).ID)
		   p, err := book.Put(c.Request.Context(), actor, uid, req)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"preference": p, "host": "query-gateway.kucoin.net"})
	   })
	  v1.GET("/assets/preferences/config", func(c *gin.Context) {
		   uid,
			  err := callerid.Require(c, id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   cfg, err := book.SnapshotConfig(c.Request.Context(), uid)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"config": cfg, "host": "query-gateway.kucoin.net"})
	   })
	   v1.GET("/assets/preferences/default", func(c *gin.Context) {
		  uid, err := callerid.Require(c, id)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   view, err := book.ResolveDefaultView(c.Request.Context(), uid)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"default": view, "host": "query-gateway.kucoin.net"})
	  })
	  v1.GET("/assets/preferences/audit", func(c *gin.Context) {
		  uid, err := callerid.Require(c, id)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   actor := perm.FromRequest(c.Request, uid)
		  target := uid
		   if actor.Check(perm.ActOpsAdmin) == nil {
			  if q := strings.TrimSpace(c.Query("user_id")); q != "" {
				   target = q
			   }
		  }
		  limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
		   rows,
			  err := book.ListAudit(c.Request.Context(), target, limit)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"user_id": target, "items": rows, "host": "query-gateway.kucoin.net"})
	   })
   }

// BindWrite 读 PUT 体并写 Problem。行情/撮合键失败即关。

  func BindWrite(c *gin.Context) (WriteReq, bool) {
	  req, err := ReadWrite(c)
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return WriteReq{}, false
	  }
	   return req, true
   }

// ReadWrite 读 PUT 体，不写响应。供 kcfund 兼容面包一层 {code,msg}。
   func ReadWrite(c *gin.Context) (WriteReq, error) {
	  if c == nil || c.Request == nil {
		  return WriteReq{}, ErrCorrupt
	  }
	  raw, err := io.ReadAll(c.Request.Body)
	   if err != nil {
		   return WriteReq{}, ErrCorrupt
	   }
	   return ParseWrite(raw)
   }

// ParseWrite 校验写体。损坏 JSON 与行情键失败即关。
   func ParseWrite(raw []byte) (WriteReq, error) {
	   if err := RejectMarket(raw); err != nil {
		  return WriteReq{}, err
	  }
	   var req WriteReq
	   if len(raw) == 0 {
		   return req, nil
	  }
	   if err := json.Unmarshal(raw, &req); err != nil {
		  return WriteReq{}, ErrCorrupt
	   }
	   return req, nil
   }
