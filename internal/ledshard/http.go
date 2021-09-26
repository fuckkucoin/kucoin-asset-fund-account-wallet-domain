package ledshard

import (
	"context"
	"net/http"
	"strconv"
	"time"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func init() {
	  apierr.Register(ErrNotFound, http.StatusNotFound)
	   apierr.Register(ErrUser, http.StatusBadRequest)
	   apierr.Register(ErrBook,
		  http.StatusBadRequest)
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	   apierr.Register(ErrAmount,
		   http.StatusBadRequest)
	  apierr.Register(ErrSameShard, http.StatusConflict)
	  apierr.Register(ErrIdemConflict, http.StatusConflict)
	   apierr.Register(ErrRouteMissing, http.StatusConflict)
	   apierr.Register(ErrRouteDraft, http.StatusConflict)
	  apierr.Register(ErrRouteWindow, http.StatusConflict)
	  apierr.Register(ErrRouteRetired, http.StatusConflict)
	  apierr.Register(ErrRoutePinned, http.StatusConflict)
	  apierr.Register(ErrRouteImmutable, http.StatusConflict)
	   apierr.Register(ErrUnpinned, http.StatusConflict)
	   apierr.Register(ErrPinLeak,
		   http.StatusConflict)
	   apierr.Register(ErrAffinity, http.StatusBadRequest)
	   apierr.Register(ErrNoRing,
		  http.StatusConflict)
	   apierr.Register(ErrSealed, http.StatusConflict)
	  apierr.Register(ErrShardMismatch, http.StatusConflict)
	  apierr.Register(ErrInsufficient, http.StatusConflict)
	  apierr.Register(ErrDoubleCredit, http.StatusConflict)
	  apierr.Register(ErrTerminal, http.StatusConflict)
	   apierr.Register(ErrHalted,
		  http.StatusConflict)
	   apierr.Register(ErrDrift, http.StatusConflict)
	  apierr.Register(ErrDesk, http.StatusConflict)
	  apierr.Register(ErrDual, http.StatusConflict)
	  apierr.Register(ErrVoucher,
		   http.StatusConflict)
	   apierr.Register(ErrStuck, http.StatusConflict)
	   apierr.Register(ErrConserve, http.StatusConflict)
	  apierr.Register(ErrTransition, http.StatusConflict)
	   apierr.Register(ErrActor, http.StatusBadRequest)
  }

// Register 挂到账本旁路进程。分片引擎独立于 ledger.Apply。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  eng := New()
	   if err := eng.Bootstrap(DefaultRing); err != nil {
		  panic(err)
	   }
	  if rt != nil && rt.SQL != nil && rt.SQL.DB != nil {
		   if err := eng.BindSQL(rt.SQL); err != nil {
			  panic(err)
		   }
	   }
	  go eng.Loop(context.Background(), 2*time.Second)
	   Mount(v1, eng)
   }

