package addrpool

import (
	"context"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   pool := New(nil, 24*time.Hour)
	   inv := NewInventory(nil, 24*time.Hour)
	   if rt != nil && !rt.Mem && rt.SQL != nil {
		  pool.BindSQL(rt.SQL)
		  inv.BindSQL(rt.SQL)
	   }
	  obs := NewObserver()
	   rec := NewRecycleBook(nil, 24*time.Hour)
	   rates := NewRateBook(nil)
	  if rt != nil && rt.KV != nil {
		  rec = NewRecycleBook(rt.KV, 24*time.Hour)
		   rates = NewRateBook(rt.KV)
		  BindAssignMeter(rates)
	   }
	   v1.POST("/addrpool/allocate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAddressWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   UserID string `json:"user_id"`
			  Chain  string `json:"chain"`
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
		   got, err := pool.Allocate(body.UserID, body.Chain)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	  })
	  shared := SharedFromRuntime(nil, nil, true)
	  id := callerid.Opt{}
	  if rt != nil {
		  sql := rt.SQL
		   if rt.Mem {
			   sql = nil
		  }
		  shared = SharedFromRuntime(rt.SharedMemo, sql, rt.Mem)
		  if rt.Cfg != nil {
			   id = callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)
		  }
	  }
	  v1.GET("/addrpool/list", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  user := c.Query("user_id")
		   if user == "" && rt != nil {
			  if u, err := callerid.Bind(c, user, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile)); err != nil {
				   mw.WriteProblem(c, err)
				  return
			   } else {
				  user = u
			   }
		  }
		   c.JSON(200, gin.H{"bindings": pool.ListChain(user, c.Query("chain")), "host": "address.kucoin.net"})
	   })
	   v1.GET("/addrpool/assigned", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), id)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   asset := strings.TrimSpace(c.Query("asset"))
		   if asset == "" {
			  asset = strings.TrimSpace(c.Query("currency"))
		  }
		  items := FillAssignedMemo(user, InspectAssignedList(pool, shared, user, c.Query("chain"), asset, ""), fundsurf.MemoFor)
		  pageRaw := strings.TrimSpace(c.Query("currentPage"))
		  sizeRaw := strings.TrimSpace(c.Query("pageSize"))
		   if pageRaw != "" || sizeRaw != "" {
			  page, _ := strconv.Atoi(pageRaw)
			   size, _ := strconv.Atoi(sizeRaw)
			  got, total, pageOut, sizeOut, pages := PageAssigned(items, page, size)
			  c.JSON(200, gin.H{"items": got, "currentPage": pageOut, "pageSize": sizeOut, "totalNum": total, "totalPage": pages, "host": "address.kucoin.net"})
			  return
		   }
		  c.JSON(200, gin.H{"items": items, "host": "address.kucoin.net"})
	  })
	   v1.GET("/addrpool/by-address", func(c *gin.Context) {
		   user, err := callerid.Bind(c, c.Query("user_id"), id)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  view := InspectByAddress(pool, c.Query("address"))
		  if view.Found && view.UserID != user {
			  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		   }
		   c.JSON(200, view)
	   })
	  v1.GET("/addrpool/inspect", func(c *gin.Context) {
		   user, err := callerid.Bind(c, c.Query("user_id"), id)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   asset := strings.TrimSpace(c.Query("asset"))
		   if asset == "" {
			  asset = strings.TrimSpace(c.Query("currency"))
		   }
		  view := InspectAssigned(pool, shared, user, c.Query("chain"), asset, "")
		  c.JSON(200, FillAssignedMemo(user, []AssignedView{view}, fundsurf.MemoFor)[0])
	  })
	  v1.POST("/addrpool/prefill", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   Chain string `json:"chain"`
			  N    int    `json:"n"`
			   Seed string `json:"seed"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  n,
			  err := inv.Prefill(body.Chain, body.N, body.Seed)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"added": n, "counts": inv.Counts(body.Chain)})
	  })
	  v1.POST("/addrpool/observe", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActDepositObserve); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   Chain   string `json:"chain"`
			  Address string `json:"address"`
			   Asset   string `json:"asset"`
			  Amount string `json:"amount"`
			   TxHash string `json:"tx_hash"`
			   Confirms int `json:"confirmations"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := obs.Observe(body.Chain, body.Address, body.Asset, body.Amount, body.TxHash, body.Confirms)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	   })
	   v1.POST("/addrpool/assign", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAddressWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			   UserID string `json:"user_id"`
			   Asset string `json:"asset"`
			  Chain string `json:"chain"`
			   Memo  string `json:"memo"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   uid,
			  err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  memo := strings.TrimSpace(body.Memo)
		   if memo == "" {
			  memo = fundsurf.MemoFor(uid, body.Chain)
		  }
		  var binder MemoBinder
		   if rt != nil && rt.DepositAttr != nil {
			  if b, ok := rt.DepositAttr.(MemoBinder); ok {
				  binder = b
			  }
		  }
		  got, err := Assign(pool, shared, binder, AssignReq{UserID: uid, Asset: body.Asset, Chain: body.Chain, Memo: memo})
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	   })
	  v1.POST("/addrpool/release", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAddressWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   UserID string `json:"user_id"`
			  Chain string `json:"chain"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   if err := ReleaseUser(pool, uid, body.Chain); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"ok": true, "user_id": uid, "chain": strings.ToUpper(strings.TrimSpace(body.Chain))})
	   })
	   v1.POST("/addrpool/recycle", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAddressWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   UserID string `json:"user_id"`
			   Chain  string `json:"chain"`
			   Address string `json:"address"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   got, err := RecycleUser(c.Request.Context(), pool, rec, uid, body.Chain, body.Address)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	   v1.GET("/addrpool/shortage", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		   }
		   c.JSON(200, InspectShortage(inv, shared, c.Query("chain")))
	  })
	  v1.POST("/addrpool/reclaim", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAddressWrite); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   UserID  string `json:"user_id"`
			  Chain   string `json:"chain"`
			   Address string `json:"address"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  got, err := ReclaimUser(c.Request.Context(), pool, rec, uid, body.Chain, body.Address)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   if strings.TrimSpace(body.Address) != "" {
			   _, _ = inv.ReclaimSlot(body.Address)
		   }
		   c.JSON(200, got)
	  })
	   v1.POST("/addrpool/assign-fair", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAddressWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			   UserID string `json:"user_id"`
			   Asset string `json:"asset"`
			   Chain string `json:"chain"`
			  Memo  string `json:"memo"`
			   Chains []string `json:"chains"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   memo := strings.TrimSpace(body.Memo)
		   if memo == "" {
			   memo = fundsurf.MemoFor(uid, body.Chain)
		   }
		  var binder MemoBinder
		  if rt != nil && rt.DepositAttr != nil {
			   if b, ok := rt.DepositAttr.(MemoBinder); ok {
				   binder = b
			   }
		  }
		  got, err := AssignFair(pool, shared, binder, AssignReq{UserID: uid, Asset: body.Asset, Chain: body.Chain, Memo: memo}, body.Chains)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	   })
	   v1.GET("/addrpool/fam-attr", func(c *gin.Context) {
		   user, err := callerid.Bind(c, c.Query("user_id"), id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, InspectFamAttr(shared, user, c.Query("chain"), c.Query("memo")))
	   })
	   v1.POST("/addrpool/fam-attr", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAddressWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body struct {
			   UserID string `json:"user_id"`
			  Chain string `json:"chain"`
			  Memo string `json:"memo"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  uid,
			   err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   memo := strings.TrimSpace(body.Memo)
		  if memo == "" {
			   memo = fundsurf.MemoFor(uid, body.Chain)
		   }
		  var binder MemoBinder
		  if rt != nil && rt.DepositAttr != nil {
			   if b, ok := rt.DepositAttr.(MemoBinder); ok {
				  binder = b
			   }
		   }
		  if shared == nil {
			  c.JSON(503, gin.H{"error": "shared memo pool unavailable"})
			   return
		   }
		   got, err := shared.BindUser(uid, body.Chain, memo, binder)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	  })
	   registerDeep(v1, rt, pool, inv)
   }

   func registerDeep(v1 *gin.RouterGroup, rt *app.Runtime, pool *Pool, inv *Inventory) {
	  if v1 == nil {
		   return
	  }
	   if rt != nil {
		  StartExpire(pool, inv, rt.Log)
	   }
	  v1.GET("/addrpool/occupancy", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		   }
		   var shared *SharedMemoPool
		  if rt != nil {
			   shared = SharedFromRuntime(rt.SharedMemo,
				  nil, rt.Mem)
		  }
		  c.JSON(200, InspectOccupancy(inv, pool, shared, c.Query("chain")))
	   })
	  v1.GET("/addrpool/topup", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			   }
		   }
		   want, _ := strconv.Atoi(strings.TrimSpace(c.Query("want")))
		  var shared *SharedMemoPool
		  if rt != nil {
			   shared = SharedFromRuntime(rt.SharedMemo, nil, rt.Mem)
		   }
		   c.JSON(200, InspectTopup(inv, shared, c.Query("chain"), want))
	   })
	   v1.GET("/addrpool/tiers", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		  }
		  hot, cold := inv.CountsByTier(c.Query("chain"))
		  c.JSON(200, gin.H{"hot": hot, "cold": cold})
	  })
	   v1.POST("/addrpool/tier", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   Address string `json:"address"`
			   Tier PoolTier `json:"tier"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   got,
			   err := inv.SetTier(body.Address, body.Tier)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	   v1.POST("/addrpool/heartbeat", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAddressWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			  UserID string `json:"user_id"`
			   Chain  string `json:"chain"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   got, err := pool.Heartbeat(uid, body.Chain)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	   })
	  v1.GET("/addrpool/watermark", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		   }
		   water := waterFrom(rt)
		  var shared *SharedMemoPool
		   if rt != nil {
			   shared = SharedFromRuntime(rt.SharedMemo,
				   nil, rt.Mem)
		  }
		   view, ticket := InspectWatermark(inv, shared, water, c.Query("asset"), c.Query("chain"))
		   out := gin.H{"view": view, "watermarks": water.List()}
		  if ticket != nil {
			  out["ticket"] = ticket
		   }
		   c.JSON(200, out)
	   })
	  v1.PUT("/addrpool/watermark", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body Watermark
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   water := waterFrom(rt)
		   if err := water.Put(c.Request.Context(), body); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, body)
	  })
	  v1.GET("/addrpool/tickets", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		   }
		   c.JSON(200, gin.H{"tickets": waterFrom(rt).ListTickets(c.Request.Context(), 50)})
	   })
	  v1.POST("/addrpool/assign-sticky", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAddressWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  var body struct {
			  UserID string `json:"user_id"`
			   Asset  string `json:"asset"`
			  Chain string `json:"chain"`
			   Memo  string `json:"memo"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  memo := strings.TrimSpace(body.Memo)
		  if memo == "" {
			  memo = fundsurf.MemoFor(uid, body.Chain)
		   }
		  var binder MemoBinder
		  if rt != nil && rt.DepositAttr != nil {
			   if b, ok := rt.DepositAttr.(MemoBinder); ok {
				  binder = b
			  }
		  }
		   var shared *SharedMemoPool
		  if rt != nil {
			  shared = SharedFromRuntime(rt.SharedMemo, nil, rt.Mem)
		  }
		  got, err := AssignSticky(pool, shared, binder, marksFrom(rt), AssignReq{UserID: uid, Asset: body.Asset, Chain: body.Chain, Memo: memo})
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	  })
	   v1.POST("/addrpool/blackhole", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			   UserID string `json:"user_id"`
			   Chain   string `json:"chain"`
			   Address string `json:"address"`
			  Kind    string `json:"kind"`
			   Reason string `json:"reason"`
			  Rotate bool   `json:"rotate"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   marks := marksFrom(rt)
		   if !body.Rotate {
			   got, err := marks.Mark(c.Request.Context(), body.Chain, body.Address, firstNonEmpty(body.Kind, "blackhole"), body.Reason, "ops")
			  if err != nil {
				   mw.WriteProblem(c, err)
				  return
			   }
			   c.JSON(200, got)
			  return
		  }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  var binder MemoBinder
		   if rt != nil && rt.DepositAttr != nil {
			   if b, ok := rt.DepositAttr.(MemoBinder); ok {
				  binder = b
			   }
		   }
		  var shared *SharedMemoPool
		  if rt != nil {
			  shared = SharedFromRuntime(rt.SharedMemo,
				  nil, rt.Mem)
		   }
		   rec := NewRecycleBook(nil, 24*time.Hour)
		  if rt != nil && rt.KV != nil {
			   rec = NewRecycleBook(rt.KV,
				  24*time.Hour)
		   }
		   view, mark, err := RotateCompromised(c.Request.Context(), pool, shared, binder, rec, marks, uid, body.Chain, body.Address, firstNonEmpty(body.Kind, "blackhole"), body.Reason, "ops")
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"mark": mark, "assigned": view})
	  })
	  v1.POST("/addrpool/rotate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body struct {
			  UserID string `json:"user_id"`
			   Chain  string `json:"chain"`
			  Address string `json:"address"`
			   Reason  string `json:"reason"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   var binder MemoBinder
		  if rt != nil && rt.DepositAttr != nil {
			   if b, ok := rt.DepositAttr.(MemoBinder); ok {
				   binder = b
			   }
		  }
		   var shared *SharedMemoPool
		  if rt != nil {
			   shared = SharedFromRuntime(rt.SharedMemo, nil, rt.Mem)
		  }
		   rec := NewRecycleBook(nil, 24*time.Hour)
		   if rt != nil && rt.KV != nil {
			   rec = NewRecycleBook(rt.KV, 24*time.Hour)
		   }
		  view, mark, err := RotateCompromised(c.Request.Context(), pool, shared, binder, rec, marksFrom(rt), uid, body.Chain, body.Address, "compromise", body.Reason, "ops")
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"mark": mark, "assigned": view})
	   })
	   v1.GET("/addrpool/predict", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				   return
			   }
		   }
		   rates := NewRateBook(nil)
		  if rt != nil && rt.KV != nil {
			  rates = NewRateBook(rt.KV)
		   }
		  got, err := InspectPredict(inv, rates, c.Query("chain"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	  v1.GET("/addrpool/quarantine", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		   }
		  page, _ := strconv.Atoi(strings.TrimSpace(c.Query("currentPage")))
		   size, _ := strconv.Atoi(strings.TrimSpace(c.Query("pageSize")))
		   rec := NewRecycleBook(nil, 24*time.Hour)
		   if rt != nil && rt.KV != nil {
			   rec = NewRecycleBook(rt.KV, 24*time.Hour)
		  }
		  c.JSON(200, InspectQuarantine(marksFrom(rt), rec, c.Query("chain"), page, size))
	  })
	  v1.POST("/addrpool/fork-signal", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body ForkSignal
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   var binder MemoBinder
		   if rt != nil && rt.DepositAttr != nil {
			  if b, ok := rt.DepositAttr.(MemoBinder); ok {
				   binder = b
			  }
		  }
		   var shared *SharedMemoPool
		   var kv persist.KV
		   if rt != nil {
			   shared = SharedFromRuntime(rt.SharedMemo, nil, rt.Mem)
			  kv = rt.KV
		  }
		  rec := NewRecycleBook(nil,
			  24*time.Hour)
		   if kv != nil {
			  rec = NewRecycleBook(kv, 24*time.Hour)
		  }
		  if strings.TrimSpace(body.Actor) == "" {
			   body.Actor = "ops"
		   }
		   got, err := ApplyForkSignal(c.Request.Context(), pool, shared, binder, rec, marksFrom(rt), kv, body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	   })
	  v1.GET("/addrpool/fork-signal", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		  }
		  if rt == nil || rt.KV == nil {
			   c.JSON(404, gin.H{"error": "addrpool fork store unavailable"})
			  return
		  }
		  got, err := LastForkSignal(c.Request.Context(), rt.KV, c.Query("chain"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	  })
	  v1.POST("/addrpool/expire-tick", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  n := 0
		   if pool != nil {
			   n += pool.ExpireReclaimTick()
		   }
		  if inv != nil {
			   n += inv.ExpireReservedTick()
		   }
		  c.JSON(200, gin.H{"reclaimed": n})
	   })
   }

   func waterFrom(rt *app.Runtime) *WaterBook {
	  if rt != nil && rt.KV != nil {
		  w := NewWaterBook(rt.KV)
		   _ = w.Reload(context.Background())
		  return w
	   }
	  return NewWaterBook(nil)
  }

   func marksFrom(rt *app.Runtime) *MarkBook {
	   if rt != nil && rt.KV != nil {
		  m := NewMarkBook(rt.KV)
		  _ = m.Reload(context.Background())
		  return m
	  }
	   return NewMarkBook(nil)
  }

  func firstNonEmpty(a, b string) string {
	  if strings.TrimSpace(a) != "" {
		   return strings.TrimSpace(a)
	   }
	   return strings.TrimSpace(b)
   }
