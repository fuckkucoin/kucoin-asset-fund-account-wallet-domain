// @FileName: http.go
// @Summary: 资金网关路由骨架与一致性分片之 HTTP 接口层。
// @Author: TianpengLan
// @Created: 2021.06.29

package gateway

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/fundsafety"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/snap"
	"account-assets.kucoin.net/funding/internal/stmt"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

var errBookUnavailable = errors.New("funding book unavailable")

  func idOpt(rt *app.Runtime) callerid.Opt {
	  if rt == nil || rt.Cfg == nil {
		   return callerid.Opt{}
	  }
	  return callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
  }

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  RegisterRemote(v1, rt)
	   svc := New(rt.Ledger, rt.Wallet)
	   v1.GET("/balances", func(c *gin.Context) {
		   user := callerid.Resolve(c, idOpt(rt))
		  rows, err := svc.Unified(c.Request.Context(), user)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"user_id": user, "view": "unified", "balances": rows})
	   })
	  v1.GET("/bills", func(c *gin.Context) {
		   user := callerid.Resolve(c, idOpt(rt))
		   limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		   if limit <= 0 || limit > 200 {
			  limit = 50
		  }
		   list, err := svc.Bills(c.Request.Context(), user, limit)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		  if list == nil {
			   list = []ledger.Bill{}
		   }
		   c.JSON(200, gin.H{"user_id": user, "bills": list})
	  })
	  v1.GET("/accounts", func(c *gin.Context) {
		   user := callerid.Resolve(c, idOpt(rt))
		  books, err := svc.Accounts(c.Request.Context(), user)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{"user_id": user, "accounts": books})
	  })
	   v1.GET("/bills/statement", func(c *gin.Context) {
		   user := callerid.Resolve(c, idOpt(rt))
		  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		  rep, err := svc.Statement(c.Request.Context(), BillQuery{
			   UserID: user, Asset: c.Query("asset"), Book: c.Query("book"),
			  AppType: c.Query("type"), Limit: limit,
		  })
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, rep)
	   })
	  v1.GET("/statement", func(c *gin.Context) {
		  user := callerid.Resolve(c, idOpt(rt))
		  bals, err := svc.Unified(c.Request.Context(), user)
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		   books, err := svc.Accounts(c.Request.Context(), user)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   extra, mm, err := rt.InflightFrozen(c.Request.Context(), user)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  rep, err := ledger.ReconcileWith(c.Request.Context(), rt.Store, user, extra)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   ledger.MergeMismatches(rep, mm)
		  c.JSON(200, gin.H{
			  "user_id": user,
			   "books_match": rep.OK,
			   "balances":    bals,
			  "accounts":  books,
			   "reconcile": rep,
			  "view": "funding+trading+margin+futures+earn+wallet",
		  })
	  })
	  registerExtra(v1, rt, svc)
	  registerTravel(v1, rt)
	  registerPref(v1, rt)
  }

  func registerExtra(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	   v1.GET("/movements", func(c *gin.Context) {
		   user,
			  err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   limit := validate.Limit(0,
			  50, 200)
		  if raw := c.Query("limit"); raw != "" {
			   n := 0
			   for _, r := range raw {
				  if r < '0' || r > '9' {
					   n = 0
					   break
				   }
				  n = n*10 + int(r-'0')
			   }
			  limit = validate.Limit(n, 50, 200)
		  }
		  list, err := rt.Ledger.List(c.Request.Context(), user, limit)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  views := make([]any, 0, len(list))
		   for _, tr := range list {
			  views = append(views, ledger.ViewTransfer(tr, ""))
		  }
		   c.JSON(200, gin.H{"user_id": user, "movements": views, "host": "query-gateway.kucoin.net"})
	   })
	  v1.GET("/books/funding", func(c *gin.Context) {
		  user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  books, err := svc.Accounts(c.Request.Context(), user)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   out := make([]ledger.BookView, 0)
		  for _, b := range books {
			  if b.Book == "wallet" || b.Book == "spot" || b.Subject == "funding" || b.Subject == "wallet" {
				   out = append(out, b)
			   }
		  }
		   c.JSON(200, gin.H{"user_id": user, "books": out})
	   })
	   v1.GET("/movements/page", func(c *gin.Context) {
		   user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   limit := validate.Limit(0, 20, 100)
		   list, err := rt.Ledger.List(c.Request.Context(), user, limit)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  next := ""
		   if len(list) > 0 {
			  next = list[len(list)-1].ID
		   }
		   c.JSON(200, gin.H{"user_id": user, "items": list, "next": next, "done": len(list) < limit})
	  })
	   sn := snap.New(rt.Ledger, rt.Store)
	  v1.POST("/snapshots", func(c *gin.Context) {
		   uid, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   got, err := sn.Capture(c.Request.Context(), uid)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	   })
	  v1.GET("/bills/export", func(c *gin.Context) {
		  uid, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  format, err := stmt.ParseFormat(c.DefaultQuery("format", "json"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  bills,
			   err := svc.Bills(c.Request.Context(), uid, validate.Limit(0, 200, 500))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   rep, err := stmt.Build(uid, bills, stmt.Filter{Asset: c.Query("asset"), Book: c.Query("book")})
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   exp, err := stmt.ExportReport(rep, format)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, exp)
	   })
	  registerFundingOps(v1, rt, svc)
	  v1.GET("/books/proof", func(c *gin.Context) {
		   user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  asset := c.DefaultQuery("asset", "USDT")
		  got, err := svc.ProveBook(c.Request.Context(), user, asset)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	   })
	  registerSafety(v1, rt, svc)
  }

   func RegisterFunding(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := New(rt.Ledger, rt.Wallet)
	  registerFundingOps(v1, rt, svc)
	  registerTravel(v1, rt)
	   registerPref(v1, rt)
   }

   func registerFundingOps(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  registerFundsurfAssetsRead(v1, rt)
	  registerAssetsSurface(v1, rt, svc)
	  v1.POST("/ops/exports", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   UserID string `json:"user_id"`
			  Kind   string `json:"kind"`
			  Format string `json:"format"`
			   StartAt string `json:"start_at"`
			   EndAt   string `json:"end_at"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  if body.UserID == "" {
			   body.UserID = callerid.Resolve(c, idOpt(rt))
		  }
		  uid, err := validate.UserID(body.UserID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   if rt.Exports == nil {
			   mw.WriteProblem(c, errBookUnavailable)
			  return
		   }
		   start,
			  end := parseExportBound(body.StartAt), parseExportBound(body.EndAt)
		   var job any
		  var qerr error
		  if start.IsZero() && end.IsZero() {
			   job, qerr = rt.Exports.Enqueue(uid, body.Kind, body.Format)
		  } else {
			  job, qerr = rt.Exports.EnqueueWindow(uid, body.Kind, body.Format, start, end)
		  }
		  if qerr != nil {
			  mw.WriteProblem(c, qerr)
			  return
		   }
		  c.JSON(200, gin.H{"job": job, "host": "query-gateway.kucoin.net"})
	   })
	  v1.GET("/ops/exports/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   if rt.Exports == nil {
			  mw.WriteProblem(c, errBookUnavailable)
			   return
		   }
		   job, err := rt.Exports.GetFor(callerid.Resolve(c, idOpt(rt)), c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"job": job})
	  })
	   v1.GET("/ops/exports/:id/body", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  if rt.Exports == nil {
			  mw.WriteProblem(c, errBookUnavailable)
			   return
		   }
		   body, err := rt.Exports.GetBody(c.Param("id"), callerid.Resolve(c, idOpt(rt)))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"id": c.Param("id"), "body": body})
	  })
	   v1.POST("/convert", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  UserID    string `json:"user_id"`
			   FromAsset string `json:"from_asset"`
			   ToAsset   string `json:"to_asset"`
			  Amount    string `json:"amount"`
			  Rate      string `json:"rate"`
			  Book string `json:"book"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  if body.UserID == "" {
			  body.UserID = callerid.Resolve(c, idOpt(rt))
		   }
		  uid, err := validate.UserID(body.UserID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  if rt.Convert == nil {
			  mw.WriteProblem(c, errBookUnavailable)
			   return
		   }
		  ord, err := rt.Convert.Place(uid, body.FromAsset, body.ToAsset, body.Amount, body.Rate, body.Book)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"order": ord, "host": "query-gateway.kucoin.net"})
	  })
	   v1.GET("/convert", func(c *gin.Context) {
		   user := callerid.Resolve(c, idOpt(rt))
		  if _, err := validate.UserID(user); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  if rt.Convert == nil {
			   mw.WriteProblem(c, errBookUnavailable)
			  return
		   }
		   c.JSON(200, gin.H{"orders": rt.Convert.List(user, validate.Limit(0, 20, 100))})
	   })
	  v1.GET("/convert/:id", func(c *gin.Context) {
		  if rt.Convert == nil {
			   mw.WriteProblem(c, errBookUnavailable)
			  return
		  }
		   ord, err := rt.Convert.GetFor(callerid.Resolve(c, idOpt(rt)), c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"order": ord})
	   })
  }

   func registerSafety(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	   gate := fundsafety.New()
	  if rt != nil {
		  gate.BindLedger(rt.Ledger)
	  }
	  v1.GET("/books/wallet", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   books,
			   err := svc.Accounts(c.Request.Context(), user)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"user_id": user, "books": books, "host": "query-gateway.kucoin.net"})
	   })
	  v1.POST("/query/refuse-frozen", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  UserID string `json:"user_id"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   if body.UserID == "" && rt != nil {
			  body.UserID = callerid.Resolve(c, idOpt(rt))
		  }
		  if err := gate.RefuseFrozen(c.Request.Context(), body.UserID); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"ok": true})
	  })
  }

  func parseExportBound(s string) time.Time {
	   s = strings.TrimSpace(s)
	   if s == "" {
		   return time.Time{}
	  }
	  if t, err := time.Parse(time.RFC3339, s); err == nil {
		  return t.UTC()
	  }
	  return time.Time{}
   }
