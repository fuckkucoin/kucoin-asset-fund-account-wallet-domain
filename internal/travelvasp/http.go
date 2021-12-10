package travelvasp

import (
	"encoding/json"
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := Attach(rt, New())
	   if rt != nil {
		  if book, ok := rt.TravelThresh.(*ThresholdBook); ok && book != nil {
			   svc.BindThresholds(book)
		  }
	   }
	   v1.POST("/travel/vasp", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body VASP
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   if err := svc.PutVASP(body); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, body)
	  })
	   v1.POST("/travel/out", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   VASP  string `json:"vasp"`
			  Actor string `json:"actor"`
			  Idem  string `json:"idempotency_key"`
			   Asset string `json:"asset"`
			  Amount string `json:"amount"`
			   Orig Party `json:"originator"`
			  Bene Party `json:"beneficiary"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   env, err := Shape(body.Asset, body.Amount, body.Orig, body.Bene)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  got, err := svc.OpenOut(body.VASP, body.Actor, body.Idem, env)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	   })
	   v1.POST("/travel/advance-out", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			  ID    string `json:"id"`
			   Actor string `json:"actor"`
			  To OutState `json:"to"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := svc.AdvanceOut(body.ID, body.Actor, body.To)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	  })
	   v1.POST("/travel/in", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			   VASP string `json:"vasp"`
			   Actor string `json:"actor"`
			  Idem   string `json:"idempotency_key"`
			   Asset  string `json:"asset"`
			   Amount string `json:"amount"`
			   Orig Party `json:"originator"`
			  Bene Party `json:"beneficiary"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  env, err := Shape(body.Asset, body.Amount, body.Orig, body.Bene)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   got, err := svc.OpenIn(body.VASP, body.Actor, body.Idem, env)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	   v1.POST("/travel/advance-in", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLClose); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  ID string `json:"id"`
			   Actor string `json:"actor"`
			   To InState `json:"to"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   got, err := svc.AdvanceIn(body.ID, body.Actor, body.To)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	   })
	   v1.GET("/travel/vasps", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  page, _ := strconv.Atoi(strings.TrimSpace(c.Query("currentPage")))
		  size, _ := strconv.Atoi(strings.TrimSpace(c.Query("pageSize")))
		   if page > 0 || size > 0 || c.Query("name") != "" || c.Query("lei") != "" || c.Query("did") != "" {
			   c.JSON(200, svc.SearchVASP(ResolveQuery{
				   Code: c.Query("code"), DID: c.Query("did"), LEI: c.Query("lei"),
				  Name: c.Query("name"), Country: c.Query("country"),
			  }, page, size))
			  return
		   }
		   c.JSON(200, gin.H{"vasps": svc.ListVASP(), "host": "kyt.kucoin.net"})
	   })
	   v1.GET("/travel/messages", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"messages": svc.List(c.Query("dir"), 50)})
	  })
	   v1.GET("/travel/completeness", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   got, err := svc.Completeness()
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	  RegisterReads(v1, rt, svc)
	  RegisterThresholds(v1, rt)
	   RegisterCustomer(v1, rt)
	   if rt != nil {
		  if book, ok := rt.TravelThresh.(*ThresholdBook); ok && book != nil {
			  svc.BindThresholds(book)
		  }
		   if svc.outbox != nil {
			  svc.StartOutbox(rt.Log)
		  }
	  }
   }

