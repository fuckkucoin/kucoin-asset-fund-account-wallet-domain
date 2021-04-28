package creditoutgate

import (
	"net/url"
	"strconv"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
)

// Register 注册 creditoutgate 路由组（调用方提供 *gin.RouterGroup 和预构建的 *Service）。

  func Register(v1 *gin.RouterGroup, svc *Service) {
	   if v1 == nil || svc == nil {
		  return
	  }
	  g := v1.Group("/creditoutgate")
	  // ── 投递路径 ────────────────────────────────────────────────────
	   g.POST("/enqueue", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  BoardKey  string `json:"board_key"`
			  CreditKey string `json:"credit_key"`
			   Epoch     int64  `json:"epoch"`
			  Token     string `json:"token"`
			  Amount    string `json:"amount"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  res, err := svc.Enqueue(body.BoardKey, body.CreditKey, body.Epoch, body.Token, body.Amount)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, res)
	   })
	   g.POST("/deliver", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body DeliverReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   res, err := svc.Deliver(body.CreditKey, body.Epoch, body.Token)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, res)
	  })
	  // ── 护栏操作 ────────────────────────────────────────────────────
	  g.POST("/fence/flip", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body FenceFlipReq
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   res, err := svc.FenceFlip(body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, res)
	  })
	   g.POST("/fence/close", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  BoardKey string `json:"board_key"`
			  Operator string `json:"operator"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  res, err := svc.CloseFence(body.BoardKey, body.Operator)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, res)
	  })
	  g.POST("/fence/reopen", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   BoardKey string `json:"board_key"`
			   Operator string `json:"operator"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   res, err := svc.ReopenFence(body.BoardKey, body.Operator)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, res)
	  })
	  // ── 查询端点 ────────────────────────────────────────────────────
	  g.GET("/lease/:key", func(c *gin.Context) {
		   key, err := url.PathUnescape(c.Param("key"))
		   if err != nil || key == "" {
			   c.JSON(400, gin.H{"error": ErrCreditKey.Error()})
			   return
		   }
		   l, ok := svc.LeaseByKey(key)
		   if !ok {
			   c.JSON(404, gin.H{"error": ErrLeaseNotFound.Error()})
			   return
		   }
		   c.JSON(200, l)
	  })
	  g.GET("/proof/:key", func(c *gin.Context) {
		  key, err := url.PathUnescape(c.Param("key"))
		   if err != nil || key == "" {
			  c.JSON(400, gin.H{"error": ErrCreditKey.Error()})
			   return
		  }
		  p, ok := svc.ProofByKey(key)
		   if !ok {
			  c.JSON(404, gin.H{"error": "proof not found"})
			   return
		  }
		  c.JSON(200, p)
	   })
	   g.GET("/cases", func(c *gin.Context) {
		  boardKey := c.Query("board_key")
		   creditKey := c.Query("credit_key")
		   var cases []CreditFenceCase
		   switch {
		   case creditKey != "":
			   cases = svc.CasesByKey(creditKey)
		  case boardKey != "":
			  cases = svc.CasesByBoard(boardKey)
		   default:
			   cases = svc.AllCases()
		   }
		   c.JSON(200, gin.H{"cases": cases, "total": len(cases)})
	   })
	   g.GET("/gens", func(c *gin.Context) {
		   gens := svc.GenSnapshot()
		   c.JSON(200, gin.H{"gens": gens, "total": len(gens)})
	   })
	   // ── 操作端点 ────────────────────────────────────────────────────
	   g.POST("/export", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  limitStr := c.Query("limit")
		   limit := 128
		  if limitStr != "" {
			   if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				   limit = n
			   }
		  }
		   exp := svc.ExportProofs(limit)
		  c.JSON(200, exp)
	   })
	  g.POST("/patrol", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActLedgerWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   limitStr := c.Query("limit")
		  limit := 64
		  if limitStr != "" {
			  if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				  limit = n
			  }
		  }
		   res := svc.Patrol(limit)
		  nets := svc.PatrolAllBoards()
		  c.JSON(200, gin.H{
			   "patrol":           res,
			   "safety_net_count": len(nets),
		   })
	   })
	  g.POST("/deliver/pending", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  limitStr := c.Query("limit")
		   limit := 32
		  if limitStr != "" {
			   if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				  limit = n
			  }
		  }
		  res := svc.DeliverPending(limit)
		  c.JSON(200, res)
	   })
   }
