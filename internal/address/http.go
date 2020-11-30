package address

import (
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/pathsafe"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
	"time"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New(rt.Ledger, rt.Wallet)
	   if rt != nil && !rt.Mem && rt.SQL != nil {
		   svc.BindSQL(rt.SQL)
	  }
	  // 挂 FamMEMO 共享池与入金归因（deposit.Attach 可能已设）。
	   if rt != nil {
		  var shared *addrpool.SharedMemoPool
		   if rt.SharedMemo != nil {
			  shared, _ = rt.SharedMemo.(*addrpool.SharedMemoPool)
		   }
		   if shared == nil {
			  shared = svc.SharedPool()
			  rt.SharedMemo = shared
			   if !rt.Mem && rt.SQL != nil {
				  shared.BindSQL(rt.SQL)
			   }
		  } else {
			   svc.BindSharedMemo(shared, nil)
		   }
		  if rt.DepositAttr != nil {
			  if a, ok := rt.DepositAttr.(*deposit.Attributor); ok {
				   svc.BindSharedMemo(shared, deposit.AsMemoBinder(a))
			  } else if binder, ok := rt.DepositAttr.(addrpool.MemoBinder); ok {
				   svc.BindSharedMemo(shared, binder)
			   }
		  }
		   // 拒绝 FamMEMO 种子 Ensure；共享地址仅运营 Register。
		   if rt.Cfg != nil && authn.ProductionProfile(rt.Cfg.Auth.Kind, rt.Cfg.Profile) {
			   shared.RefuseSeedEnsure()
			   shared.RefuseNilBinder()
		   }
		   if rt.KV != nil {
			   svc.BindKV(rt.KV)
			   svc.BindRecycle(addrpool.NewRecycleBook(rt.KV, 24*time.Hour))
			   addrpool.BindAssignMeter(addrpool.NewRateBook(rt.KV))
		   }
		   if rt.DestAllow != nil && rt.KV != nil {
			  destallow.BindRuntime(rt.DestAllow,
				  rt.KV)
			   destallow.StartExpire(rt.DestAllow,
				  rt.Log)
		  }
	  }
	   v1.GET("/addresses/deposit", func(c *gin.Context) {
		   user, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   chain := strings.ToUpper(c.DefaultQuery("chain", "ETH"))
		   addr, err := svc.DepositAddress(c.Request.Context(), user, chain)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  bind, _ := svc.Binding(user, chain)
		  memo := ""
		  if wallet.RequiresFamMemo(chain) {
			   memo = fundsurfMemo(user, chain)
			  if memo != "" {
				  _, _ = svc.BindFamMemo(user, chain, memo)
			  }
		   }
		  tagName := fundsurf.MemoTagName(chain)
		   view := gin.H{
			   "user_id": user, "chain": chain, "address": addr, "memo": memo,
			  "family": bind.Family, "shared": wallet.RequiresFamMemo(chain),
			   "needMemo": wallet.RequiresFamMemo(chain), "tagName": tagName,
			  "format": wallet.FormatSpec(chain),
		  }
		  if tagName == "destination_tag" && memo != "" {
			  view["destinationTag"] = memo
		  }
		  if tagName == "comment" && memo != "" {
			   view["comment"] = memo
		  }
		  c.JSON(200, view)
	   })
	  v1.GET("/addresses/bindings", func(c *gin.Context) {
		   user, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"user_id": user, "bindings": svc.Bindings(user)})
	   })
	  v1.GET("/addresses/allowlisted", func(c *gin.Context) {
		   user, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   var pref destallow.PrefReader
		  var book *destallow.Book
		  if rt != nil && rt.FundSurf != nil {
			  pref = rt.FundSurf
		   }
		   if rt != nil {
			   book = rt.DestAllow
		   }
		   view, err := svc.InspectAllow(c.Request.Context(), user, c.DefaultQuery("chain", "ETH"), c.Query("address"), book, pref)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(200, view)
	   })
	  v1.GET("/addresses/book", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   pageRaw := strings.TrimSpace(c.Query("currentPage"))
		   sizeRaw := strings.TrimSpace(c.Query("pageSize"))
		  chain := strings.TrimSpace(c.Query("chain"))
		  addr := strings.TrimSpace(c.Query("address"))
		   cur := strings.TrimSpace(c.Query("currency"))
		   if cur == "" {
			   cur = strings.TrimSpace(c.Query("asset"))
		   }
		   includeDisabled := strings.TrimSpace(c.Query("include_disabled")) == "1" || strings.EqualFold(c.Query("include_disabled"), "true")
		  tag := strings.TrimSpace(c.Query("tag"))
		   if pageRaw == "" && sizeRaw == "" && chain == "" && addr == "" && cur == "" && tag == "" && !includeDisabled {
			  got, err := svc.ListBookPage(c.Request.Context(), BookQuery{UserID: user, Page: 1, PageSize: 200, IncludeDisabled: false})
			  if err != nil {
				   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
				   return
			   }
			  c.JSON(200, gin.H{"entries": got.Entries})
			  return
		   }
		  page, _ := strconv.Atoi(pageRaw)
		  size, _ := strconv.Atoi(sizeRaw)
		   got, err := svc.ListBookPage(c.Request.Context(), BookQuery{
			  UserID: user, Chain: chain, Address: addr, Currency: cur, Asset: cur, Tag: tag,
			  Page: page, PageSize: size, IncludeDisabled: includeDisabled,
		  })
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, gin.H{
			  "entries": got.Entries, "currentPage": got.Page, "pageSize": got.PageSize,
			  "totalNum": got.Total, "totalPage": got.TotalPage, "currency": got.Currency,
		  })
	  })
	   v1.POST("/addresses/book", func(c *gin.Context) {
		   var body BookWrite
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
		  body.Actor = uid
		   if body.IdemKey == "" {
			  body.IdemKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		  }
		  e, err := svc.AddBookWrite(c.Request.Context(), body)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  syncDestAllow(rt, e, false)
		   c.JSON(200, svc.decorate(c.Request.Context(), e))
	  })
	  v1.DELETE("/addresses/book", func(c *gin.Context) {
		  user, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   chain := c.Query("chain")
		  addr := c.Query("address")
		   // 默认软删；purge=1 且运营闸才物理删除。
		   if strings.TrimSpace(c.Query("purge")) == "1" {
			   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
			  if err := svc.RemoveBook(c.Request.Context(), user, chain, addr); err != nil {
				  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
				   return
			   }
		   } else {
			   e, err := svc.DisableBook(c.Request.Context(), user, chain, addr, user)
			   if err != nil {
				   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
				  return
			   }
			   syncDestAllow(rt, e, true)
			   c.JSON(200, gin.H{"ok": true, "soft_deleted": true, "entry": svc.decorate(c.Request.Context(), e)})
			   return
		   }
		  if rt != nil && rt.DestAllow != nil {
			   _ = rt.DestAllow.RemoveExact(user,
				   chain, addr)
		  }
		   c.JSON(200, gin.H{"ok": true, "purged": true})
	  })
	  registerExtra(v1, rt, svc)
  }

   func registerExtra(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  // 运营登记 FamMEMO 共享充值地址。
	  v1.POST("/addresses/shared-addrs", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  Chain   string `json:"chain"`
			  Address string `json:"address"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  pool := svc.SharedPool()
		  if pool == nil {
			  c.JSON(503, gin.H{"error": "shared memo pool unavailable"})
			  return
		   }
		  if err := pool.Register(body.Chain, body.Address); err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, gin.H{
			   "chain":   strings.ToUpper(strings.TrimSpace(body.Chain)),
			   "address": strings.TrimSpace(body.Address),
			   "shared":             true,
			  "refuse_seed_ensure": pool.SeedEnsureRefused(),
			   "path": "ops-register",
		  })
	  })
	  v1.GET("/addresses/shared-addrs", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		   }
		   pool := svc.SharedPool()
		   if pool == nil {
			   c.JSON(200, gin.H{"items": []any{}})
			   return
		  }
		   c.JSON(200, gin.H{
			  "items": pool.List(),
			   "refuse_seed_ensure": pool.SeedEnsureRefused(),
			   "registered":         pool.RegisteredCount(),
			   "missing": pool.MissingFamChains(),
		   })
	   })
	  v1.POST("/addresses/validate", func(c *gin.Context) {
		   var body struct {
			   Chain string `json:"chain"`
			  Address string `json:"address"`
			  Memo    string `json:"memo"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  rule, err := CheckFormatMatrix(body.Chain, body.Address, body.Memo)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  ch := rule.Chain
		  addr := strings.TrimSpace(body.Address)
		   // FamMEMO 可选加深 memo 校验；形态 UX 一并回传。
		  out := gin.H{
			  "ok": true, "chain": ch, "address": addr, "evm": wallet.IsEVM(ch),
			  "needMemo": wallet.RequiresFamMemo(ch), "tagName": wallet.FamMemoTagName(ch),
			   "format": rule.AddrFormatSpec, "rule": rule.Rule, "matrix": true,
		   }
		  if strings.TrimSpace(body.Memo) != "" {
			   if err := wallet.ValidFamMemoTag(ch, body.Memo); err != nil {
				   out["ok"] = false
				   out["memo_error"] = err.Error()
				  c.JSON(400, out)
				   return
			   }
			  out["memo"] = strings.TrimSpace(body.Memo)
		  }
		   c.JSON(200, out)
	  })
	   v1.GET("/addresses/book/audit", func(c *gin.Context) {
		   user, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
		  list, err := svc.ListBookAudit(c.Request.Context(), user, c.Query("chain"), c.Query("address"), limit)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, gin.H{"audits": list, "user_id": user})
	   })
	  v1.GET("/addresses/family", func(c *gin.Context) {
		  chain, err := validate.Chain(c.DefaultQuery("chain", "ETH"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  fam := chain
		   if wallet.IsEVM(chain) {
			   fam = "EVM"
		   }
		  c.JSON(200, gin.H{"chain": chain, "family": fam})
	   })
	  v1.GET("/addresses/owner", func(c *gin.Context) {
		   addr := strings.TrimSpace(c.Query("address"))
		  if addr == "" {
			  c.JSON(400, gin.H{"error": "address required"})
			   return
		  }
		  bind, ok := svc.OwnerOf(addr)
		  c.JSON(200, gin.H{"found": ok, "binding": bind})
	   })
	   registerSafety(v1, rt, svc)
	  registerRead(v1, rt, svc)
  }

  func registerSafety(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  _ = svc
	  _ = rt
	   v1.POST("/addresses/path-check", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAddressWrite); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  Path string `json:"path"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := pathsafe.Check(body.Path)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"ok": true, "path": got, "host": "address.kucoin.net"})
	   })
	   v1.POST("/addresses/bind-safe", func(c *gin.Context) {
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
		  if _, err := validate.UserID(body.UserID); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   addr, err := validate.Address(body.Chain, body.Address)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"ok": true, "address": addr})
	   })
   }

   func fundsurfMemo(userID, chain string) string {
	  return fundsurf.MemoFor(userID, chain)
  }