// RegisterReads 出站案件与对手解析读面。gateway 可再挂 /funding/travel/*。

  func RegisterReads(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  if v1 == nil {
		  return
	   }
	   if svc == nil {
		   svc = Attach(rt, New())
	  }
	   v1.GET("/travel/vasp/resolve", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  got, err := svc.Resolve(ResolveQuery{
			   Code: c.Query("code"), DID: c.Query("did"), LEI: c.Query("lei"),
			  Name: c.Query("name"), Country: c.Query("country"), Type: c.Query("type"),
		  })
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"vasp": got, "host": "kyt.kucoin.net"})
	   })
	  v1.GET("/travel/cases", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		  }
		  uid := strings.TrimSpace(c.Query("user_id"))
		   if uid == "" && rt != nil && rt.Cfg != nil {
			  uid = callerid.Resolve(c, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   }
		   c.JSON(200, gin.H{"cases": svc.ListCases(uid, c.Query("status"), 50), "host": "kyt.kucoin.net"})
	  })
	   v1.GET("/travel/cases/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		  }
		   got, err := svc.GetCase(c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  uid := ""
		  if rt != nil && rt.Cfg != nil {
			   uid = callerid.Resolve(c, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  }
		   if uid != "" && got.UserID != uid {
			  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
				   c.JSON(403, gin.H{"error": "travel-rule case forbidden"})
				   return
			  }
		  }
		  c.JSON(200, got)
	  })
	   v1.POST("/travel/cases/advance", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLClose); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		   }
		  var body struct {
			  ID    string `json:"id"`
			   Actor string `json:"actor"`
			  To   CaseStatus `json:"to"`
			   Note string     `json:"note"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.AdvanceCase(body.ID, body.Actor, body.To, body.Note)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	   v1.GET("/travel/cases/queue", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				   return
			   }
		  }
		  page, _ := strconv.Atoi(strings.TrimSpace(c.Query("currentPage")))
		   size, _ := strconv.Atoi(strings.TrimSpace(c.Query("pageSize")))
		  c.JSON(200, svc.ListQueue(c.Query("status"), page, size))
	   })
	  v1.GET("/travel/adapters", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		  }
		  hub := svc.Adapters()
		  items := make([]gin.H, 0)
		  if hub != nil {
			  for _, proto := range hub.List() {
				   a, _ := hub.Get(proto)
				   ep := ""
				  if ha, ok := a.(*HTTPAdapter); ok && ha != nil {
					  ep = ha.Endpoint()
				   }
				  items = append(items, gin.H{"protocol": proto, "wired": a != nil, "endpoint_set": strings.TrimSpace(ep) != ""})
			   }
		   }
		  c.JSON(200, gin.H{"adapters": items, "host": "kyt.kucoin.net"})
	  })
	   v1.GET("/travel/policies", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		   }
		  book := svc.Policies()
		  rows := []CorridorPolicy{}
		   if book != nil {
			  rows = book.List()
		   }
		   c.JSON(200, gin.H{"policies": rows, "host": "kyt.kucoin.net"})
	   })
	  v1.PUT("/travel/policies", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		  }
		  var body CorridorPolicy
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   book := svc.Policies()
		  if book == nil {
			   book = NewPolicyBook()
			  svc.BindPolicy(book)
		  }
		   if err := book.Put(c.Request.Context(), body); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, body)
	  })
	  v1.POST("/travel/policies/reload", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			   }
		  }
		  book := svc.Policies()
		  if book == nil {
			   c.JSON(200, gin.H{"ok": true, "reloaded": 0})
			  return
		  }
		  if err := book.Reload(c.Request.Context()); err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"ok": true, "reloaded": len(book.List())})
	  })
	   v1.POST("/travel/vasp/cache/invalidate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		  }
		   svc.InvalidateVASPCache()
		   c.JSON(200, gin.H{"ok": true})
	  })
	  v1.GET("/travel/outbox/poison", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			   }
		  }
		   limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
		   list, err := svc.ListPoison(c.Request.Context(), limit)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"items": list})
	   })
	   v1.POST("/travel/outbox/replay", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		   }
		  var body struct {
			   CaseID string `json:"case_id"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   got, err := svc.ReplayOutbox(c.Request.Context(), body.CaseID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	  v1.POST("/travel/cases/evidence", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		   }
		   var body EvidenceMeta
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   got, err := svc.PutEvidence(c.Request.Context(), body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	  })
	   v1.GET("/travel/cases/:id/evidence", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		  }
		  list, err := svc.ListEvidence(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"items": list})
	  })
	  v1.GET("/travel/fallbacks", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		  }
		   rows := []FallbackList{}
		   if book := svc.Fallbacks(); book != nil {
			   rows = book.List()
		  }
		  c.JSON(200, gin.H{"fallbacks": rows, "host": "kyt.kucoin.net"})
	   })
	  v1.PUT("/travel/fallbacks", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			   }
		  }
		  var body FallbackList
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   book := svc.Fallbacks()
		  if book == nil {
			  book = NewFallbackBook()
			   svc.BindFallback(book)
		   }
		  if err := book.Put(c.Request.Context(), body); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, body)
	   })
	   v1.GET("/travel/sunrise", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		   }
		  rows := []SunriseRow{}
		  if book := svc.Sunrise(); book != nil {
			  rows = book.List()
		   }
		   c.JSON(200, gin.H{"sunrise": rows, "host": "kyt.kucoin.net"})
	  })
	   v1.PUT("/travel/sunrise", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		   }
		  var body SunriseRow
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   book := svc.Sunrise()
		   if book == nil {
			   book = NewSunriseBook()
			   svc.BindSunrise(book)
		  }
		  if err := book.Put(c.Request.Context(), body); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, body)
	   })
	   v1.GET("/travel/cases/sla", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				   return
			   }
		  }
		  if id := strings.TrimSpace(c.Query("id")); id != "" {
			  got, err := svc.InspectSLA(id)
			  if err != nil {
				   mw.WriteProblem(c, err)
				  return
			   }
			   c.JSON(200, got)
			  return
		   }
		  c.JSON(200, gin.H{"items": svc.ListSLA(c.Query("status"), 50)})
	  })
	  v1.POST("/travel/cases/bulk-advance", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLClose); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		  }
		   var body struct {
			  Actor string `json:"actor"`
			   Items []BulkAdvanceItem `json:"items"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  rep, err := svc.BulkAdvance(body.Actor, body.Items)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, rep)
	   })
	   v1.POST("/travel/webhook/confirm", func(c *gin.Context) {
		  raw, err := c.GetRawData()
		   if err != nil {
			   c.JSON(400, gin.H{"error": err.Error(), "acked": false})
			  return
		  }
		  var body WebhookConfirm
		  if json.Unmarshal(raw, &body) != nil {
			  c.JSON(400, gin.H{"error": "invalid webhook body", "acked": false})
			   return
		   }
		  sig := strings.TrimSpace(c.GetHeader("X-Travel-Signature"))
		   if sig == "" {
			   sig = strings.TrimSpace(c.GetHeader("X-Signature"))
		  }
		   got, err := svc.ConfirmWebhook(WebhookSecret(), sig, raw, body)
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "acked": false})
			  return
		   }
		  c.JSON(200, gin.H{"acked": true, "case": got})
	   })
	   v1.POST("/travel/pii/rotate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   NewKey string `json:"new_key"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   rep,
			  err := svc.RotatePII(body.NewKey)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, rep)
	  })
	   v1.GET("/travel/vasp/health", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		   }
		  c.JSON(200, svc.InspectPeerHealth(c.Query("code")))
	   })
	   v1.POST("/travel/ivms/validate", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		  }
		   var body struct {
			   IVMSMap
			   CaseID string `json:"case_id"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  if strings.TrimSpace(body.CaseID) != "" {
			   got, err := svc.InspectIVMS(body.CaseID, body.IVMSMap)
			   if err != nil {
				   c.JSON(apierr.HTTPStatus(err), gin.H{"ok": false, "error": err.Error(), "map": got})
				  return
			  }
			  c.JSON(200, gin.H{"ok": true, "map": got})
			  return
		  }
		  if err := ValidateIVMS(body.IVMSMap); err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"ok": false, "error": err.Error()})
			  return
		  }
		   c.JSON(200, gin.H{"ok": true})
	   })
	   v1.GET("/travel/retry-budget", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		   }
		  c.JSON(200, svc.InspectRetryBudget(c.Query("corridor")))
	   })
	  v1.GET("/travel/dead-letters", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			   }
		   }
		   limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
		   list, err := svc.ExportDeadLetters(c.Request.Context(), limit)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"items": list})
	  })
	   v1.POST("/travel/extract", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		  }
		   var body struct {
			  Format string `json:"format"`
			  Status string `json:"status"`
			   Actor string `json:"actor"`
			   Limit int    `json:"limit"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  job,
			  err := svc.RunExtract(c.Request.Context(), body.Format, body.Status, body.Actor, body.Limit)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, job)
	   })
	  v1.GET("/travel/extract/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		  }
		  job, err := svc.GetExtract(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, job)
	   })
	  v1.POST("/travel/cases/link", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		  }
		  var body WDLink
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  got, err := svc.LinkWithdraw(c.Request.Context(), body.CaseID, body.WithdrawID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	   v1.GET("/travel/cases/by-withdraw", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		  }
		   got, err := svc.CaseByWithdraw(c.Request.Context(), c.Query("withdraw_id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	  v1.GET("/travel/withdraw/by-case", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		   }
		   got, err := svc.WithdrawByCase(c.Request.Context(), c.Query("case_id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, got)
	   })
	  v1.POST("/travel/outbox/tick", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActAMLOpen); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		  }
		   if err := svc.TickOutbox(c.Request.Context()); err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"ok": true})
	   })
  }
// temporary guard removed after the production check
