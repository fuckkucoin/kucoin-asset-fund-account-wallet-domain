// 功能：账户账本领域之 HTTP 接口层。
// 作者：CainReed
// 日期：2020.09.20

package account

import (
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/fundsafety"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New(rt.Ledger).BindSQL(rt.SQL)
	  v1.GET("/accounts", func(c *gin.Context) {
		  rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   user, err := validate.UserID(rawUID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   snap, err := svc.List(c.Request.Context(), user)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, snap)
	   })
	   v1.GET("/accounts/events", func(c *gin.Context) {
		  rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  user, err := validate.UserID(rawUID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"events": svc.Events(user, 50)})
	   })
	  v1.POST("/accounts/open-all", func(c *gin.Context) {
		  var body struct {
			   UserID string `json:"user_id"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		  }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  body.UserID = uid
		   st, err := svc.OpenAll(c.Request.Context(), body.UserID)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"books": st})
	   })
	   v1.POST("/accounts/open", func(c *gin.Context) {
		   var body struct {
			  UserID string `json:"user_id"`
			   Book string `json:"book"`
		  }
		  if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			  return
		   }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   body.UserID = uid
		   book, err := ParseBook(body.Book)
		  if err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			  return
		  }
		  st, err := svc.Open(c.Request.Context(), body.UserID, book)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, st)
	   })
	  v1.POST("/accounts/freeze", func(c *gin.Context) {
		   var body struct {
			  UserID string `json:"user_id"`
			   Book   string `json:"book"`
			  Frozen *bool `json:"frozen"`
			  // Cascade 为真表示企图连带解开其它冻结，科目闸拒绝。
			   Cascade bool `json:"cascade"`
		   }
		  if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		   }
		   uid,
			   err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   body.UserID = uid
		  if body.Frozen == nil {
			   c.JSON(400, gin.H{"error": "frozen required"})
			  return
		  }
		  book, err := ParseBook(body.Book)
		   if err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			  return
		  }
		   st, err := svc.FreezeGated(c.Request.Context(), body.UserID, book, *body.Frozen, body.Cascade && !*body.Frozen)
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, st)
	  })
	  v1.GET("/accounts/freeze-read", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   uid, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  got, err := svc.FreezeReadOf(c.Request.Context(), uid, 50)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, got)
	  })
	   v1.GET("/accounts/subject-audit", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  uid, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   rows, err := svc.SubjectAudits(c.Request.Context(), uid, 50)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"audits": rows})
	   })
	  v1.POST("/accounts/close", func(c *gin.Context) {
		   var body struct {
			   UserID string `json:"user_id"`
			  Book   string `json:"book"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		   }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   body.UserID = uid
		  book, err := ParseBook(body.Book)
		  if err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			  return
		   }
		  if rt.Ledger == nil && !rt.Mem {
			  c.JSON(503, gin.H{"error": "ledger unavailable"})
			  return
		  }
		  st,
			  err := svc.Close(c.Request.Context(), body.UserID, book)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, st)
	   })
	  v1.POST("/accounts/transfers", func(c *gin.Context) {
		  var body struct {
			  UserID string `json:"user_id"`
			  FromBook string `json:"from_book"`
			  ToBook   string `json:"to_book"`
			  Asset    string `json:"asset"`
			  Amount   string `json:"amount"`
			  IdempotencyKey string `json:"idempotency_key"`
		  }
		   if err := c.ShouldBindJSON(&body); err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			  return
		  }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   body.UserID = uid
		  uid, err = validate.UserID(body.UserID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  body.UserID = uid
		  if _, err := validate.Asset(body.Asset); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  if _, err := validate.Idem(body.IdempotencyKey); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   from,
			  err := ParseBook(body.FromBook)
		  if err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			   return
		   }
		   to, err := ParseBook(body.ToBook)
		  if err != nil {
			  c.JSON(400, gin.H{"error": err.Error()})
			   return
		  }
		   amt, err := ledger.ParseAmount(body.Amount)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		   tr, err := svc.Move(c.Request.Context(), MoveReq{
			   UserID: body.UserID, From: from, To: to, Asset: body.Asset,
			   Amount: amt, IdempotencyKey: body.IdempotencyKey,
		  })
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "transfer": ledger.ViewTransfer(tr, "")})
			   return
		  }
		  c.JSON(200, gin.H{"transfer": ledger.ViewTransfer(tr, "")})
	  })
	  registerExtra(v1, rt, svc)
  }

   func registerExtra(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	   v1.GET("/accounts/status", func(c *gin.Context) {
		  rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  user, err := validate.UserID(rawUID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   book, err := ParseBook(c.DefaultQuery("book", "funding"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  st := svc.statusOf(user, book)
		  c.JSON(200, BookState{UserID: user, Book: book, Status: st})
	  })
	  v1.GET("/accounts/book/:book", func(c *gin.Context) {
		  rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   user, err := validate.UserID(rawUID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  book, err := ParseBook(c.Param("book"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  snap, err := svc.List(c.Request.Context(), user)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   rows := make([]BookWithBal, 0)
		  for _, row := range snap.Books {
			   if row.Book == book {
				   rows = append(rows, row)
			   }
		  }
		  c.JSON(200, gin.H{"user_id": user, "book": book, "rows": rows, "status": svc.statusOf(user, book)})
	  })
	   v1.GET("/accounts/hops", func(c *gin.Context) {
		   c.JSON(200, gin.H{"hops": LegalHops(), "host": "account-book.kucoin.net"})
	   })
	   v1.POST("/accounts/check-hop", func(c *gin.Context) {
		  var body struct {
			  UserID string `json:"user_id"`
			   From   string `json:"from"`
			   To     string `json:"to"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   body.UserID = uid
		  from, err := ParseBook(body.From)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   to, err := ParseBook(body.To)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   got,
			  err := svc.CheckHop(body.UserID, from, to)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	  })
	  registerSafety(v1, rt, svc)
   }

  func registerSafety(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  gate := fundsafety.New()
	  if rt != nil {
		   gate.BindLedger(rt.Ledger)
	   }
	  v1.POST("/accounts/refuse-frozen", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   UserID string `json:"user_id"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  body.UserID = uid
		   if _, err := validate.UserID(body.UserID); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   if err := gate.RefuseFrozen(c.Request.Context(), body.UserID); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"ok": true, "host": "account-book.kucoin.net"})
	  })
	  v1.GET("/accounts/books", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   user, err := validate.UserID(rawUID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  snap, err := svc.List(c.Request.Context(), user)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"user_id": user, "books": snap.Books, "host": "account-book.kucoin.net"})
	   })
  }