// Mount 注册路由管理、意图、凭证、看板和双人补偿。
   func Mount(v1 *gin.RouterGroup, eng *Engine) {
	  g := v1.Group("/ledshard")
	   g.GET("/health", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			   return
		   }
		   c.JSON(200, gin.H{"ok": eng.Healthy(), "dashboard": eng.Dashboard()})
	   })
	   g.GET("/dashboard", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			   return
		   }
		  c.JSON(200, eng.Dashboard())
	   })
	   g.GET("/alerts", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			  return
		   }
		   c.JSON(200, gin.H{"alerts": eng.Alerts()})
	   })
	   g.POST("/alerts/:id/ack", func(c *gin.Context) {
		   if !allow(c, perm.ActOpsAdmin) {
			  return
		   }
		  got, err := eng.AckAlert(c.Param("id"))
		   write(c, got, err)
	   })
	   g.GET("/hints", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			  return
		   }
		   c.JSON(200, gin.H{"hints": eng.Hints(), "note": "edge hints only; no causal graph store"})
	  })
	   g.GET("/shards", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			   return
		   }
		  c.JSON(200, gin.H{"shards": eng.ListShards()})
	   })
	  g.POST("/shards", func(c *gin.Context) {
		   if !allow(c, perm.ActOpsAdmin) {
			   return
		  }
		   var body Shard
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   got, err := eng.UpsertShard(body)
		  write(c, got, err)
	  })
	   g.GET("/routes", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			  return
		   }
		   c.JSON(200, gin.H{"routes": eng.ListRoutes()})
	   })
	  g.POST("/routes/draft", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			  return
		   }
		  var body struct {
			   Note string `json:"note"`
			  UserShards []string `json:"user_shards"`
			   Affinity map[string]string `json:"affinity"`
			  Reverse bool `json:"reverse"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   var got RouteVersion
		  var err error
		  if body.Reverse {
			  got, err = eng.DraftReverse(body.Note)
		  } else {
			  got,
				   err = eng.Draft(body.Note, body.UserShards, body.Affinity)
		  }
		  write(c, got, err)
	   })
	   g.POST("/routes/:ver/publish", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			  return
		  }
		   ver, err := strconv.ParseInt(c.Param("ver"), 10, 64)
		   if err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			  return
		  }
		   got, err := eng.Publish(ver)
		  write(c, got, err)
	   })
	  g.POST("/routes/:ver/activate", func(c *gin.Context) {
		   if !allow(c, perm.ActOpsAdmin) {
			  return
		  }
		   ver, err := strconv.ParseInt(c.Param("ver"), 10, 64)
		   if err != nil {
			   c.JSON(400, gin.H{"error": err.Error()})
			   return
		   }
		   got, err := eng.Activate(ver)
		   write(c, got, err)
	  })
	  g.POST("/resolve", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			  return
		  }
		   var body struct {
			   Version int64 `json:"route_version"`
			   UserID string `json:"user_id"`
			  Book   string `json:"book"`
			  Asset  string `json:"asset"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := eng.ResolveSubject(body.Version, body.UserID, body.Book, body.Asset)
		  write(c, got, err)
	   })
	  g.POST("/balances/seed", func(c *gin.Context) {
		   if !allow(c, perm.ActOpsAdmin) {
			   return
		   }
		  var body struct {
			   UserID string `json:"user_id"`
			  Book   string `json:"book"`
			  Asset  string `json:"asset"`
			  Amount string `json:"amount"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   got, err := eng.Seed(body.UserID, body.Book, body.Asset, body.Amount)
		   write(c, got, err)
	   })
	   g.GET("/balances", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			  return
		  }
		   got, err := eng.BalanceOf(c.Query("user_id"), c.Query("book"), c.Query("asset"))
		  write(c, got, err)
	  })
	   g.POST("/intents", func(c *gin.Context) {
		   if !allow(c, perm.ActLedgerWrite) {
			   return
		  }
		   var body OpenReq
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  got, err := eng.Open(body)
		   write(c, got, err)
	  })
	  g.GET("/intents", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			  return
		   }
		  c.JSON(200, gin.H{"intents": eng.ListIntents()})
	   })
	   g.GET("/intents/:id", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			  return
		  }
		   got, err := eng.Get(c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"intent": got, "steps": eng.Steps(got.ID), "postings": eng.Postings(got.ID)})
	   })
	  g.GET("/intents/:id/conserve", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			   return
		  }
		   got, err := eng.Conserve(c.Param("id"))
		  write(c, got, err)
	  })
	  g.POST("/intents/:id/advance", func(c *gin.Context) {
		   if !allow(c, perm.ActLedgerWrite) {
			   return
		  }
		  got, err := eng.Advance(c.Param("id"))
		   write(c, got, err)
	  })
	   g.POST("/intents/:id/run", func(c *gin.Context) {
		   if !allow(c, perm.ActLedgerWrite) {
			  return
		  }
		   got, err := eng.Run(c.Param("id"))
		   write(c, got, err)
	   })
	  g.POST("/intents/:id/kill", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			   return
		   }
		   got, err := eng.Kill(c.Param("id"))
		  write(c, got, err)
	  })
	  g.POST("/intents/:id/rebind", func(c *gin.Context) {
		   if !allow(c, perm.ActOpsAdmin) {
			  return
		   }
		   var body struct {
			   Version int64 `json:"route_version"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := eng.Rebind(c.Param("id"), body.Version)
		  write(c, got, err)
	  })
	   g.POST("/intents/:id/compensate", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			  return
		  }
		  var body struct {
			   Reason string `json:"reason"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := eng.Compensate(c.Param("id"), body.Reason)
		   write(c, got, err)
	  })
	  g.GET("/vouchers/:id", func(c *gin.Context) {
		  if !allow(c, perm.ActQueryRead) {
			   return
		   }
		   got, err := eng.Voucher(c.Param("id"))
		   write(c, got, err)
	   })
	   g.POST("/worker/tick", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			   return
		   }
		   got, err := eng.Tick()
		   write(c, got, err)
	  })
	   g.POST("/desk/open", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			  return
		  }
		   var body struct {
			   IntentID string `json:"intent_id"`
			   Actor    string `json:"actor"`
			   Reason   string `json:"reason"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		   if body.Actor == "" {
			  body.Actor = c.GetHeader("X-Actor")
		   }
		  got, err := eng.OpenDesk(body.IntentID, body.Actor, body.Reason)
		   write(c, got, err)
	  })
	  g.POST("/desk/:id/approve", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			  return
		   }
		  var body struct {
			   Actor string `json:"actor"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  if body.Actor == "" {
			   body.Actor = c.GetHeader("X-Actor")
		  }
		   got, err := eng.ApproveDesk(c.Param("id"), body.Actor)
		  write(c, got, err)
	  })
	  g.POST("/desk/:id/compensate", func(c *gin.Context) {
		  if !allow(c, perm.ActOpsAdmin) {
			   return
		   }
		  var body struct {
			  Actor string `json:"actor"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   if body.Actor == "" {
			  body.Actor = c.GetHeader("X-Actor")
		   }
		  got, err := eng.ExecuteDesk(c.Param("id"), body.Actor)
		  write(c, got, err)
	   })
	  g.GET("/desk", func(c *gin.Context) {
		   if !allow(c, perm.ActQueryRead) {
			  return
		   }
		  c.JSON(200, gin.H{"desk": eng.ListDesk()})
	   })
   }

   func allow(c *gin.Context, act perm.Action) bool {
	  if err := perm.FromRequest(c.Request, "").Check(act); err != nil {
		  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		   return false
	   }
	  return true
   }

   func write(c *gin.Context, body any, err error) {
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	  }
	  c.JSON(200, body)
  }
