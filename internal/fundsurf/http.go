package fundsurf

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/seclock"
	"account-assets.kucoin.net/funding/internal/tradepw"
	"account-assets.kucoin.net/funding/internal/wdverify"
	"github.com/gin-gonic/gin"
)

type HTTP struct {
	Surf *Book
	PW   *tradepw.Book
	Gate *wdverify.Gate
	Sec  *seclock.Service
	ID   callerid.Opt
	Mem  bool
	Send wdverify.Sender
	// MemoDep 审批通过后入账适配；为空时仅改工单状态。
	MemoDep MemoDepositor
}

   func NewHTTP(kv persist.KV, led *ledger.Service, cfg *config.Config, mem bool) HTTP {
	  seed, kind, profile := "", "", ""
	  if cfg != nil {
		  seed, kind, profile = cfg.SeedUserID, cfg.Auth.Kind, cfg.Profile
	  }
	  sec := seclock.New(led)
	  pw := tradepw.New(kv).BindCooldown(sec)
	   var combos [][]string
	  if cfg != nil {
		  combos = config.ParseWithdrawCombos(cfg.Funds.WithdrawCombos)
	  }
	   gate := wdverify.New(kv,
		  pw, combos).BindCooldown(sec)
	  surf := New(kv, led, cfg).BindSecurity(sec, pw, gate)
	   return HTTP{
		   Surf: surf,
		   PW:   pw,
		   Gate: gate,
		  Sec: sec,
		  ID:  callerid.FromRuntime(mem, seed, kind, profile),
		  Mem: mem,
	  }
   }

  func (h *HTTP) gate() *wdverify.Gate {
	   if h == nil {
		   return nil
	   }
	   if h.Gate != nil {
		  return h.Gate
	  }
	  if h.Surf != nil {
		  return h.Surf.EnsureGate()
	   }
	   return nil
   }

   func (h *HTTP) pw() *tradepw.Book {
	   if h == nil {
		   return nil
	  }
	   if h.PW != nil {
		  return h.PW
	  }
	  if h.Surf != nil {
		  return h.Surf.TradePW()
	   }
	  return nil
  }

  func Register(v1 *gin.RouterGroup, h HTTP) {
	   if v1 == nil {
		  return
	  }
	  if h.Surf == nil {
		   h.Surf = New(nil, nil, nil)
	  }
	   if h.PW == nil {
		  h.PW = tradepw.New(nil)
	   }
	   if h.Gate == nil {
		  h.Gate = h.Surf.EnsureGate()
	   }
	   surf, pw := h.Surf, h.pw()
	  v1.GET("/security/withdraw-lock", func(c *gin.Context) {
		   uid, err := callerid.Require(c, h.ID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   lock, err := surf.Lock(c.Request.Context(), uid)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"user_id": uid, "lock": lock, "host": config.HostSecurity})
	   })
	  // 安全读面：冷却锁 + 交易密码态 + 验证因子绑定/失败锁。
	  v1.GET("/security/status", func(c *gin.Context) {
		  uid, err := callerid.Require(c, h.ID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   st,
			   err := surf.SecurityStatus(c.Request.Context(), uid)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"user_id": uid, "security": st})
	   })
	  v1.GET("/security/factors", func(c *gin.Context) {
		   uid,
			  err := callerid.Require(c, h.ID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  g := h.gate()
		  if g == nil {
			  c.JSON(200, gin.H{"user_id": uid, "factors": wdverify.StatusView{}, "host": config.HostSecurity})
			  return
		   }
		  st, err := g.Status(c.Request.Context(), uid)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"user_id": uid, "factors": st, "host": config.HostSecurity})
	   })
	   v1.GET("/security/trading-password", func(c *gin.Context) {
		   uid,
			  err := callerid.Require(c, h.ID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  if pw == nil {
			   mw.WriteProblem(c, tradepw.ErrStore)
			   return
		   }
		  st,
			   err := pw.Status(c.Request.Context(), uid)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"user_id": uid, "trading_password": st, "host": config.HostSecurity})
	   })
	  v1.POST("/security/trading-password", func(c *gin.Context) {
		  var body struct {
			  Password string `json:"password"`
			  OldPassword string `json:"old_password"`
			  Reset bool `json:"reset"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  uid, err := callerid.Require(c, h.ID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  if pw == nil {
			   mw.WriteProblem(c, tradepw.ErrStore)
			   return
		  }
		   switch {
		   case body.Reset:
			   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				   return
			   }
			   err = pw.Reset(c.Request.Context(), uid, body.Password)
		   case pw.Setted(c.Request.Context(), uid):
			  err = pw.Change(c.Request.Context(), uid, body.OldPassword, body.Password)
		   default:
			  err = pw.Set(c.Request.Context(), uid, body.Password)
		   }
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"ok": true, "user_id": uid, "host": config.HostSecurity})
	   })
	  v1.POST("/security/trading-password/verify", func(c *gin.Context) {
		  var body struct {
			  Password string `json:"password"`
		   }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  uid, err := callerid.Require(c, h.ID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  if pw == nil {
			   mw.WriteProblem(c, tradepw.ErrStore)
			  return
		   }
		   if err := pw.Verify(c.Request.Context(), uid, body.Password); err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"ok": true, "host": config.HostSecurity})
	  })
	  v1.POST("/ops/users/withdraw-forbid", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  UserID string `json:"user_id"`
			   Forbidden bool `json:"forbidden"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   if body.UserID == "" {
			  mw.WriteProblem(c, ErrUser)
			   return
		   }
		   if err := surf.SetForbidden(c.Request.Context(), body.UserID, body.Forbidden); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"ok": true, "user_id": body.UserID, "forbidden": body.Forbidden})
	  })
	   v1.GET("/ops/withdraw-halt", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  f, err := surf.HaltGate().Snapshot(c.Request.Context())
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"halt": f})
	   })
	  v1.POST("/ops/withdraw-halt", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  var body struct {
			  On    bool   `json:"on"`
			   Actor string `json:"actor"`
			   Reason string `json:"reason"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   gate := surf.HaltGate()
		   var err error
		   if body.On {
			  err = gate.Engage(c.Request.Context(), body.Actor, body.Reason)
		  } else {
			   err = gate.Clear(c.Request.Context(),
				  body.Actor, body.Reason)
		   }
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   f, _ := gate.Snapshot(c.Request.Context())
		   c.JSON(200, gin.H{"ok": true, "halt": f})
	   })
	  v1.POST("/deposits/memo-recover", func(c *gin.Context) {
		   var body struct {
			  Chain  string `json:"chain"`
			  TxHash string `json:"tx_hash"`
			   Memo string `json:"memo"`
			  Address string `json:"address"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  uid, err := callerid.Require(c, h.ID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   t, err := surf.OpenMemoTicket(c.Request.Context(), uid, body.Chain, body.TxHash, body.Memo, body.Address)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, gin.H{"ticket": t})
	  })
	   v1.POST("/ops/memo-recover/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   Status string `json:"status"`
			  Asset  string `json:"asset"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  var (
			   t MemoTicket
			   err error
		  )
		  if strings.EqualFold(strings.TrimSpace(body.Status), "approved") && h.MemoDep != nil {
			   t, err = surf.ApproveMemoCredit(c.Request.Context(), c.Param("id"), body.Asset, h.MemoDep)
		  } else {
			  t, err = surf.DecideMemo(c.Request.Context(), c.Param("id"), body.Status)
		  }
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, gin.H{"ticket": t})
	   })
	  v1.POST("/withdrawals/whitelist-mode", func(c *gin.Context) {
		   var body struct {
			  Enabled bool `json:"enabled"`
		   }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  uid,
			  err := callerid.Require(c, h.ID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   if err := surf.SetWhitelistOnly(c.Request.Context(), uid, body.Enabled); err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"ok": true, "enabled": body.Enabled})
	   })
	   v1.POST("/security/withdraw-challenge", func(c *gin.Context) {
		  var body struct {
			  Factor string `json:"factor"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   uid, err := callerid.Require(c, h.ID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  g := h.gate()
		  if g == nil {
			   var kv persist.KV
			  if h.Surf != nil {
				  kv = h.Surf.kv
			   }
			  g = wdverify.New(kv, h.pw(), nil)
		   }
		  if h.Send != nil {
			   g.BindSender(h.Send)
		  }
		   ch, code, err := g.Issue(c.Request.Context(), uid, body.Factor)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   out := gin.H{"challenge": ch, "host": config.HostSecurity}
		   if h.Mem {
			   out["code"] = code
		  }
		   c.JSON(200, out)
	   })
	  v1.POST("/security/totp", func(c *gin.Context) {
		  var body struct {
			   Secret string `json:"secret"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  uid, err := callerid.Require(c, h.ID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  g := h.gate()
		   if g == nil {
			  var kv persist.KV
			   if h.Surf != nil {
				   kv = h.Surf.kv
			   }
			  g = wdverify.New(kv, h.pw(), nil).BindCooldown(h.Sec)
		  }
		  if err := g.BindTOTP(c.Request.Context(), uid, body.Secret); err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   secret, _ := g.TOTPSecret(c.Request.Context(), uid)
		   out := gin.H{"ok": true, "user_id": uid, "host": config.HostSecurity}
		   if h.Mem {
			  out["secret"] = secret
		   }
		   c.JSON(200, out)
	  })
	  // 出金前预检：跑统一组合闸，不创建出金单。
	  v1.POST("/security/withdraw-precheck", func(c *gin.Context) {
		   var body struct {
			  Chain string `json:"chain"`
			  ToAddress     string `json:"to_address"`
			   TradePassword string `json:"trade_password"`
			   TOTP string `json:"totp"`
			  Email string `json:"email"`
			  SMS string `json:"sms"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   uid, err := callerid.Require(c, h.ID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   err = surf.RequireWithdrawSecure(c.Request.Context(), uid, body.Chain, body.ToAddress, wdverify.Presented{
			   TradePassword: body.TradePassword, TOTP: body.TOTP, Email: body.Email, SMS: body.SMS,
		   })
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"ok": true, "user_id": uid, "host": config.HostSecurity})
	   })
	   registerDepFace(v1, surf)
	   registerAttribDesk(v1, surf)
   }

   func csvCell(s string) string {
	   if strings.ContainsAny(s, ",\"\n\r") {
		  return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
	  }
	  return s
  }
