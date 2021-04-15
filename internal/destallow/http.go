package destallow

import (
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, book *Book, pref PrefReader, id callerid.Opt) {
	  if v1 == nil {
		  return
	  }
	  v1.GET("/destallow/entries", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), id)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   page, _ := strconv.Atoi(strings.TrimSpace(c.Query("currentPage")))
		  size, _ := strconv.Atoi(strings.TrimSpace(c.Query("pageSize")))
		  items, total, pageOut, sizeOut, pages := book.ListPage(user, c.Query("chain"), page, size, c.Query("label"))
		  c.JSON(200, gin.H{
			  "user_id": user, "items": book.DecorateEntries(items),
			  "currentPage": pageOut, "pageSize": sizeOut, "totalNum": total, "totalPage": pages,
		  })
	   })
	  v1.GET("/destallow/item", func(c *gin.Context) {
		   user, err := callerid.Bind(c, c.Query("user_id"), id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   e, ok := book.GetExact(user, c.Query("chain"), c.Query("address"))
		  if !ok {
			   if folded, foldOK := foldHit(book, user, c.Query("chain"), c.Query("address")); foldOK {
				   c.JSON(200, gin.H{"found": false, "destallow": true, "destallow_exact": false, "entry": book.ViewOf(folded)})
				   return
			  }
			   c.JSON(404, gin.H{"found": false, "error": "destination not allowlisted"})
			   return
		   }
		  c.JSON(200, gin.H{"found": true, "destallow_exact": true, "entry": book.ViewOf(e)})
	  })
	  v1.GET("/destallow/check", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), id)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, book.InspectGate(c.Request.Context(), pref, user, c.Query("chain"), c.Query("address")))
	  })
	  v1.GET("/destallow/whitelist-only", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), id)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  on := false
		  if pref != nil {
			   on = pref.WhitelistOnly(c.Request.Context(), user)
		  }
		  c.JSON(200, gin.H{"user_id": user, "whitelist_only": on})
	  })
	  registerWrite(v1, book, pref, id)
	   registerDeep(v1, book, pref, id)
  }

  func registerDeep(v1 *gin.RouterGroup, book *Book, pref PrefReader, id callerid.Opt) {
	  if v1 == nil {
		   return
	   }
	  v1.GET("/destallow/cooldown", func(c *gin.Context) {
		   if book == nil {
			  c.JSON(200, gin.H{"seconds": 0, "persisted": false})
			  return
		   }
		  c.JSON(200, gin.H{"seconds": int64(book.Cooldown() / time.Second), "persisted": book.kvOn()})
	  })
	  v1.PUT("/destallow/cooldown", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			   Seconds int64 `json:"seconds"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   if book == nil {
			  c.JSON(503, gin.H{"error": ErrMissing.Error()})
			   return
		   }
		  if err := book.PersistCooldown(c.Request.Context(), time.Duration(body.Seconds)*time.Second); err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{"seconds": int64(book.Cooldown() / time.Second), "persisted": book.kvOn()})
	   })
	  v1.POST("/destallow/import", func(c *gin.Context) {
		   var body struct {
			  UserID string      `json:"user_id"`
			  Rows   []ImportRow `json:"rows"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  user,
			  err := callerid.Bind(c, body.UserID, id)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  if book == nil {
			  c.JSON(503, gin.H{"error": ErrMissing.Error()})
			  return
		   }
		  for i := range body.Rows {
			   if strings.TrimSpace(body.Rows[i].UserID) == "" {
				   body.Rows[i].UserID = user
			  }
		   }
		  rep, err := book.ImportExact(body.Rows, user)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, rep)
	   })
	  v1.GET("/destallow/export", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), id)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   if book == nil {
			  c.JSON(200, gin.H{"items": []EntryView{}})
			  return
		   }
		  c.JSON(200, gin.H{"items": book.ExportFilter(user, c.Query("chain"), c.Query("label"))})
	   })
	   v1.GET("/destallow/precheck", func(c *gin.Context) {
		   user, err := callerid.Bind(c, c.Query("user_id"), id)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   if err := PrecheckWithdraw(c.Request.Context(), book, pref, user, c.Query("chain"), c.Query("address")); err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"ok": false, "error": err.Error(), "breach": true})
			  return
		   }
		  c.JSON(200, gin.H{"ok": true, "user_id": user})
	   })
	   v1.GET("/destallow/breaches", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				   return
			   }
		  }
		  user := strings.TrimSpace(c.Query("user_id"))
		   if user != "" {
			  if uid, err := callerid.Bind(c, user, id); err == nil {
				  user = uid
			   }
		  }
		  limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
		  if book == nil {
			  c.JSON(200, gin.H{"breaches": []AuditRec{}})
			  return
		  }
		   list, err := book.ListBreach(c.Request.Context(), user, c.Query("chain"), limit)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, gin.H{"breaches": list})
	   })
	  v1.POST("/destallow/temp", func(c *gin.Context) {
		  var body struct {
			   UserID string `json:"user_id"`
			   Chain  string `json:"chain"`
			   Address string `json:"address"`
			  Label   string `json:"label"`
			   RiskTag string `json:"risk_tag"`
			  TTLSec  int64  `json:"ttl_sec"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  user, err := callerid.Bind(c, body.UserID, id)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  if book == nil {
			  c.JSON(503, gin.H{"error": ErrMissing.Error()})
			  return
		  }
		  e, err := book.AddTempExact(user, body.Chain, body.Address, body.Label, body.RiskTag, time.Duration(body.TTLSec)*time.Second)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, book.ViewOf(e))
	  })
	   v1.PUT("/destallow/risk-tag", func(c *gin.Context) {
		   var body struct {
			   UserID string `json:"user_id"`
			  Chain   string `json:"chain"`
			   Address string `json:"address"`
			  RiskTag string `json:"risk_tag"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  user, err := callerid.Bind(c, body.UserID, id)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   if book == nil {
			  c.JSON(503, gin.H{"error": ErrMissing.Error()})
			   return
		  }
		  m,
			   err := book.SetRiskTag(user, body.Chain, body.Address, body.RiskTag)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, m)
	  })
	  v1.POST("/destallow/precheck-batch", func(c *gin.Context) {
		   var body struct {
			   UserID string `json:"user_id"`
			  Rows []PrecheckRow `json:"rows"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   user, err := callerid.Bind(c, body.UserID, id)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   if len(body.Rows) > 200 {
			  c.JSON(400, gin.H{"error": "destallow precheck batch exceeds 200"})
			  return
		   }
		  c.JSON(200, gin.H{"items": PrecheckBatch(c.Request.Context(), book, pref, user, body.Rows), "dry_run": true})
	   })
	  v1.GET("/destallow/geo-policy", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		  }
		  if book == nil {
			   c.JSON(200, GeoPolicy{})
			  return
		   }
		  c.JSON(200, book.LoadGeoPolicy(c.Request.Context()))
	   })
	  v1.PUT("/destallow/geo-policy", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body GeoPolicy
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  if book == nil {
			  c.JSON(503, gin.H{"error": ErrMissing.Error()})
			   return
		  }
		  if err := book.PersistGeoPolicy(c.Request.Context(), body); err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, book.LoadGeoPolicy(c.Request.Context()))
	   })
	  v1.GET("/destallow/snapshot", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), id)
		  if err != nil {
			   if perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin) != nil {
				  mw.WriteProblem(c, err)
				  return
			   }
			  user = strings.TrimSpace(c.Query("user_id"))
		   }
		   if book == nil {
			  c.JSON(200, Snapshot{Algo: "sha256"})
			   return
		   }
		   c.JSON(200, book.SnapshotHash(user))
	   })
	   v1.POST("/destallow/snapshot/compare", func(c *gin.Context) {
		  var body struct {
			  UserID string `json:"user_id"`
			  PeerHash string `json:"peer_hash"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  user, err := callerid.Bind(c, body.UserID, id)
		  if err != nil {
			  if perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin) != nil {
				   mw.WriteProblem(c, err)
				  return
			  }
			  user = strings.TrimSpace(body.UserID)
		   }
		   if book == nil {
			   c.JSON(503, gin.H{"error": ErrMissing.Error()})
			  return
		   }
		   got, err := book.CompareSnapshot(user, body.PeerHash)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "snapshot": got})
			  return
		  }
		   c.JSON(200, got)
	   })
	  v1.POST("/destallow/break-glass", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  UserID  string `json:"user_id"`
			  Chain   string `json:"chain"`
			  Address string `json:"address"`
			   ApproverA string `json:"approver_a"`
			  ApproverB string `json:"approver_b"`
			  Reason    string `json:"reason"`
			   TTLSec int64  `json:"ttl_sec"`
			   Actor  string `json:"actor"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   if book == nil {
			  c.JSON(503, gin.H{"error": ErrMissing.Error()})
			   return
		  }
		  got, err := book.PutBreakGlass(c.Request.Context(), BreakGlass{
			  UserID: body.UserID, Chain: body.Chain, Address: body.Address,
			   ApproverA: body.ApproverA, ApproverB: body.ApproverB,
			   Reason: body.Reason, Actor: strings.TrimSpace(body.Actor),
		  }, time.Duration(body.TTLSec)*time.Second)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, got)
	  })
	  v1.GET("/destallow/break-glass", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  if book == nil {
			   c.JSON(404, gin.H{"error": ErrMissing.Error()})
			   return
		   }
		   got, err := book.GetBreakGlass(c.Query("user_id"), c.Query("chain"), c.Query("address"))
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, got)
	   })
	   v1.POST("/destallow/expire-tick", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  if book == nil {
			  c.JSON(200, gin.H{"expired": 0})
			  return
		   }
		   n,
			   err := book.ExpireTick(c.Request.Context())
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"expired": n})
	  })
	  v1.GET("/destallow/audit", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		  }
		   user := strings.TrimSpace(c.Query("user_id"))
		   if user != "" {
			   if uid, err := callerid.Bind(c, user, id); err == nil {
				  user = uid
			  }
		  }
		  limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
		  if book == nil {
			  c.JSON(200, gin.H{"audits": []AuditRec{}})
			  return
		  }
		   list, err := book.ListAudit(c.Request.Context(), user, c.Query("chain"), limit)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"audits": list})
	  })
   }

  func registerWrite(v1 *gin.RouterGroup, book *Book, pref PrefReader, id callerid.Opt) {
	   if v1 == nil {
		  return
	   }
	  v1.POST("/destallow/entries", func(c *gin.Context) {
		   var body struct {
			   UserID string `json:"user_id"`
			   Chain string `json:"chain"`
			   Address string `json:"address"`
			  Label   string `json:"label"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  user,
			  err := callerid.Bind(c, body.UserID, id)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  e, err := book.AddExact(user, body.Chain, body.Address, body.Label)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, book.ViewOf(e))
	   })
	  v1.DELETE("/destallow/entries", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), id)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   if err := book.RemoveExact(user, c.Query("chain"), c.Query("address")); err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"ok": true})
	  })
	  v1.POST("/destallow/whitelist-only", func(c *gin.Context) {
		  var body struct {
			  UserID string `json:"user_id"`
			   Enabled bool `json:"enabled"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   user,
			   err := callerid.Bind(c, body.UserID, id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   w, _ := pref.(PrefWriter)
		  if err := SetPref(c.Request.Context(), w, user, body.Enabled); err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"user_id": user, "whitelist_only": body.Enabled})
	   })
  }
