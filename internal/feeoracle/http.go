// 功能：多链手续费抽象层——HTTP 接口层。
// 路由：
// GET /feeoracle/health oracle 健康探活
// POST /feeoracle/quote 出金预检报价（创建 QuoteVersion）
// GET /feeoracle/versions/:id 代次快照回放（replay）
// POST /feeoracle/versions/:id/bind 绑定代次到出金单
// GET /feeoracle/withdraw/:wid/version 按出金单读取已绑定代次
// POST /feeoracle/ticks 推送单源 fee tick（来自采集 worker）
// GET /feeoracle/ticks 列出所有已知 tick（ops）
// POST /feeoracle/sources 注册 oracle 源
// GET /feeoracle/sources 列出所有源
// 所有写路径要求 X-Actor；读路径无额外鉴权（由 svcboot 统一处理 Bearer token）。

// 作者：KianFord
// 日期：2021.04.29

package feeoracle

import (
	"net/http"
	"strings"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/oracle"
)

// Register 挂载所有 feeoracle 路由。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   svc := buildService(rt)
	  addRoutes(v1, svc)
   }

// addRoutes 供时直接注入 Service（不依赖 app.Runtime）。
  func addRoutes(v1 *gin.RouterGroup, svc *Service) {
	  v1.GET("/feeoracle/health", handleHealth(svc))
	   v1.POST("/feeoracle/quote", handleQuote(svc))
	  v1.GET("/feeoracle/versions/:id", handleGetVersion(svc))
	   v1.POST("/feeoracle/versions/:id/bind", handleBindVersion(svc))
	  v1.GET("/feeoracle/withdraw/:wid/version", handleGetVersionByWithdraw(svc))
	   v1.POST("/feeoracle/ticks", handlePushTick(svc))
	   v1.GET("/feeoracle/ticks", handleListTicks(svc))
	   v1.POST("/feeoracle/sources", handleRegisterSource(svc))
	   v1.GET("/feeoracle/sources", handleListSources(svc))
   }

// buildService 从 Runtime 装配 Service。

  func buildService(rt *app.Runtime) *Service {
	   if rt == nil {
		  return NewMem()
	   }
	   o := NewOracle(500)
	  var store *QuoteVersionStore
	   if rt.SQL != nil {
		   store = NewVersionStore(rt.SQL)
	   } else {
		   store = NewVersionStore(nil)
	  }
	  var feed *oracle.Feed
	   if rt.Market != nil {
		  feed = oracleFeedFromMarket(rt)
	   }
	   svc := New(o, store, nil, feed)
	  if rt.SQL != nil {
		  svc.BindSQL(rt.SQL)
	  }
	   return svc
  }

// oracleFeedFromMarket 从 market 服务中提取价格 Feed（为适配层，避免 import cycle）。

   func oracleFeedFromMarket(rt *app.Runtime) *oracle.Feed {
	   if rt == nil {
		  return nil
	  }
	   feed := oracle.NewFeed(30 * time.Second)
	   return feed
  }

// --- handlers ---

   func handleHealth(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   h := svc.Health()
		   code := http.StatusOK
		  if !h.Healthy {
			  code = http.StatusServiceUnavailable
		   }
		   c.JSON(code, h)
	   }
   }

// quoteRequest 出金预检报价请求体。

type quoteRequest struct {
	Chain  string `json:"chain"`
	Family string `json:"family,omitempty"`
	Asset string `json:"asset"`
	Gross      string `json:"gross"`
	ScheduleID string `json:"schedule_id,omitempty"`
	// TTLSeconds 报价有效期秒数；0 使用服务默认值。
	TTLSeconds int `json:"ttl_seconds,omitempty"`
}

   func handleQuote(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   var body quoteRequest
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  ttl := time.Duration(0)
		  if body.TTLSeconds > 0 {
			   ttl = time.Duration(body.TTLSeconds) * time.Second
		  }
		  v, err := svc.QuoteWithdrawFee(c.Request.Context(), QuoteWithdrawFeeReq{
			   Chain: body.Chain,
			  Family:     body.Family,
			   Asset:      body.Asset,
			   Gross:      body.Gross,
			   QuoteTTL:   ttl,
			   ScheduleID: body.ScheduleID,
		  })
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, v)
	  }
  }

  func handleGetVersion(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  id := strings.TrimSpace(c.Param("id"))
		   if id == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "version id required"})
			   return
		   }
		  v, err := svc.GetVersion(c.Request.Context(), id)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, v)
	  }
  }

// bindRequest 绑定请求体。
type bindRequest struct {
	WithdrawID string `json:"withdraw_id"`
}

   func handleBindVersion(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   id := strings.TrimSpace(c.Param("id"))
		   if id == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "version id required"})
			   return
		  }
		   var body bindRequest
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  if strings.TrimSpace(body.WithdrawID) == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "withdraw_id required"})
			  return
		  }
		  v, err := svc.BindPrecheck(c.Request.Context(), id, body.WithdrawID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(http.StatusOK, v)
	   }
   }

  func handleGetVersionByWithdraw(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  wid := strings.TrimSpace(c.Param("wid"))
		  if wid == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "withdraw_id required"})
			  return
		  }
		  v, err := svc.GetVersionByWithdraw(c.Request.Context(), wid)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, v)
	   }
  }

// tickPushRequest 采集 worker 推送 tick 的请求体。
type tickPushRequest struct {
	SourceID string       `json:"source_id"`
	Tick     ChainFeeTick `json:"tick"`
}

   func handlePushTick(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   var body tickPushRequest
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   if strings.TrimSpace(body.SourceID) == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "source_id required"})
			   return
		  }
		  if err := svc.PushTick(body.SourceID, body.Tick); err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"ok": true})
	  }
  }

   func handleListTicks(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  ticks := svc.Oracle().AllTicks()
		   c.JSON(http.StatusOK, gin.H{"ticks": ticks, "count": len(ticks)})
	   }
   }

// sourceRegisterRequest 注册源请求体。
type sourceRegisterRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	Chain    string `json:"chain"`
	Family   string `json:"family"`
	Endpoint string `json:"endpoint,omitempty"`
	Weight   int    `json:"weight,omitempty"`
	Enabled  bool   `json:"enabled"`
	StaleSec int `json:"stale_sec,omitempty"`
}

   func handleRegisterSource(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		  var body sourceRegisterRequest
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  family, err := ParseFamily(body.Family)
		   if err != nil {
			  c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			   return
		  }
		   staleDur := time.Duration(body.StaleSec) * time.Second
		   if staleDur <= 0 {
			  staleDur = 120 * time.Second
		  }
		  weight := body.Weight
		   if weight <= 0 {
			  weight = 1
		  }
		  cfg := SourceConfig{
			  ID: body.ID,
			  Name: body.Name,
			   Kind: SourceKind(strings.ToLower(strings.TrimSpace(body.Kind))),
			  Chain:    body.Chain,
			  Family:   family,
			   Endpoint: body.Endpoint,
			  Weight:  weight,
			  Enabled: body.Enabled,
			   StaleDur: staleDur,
		  }
		  if err := svc.RegisterSource(cfg); err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, cfg)
	   }
  }

  func handleListSources(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  sources := svc.Oracle().Sources()
		   c.JSON(http.StatusOK, gin.H{"sources": sources, "count": len(sources)})
	  }
   }
