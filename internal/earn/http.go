package earn

import (
	"context"
	"strconv"
	"time"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/cvfund"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New(rt.Ledger).BindKV(rt.KV).BindHolds(holdcat.New(rt.KV))
	  // 绑 SQL 主存并 Hydrate；OpenMem 跳过。
	  if rt != nil && !rt.Mem && rt.SQL != nil {
		  svc.BindSQL(rt.SQL)
		   _ = svc.Hydrate(context.Background())
		  // 空目录时把 DefaultCatalog 种子落库，供跨实例一致。
		  if cat := svc.Catalog(); cat != nil && cat.sqlOn() {
			   h := cat.Health()
			  if n, _ := h["hydrated_n"].(int); n == 0 {
				  _ = cat.PersistAll(context.Background())
				   _ = cat.Hydrate(context.Background())
			   }
		   }
	   }
	   registerRoutes(v1, rt, svc)
	  registerOpsRoutes(v1, svc)
   }

  func registerRoutes(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  v1.GET("/earn/positions", func(c *gin.Context) {
		  rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  uid, err := validate.UserID(rawUID)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   // 支持 status + currentPage/pageSize SQL 分页；无分页参数保持全量兼容。
		   status := strings.TrimSpace(c.Query("status"))
		   pageRaw := strings.TrimSpace(c.Query("currentPage"))
		   sizeRaw := strings.TrimSpace(c.Query("pageSize"))
		  if pageRaw != "" || sizeRaw != "" || status != "" {
			  page := validate.Page(atoiDefault(pageRaw,
				   1))
			   size := validate.Limit(atoiDefault(sizeRaw, 50), 50, 200)
			  items,
				   total, pageOut, sizeOut, totalPage, err := svc.ListPaged(c.Request.Context(), uid, status, page, size)
			  if err != nil {
				   mw.WriteProblem(c, err)
				  return
			  }
			   c.JSON(200, gin.H{
				  "user_id": uid, "positions": items,
				  "currentPage": pageOut, "pageSize": sizeOut, "totalNum": total, "totalPage": totalPage,
				  "status": status, "host": "account-book.kucoin.net",
			   })
			   return
		   }
		   c.JSON(200, gin.H{"user_id": uid, "positions": svc.List(uid)})
	   })
	   v1.POST("/earn/subscribe", func(c *gin.Context) {
		  var body struct {
			  UserID string `json:"user_id"`
			  ProductID string `json:"product_id"`
			   Asset          string `json:"asset"`
			  Amount         string `json:"amount"`
			  IdempotencyKey string `json:"idempotency_key"`
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
		   body.UserID = uid
		   pos, err := svc.Subscribe(c.Request.Context(), SubscribeReq{
			   UserID: body.UserID, ProductID: body.ProductID, Asset: body.Asset,
			   Amount: body.Amount,
			   IdempotencyKey: body.IdempotencyKey,
		  })
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, pos)
	   })
	  v1.POST("/earn/redeem", func(c *gin.Context) {
		   var body struct {
			   UserID     string `json:"user_id"`
			  PositionID string `json:"position_id"`
			   IdempotencyKey string `json:"idempotency_key"`
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
		   pos, err := svc.Redeem(c.Request.Context(), RedeemReq{
			   UserID: body.UserID, PositionID: body.PositionID, IdempotencyKey: body.IdempotencyKey,
		  })
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(200, pos)
	   })
	   v1.POST("/earn/accrue", func(c *gin.Context) {
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
		   rows, err := svc.AccrueUser(body.UserID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"accruals": rows})
	  })
	   v1.POST("/earn/claim", func(c *gin.Context) {
		   var body struct {
			  PositionID string `json:"position_id"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  cl,
			  err := svc.Claim(body.PositionID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, cl)
	   })
	   v1.POST("/earn/tick", func(c *gin.Context) {
		   rep := svc.Tick(c.Request.Context())
		   c.JSON(200, gin.H{"job": rep, "host": "account-book.kucoin.net"})
	  })
	  v1.POST("/earn/redeem-early", func(c *gin.Context) {
		   var body struct {
			  UserID         string `json:"user_id"`
			   PositionID     string `json:"position_id"`
			   IdempotencyKey string `json:"idempotency_key"`
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
		  pos, pen, err := svc.RedeemEarly(c.Request.Context(), EarlyReq{
			  UserID: body.UserID, PositionID: body.PositionID, IdempotencyKey: body.IdempotencyKey,
		  })
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"position": pos, "penalty": pen})
	   })
	   v1.POST("/earn/post-interest", func(c *gin.Context) {
		  var body struct {
			  UserID         string `json:"user_id"`
			  PositionID     string `json:"position_id"`
	   v1.POST("/earn/convert-subscribe", func(c *gin.Context) {
		   var body ConvertSubscribeReq
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		   uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  body.UserID = uid
		  var cv *cvfund.Book
		  if rt != nil {
			  cv = rt.Convert
		  }
		   got, err := svc.ConvertSubscribe(c.Request.Context(), cv, body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
   }

// registerOpsRoutes 理财目录/EarlyBPS 运营热更新 + 软删恢复 + Hydrate 健康面。
// 写路径仅 ops；PUT 先写 SQL 再热表；undelete 清 deleted_at。
// 损坏 BPS/限额不得绕过申购/提前赎回闸；恢复后立即 Active。

   func registerOpsRoutes(v1 *gin.RouterGroup, svc *Service) {
	  v1.GET("/ops/earn/products", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		   }
		  cat := svc.Catalog()
		  // deleted=1 列软删行供运营恢复；否则列热表。
		   if strings.EqualFold(c.Query("deleted"), "1") || strings.EqualFold(c.Query("deleted"), "true") {
			  if cat == nil {
				   mw.WriteProblem(c,
					   ErrUnavailable)
				   return
			  }
			   list, err := cat.ListDeletedProducts(c.Request.Context())
			  if err != nil {
				  mw.WriteProblem(c, err)
				  return
			  }
			   c.JSON(200, gin.H{
				   "products": list, "deleted": true,
				   "health": svc.Health(), "host": "account-book.kucoin.net",
			   })
			   return
		  }
		  activeOnly := strings.EqualFold(c.Query("active"), "1") || strings.EqualFold(c.Query("active"), "true")
		   list := []Product{}
		   if cat != nil {
			  list = cat.List(activeOnly)
		   }
		  c.JSON(200, gin.H{
			  "products": list,
			  "health":   svc.Health(),
			   "host":     "account-book.kucoin.net",
		  })
	  })
	  v1.PUT("/ops/earn/products", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body struct {
			  Products []Product `json:"products"`
			  Replace  bool      `json:"replace"`
		   }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   cat := svc.Catalog()
		   if cat == nil {
			   mw.WriteProblem(c, ErrUnavailable)
			  return
		   }
		   softDeleted := int64(0)
		  if body.Replace && len(body.Products) > 0 {
			   // replace 热表 + 写库 + 软删多余产品行。
			  n, err := cat.PersistReplace(c.Request.Context(), body.Products, actor.ID)
			   if err != nil {
				  mw.WriteProblem(c, err)
				   return
			   }
			   softDeleted = n
		   } else {
			  for _, p := range body.Products {
				   if err := cat.PersistProduct(c.Request.Context(), p); err != nil {
					   mw.WriteProblem(c, err)
					  return
				   }
			  }
		  }
		   c.JSON(200, gin.H{
			   "products":  cat.List(false),
			  "persisted": cat.sqlOn(),
			  "soft_deleted": softDeleted,
			  "replace":      body.Replace,
			  "health": svc.Health(),
		  })
	   })
	  v1.POST("/ops/earn/products/undelete", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  var body struct {
			  ProductID string `json:"product_id"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		  cat := svc.Catalog()
		  if cat == nil {
			   mw.WriteProblem(c, ErrUnavailable)
			  return
		  }
		  // 恢复软删产品并装回热表，可立即申购。
		  got, err := cat.UndeleteProduct(c.Request.Context(), body.ProductID, actor.ID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, gin.H{"product": got, "persisted": cat.sqlOn(), "health": svc.Health()})
	   })
	   v1.GET("/ops/earn/products/audit", func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActQueryRead); err != nil {
			  if err2 := actor.Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		   }
		   cat := svc.Catalog()
		  if cat == nil {
			  mw.WriteProblem(c, ErrUnavailable)
			  return
		   }
		   limit := 100
		   if v := strings.TrimSpace(c.Query("limit")); v != "" {
			   if n, err := strconv.Atoi(v); err == nil {
				  limit = n
			   }
		  }
		   EmitProductAuditMetrics(c.Request.Context(), cat)
		  format := strings.ToLower(strings.TrimSpace(c.Query("format")))
		  var start, end time.Time
		  if v := strings.TrimSpace(c.Query("start_at")); v != "" {
			  if t, err := time.Parse(time.RFC3339, v); err == nil {
				   start = t.UTC()
			   }
		   }
		  if v := strings.TrimSpace(c.Query("end_at")); v != "" {
			  if t, err := time.Parse(time.RFC3339, v); err == nil {
				   end = t.UTC()
			  }
		   }
		   var (
			  list []sqlrepo.EarnProductAudit
			   err  error
		   )
		   if !start.IsZero() || !end.IsZero() || format == "csv" {
			  // CSV/窗查询走保留闸；缺省近 100 天。
			  if format == "csv" && limit < 1000 {
				  limit = 1000
			   }
			   list, err = cat.ListProductAuditWindow(c.Request.Context(), c.Query("product_id"), start, end, limit)
		   } else {
			  list, err = cat.ListProductAudit(c.Request.Context(), c.Query("product_id"), limit)
		   }
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   if format == "csv" {
			   RecordProductAuditAction("export")
			   c.Header("Content-Type", "text/csv; charset=utf-8")
			   c.Header("Content-Disposition", `attachment; filename="earn_product_audit.csv"`)
			   if err := WriteProductAuditCSV(c.Writer, list); err != nil {
				  mw.WriteProblem(c, err)
			  }
			   return
		  }
		   c.JSON(200, gin.H{
			  "audits": list, "product_id": c.Query("product_id"),
			  "retentionDays":    int(ProductAuditRetention / (24 * time.Hour)),
			   "exportWindowDays": int(ProductAuditExportWindow / (24 * time.Hour)),
			   "health":           svc.Health(),
		   })
	   })
	  v1.POST("/ops/earn/products/audit/purge", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   cat := svc.Catalog()
		  if cat == nil {
			   mw.WriteProblem(c, ErrUnavailable)
			  return
		   }
		  // 清扫超 365d 保留窗审计；须先 CSV 导出。
		   n, err := cat.PurgeProductAudit(c.Request.Context(), 0)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   EmitProductAuditMetrics(c.Request.Context(), cat)
		  c.JSON(200, gin.H{"purged": n, "retentionDays": int(ProductAuditRetention / (24 * time.Hour)), "actor": actor.ID})
	  })
	   // 按产品分片冷存审计 CSV（超越 SQL 保留/purge）；可选随后 purge。
	  v1.POST("/ops/earn/products/audit/archive", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		   if err := actor.Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		  cat := svc.Catalog()
		   if cat == nil {
			  mw.WriteProblem(c, ErrUnavailable)
			  return
		  }
		   var body struct {
			   Dir       string `json:"dir"`
			   ProductID string `json:"product_id"`
			   StartAt   string `json:"start_at"`
			  EndAt      string `json:"end_at"`
			   PurgeAfter bool   `json:"purge_after"`
		   }
		  _ = c.ShouldBindJSON(&body)
		  var start, end time.Time
		   if v := strings.TrimSpace(body.StartAt); v != "" {
			  if t, err := time.Parse(time.RFC3339, v); err == nil {
				  start = t.UTC()
			   }
		   }
		  if v := strings.TrimSpace(body.EndAt); v != "" {
			   if t, err := time.Parse(time.RFC3339, v); err == nil {
				  end = t.UTC()
			   }
		  }
		  got, err := cat.ColdArchiveProductAudit(c.Request.Context(), body.Dir, body.ProductID, actor.ID, start, end, body.PurgeAfter)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{
			   "archives": got, "actor": actor.ID, "purge_after": body.PurgeAfter,
			   "dir": ResolveColdArchiveDir(body.Dir), "path": "earn-audit-cold-archive",
		  })
	  })
	   v1.GET("/ops/earn/products/audit/archives", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		   }
		   cat := svc.Catalog()
		   if cat == nil {
			   mw.WriteProblem(c, ErrUnavailable)
			   return
		  }
		  limit := 100
		   if v := strings.TrimSpace(c.Query("limit")); v != "" {
			  if n, err := strconv.Atoi(v); err == nil {
				   limit = n
			   }
		  }
		   list, err := cat.ListColdArchives(c.Request.Context(), c.Query("product_id"), limit)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  EmitProductAuditMetrics(c.Request.Context(), cat)
		   c.JSON(200, gin.H{
			   "archives": list, "product_id": c.Query("product_id"),
			  "dir": ResolveColdArchiveDir(""), "path": "earn-audit-cold-list",
		   })
	  })
	   v1.PUT("/ops/earn/products/early-bps", func(c *gin.Context) {
		   actor := perm.FromRequest(c.Request, "")
		  if err := actor.Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			   ProductID string `json:"product_id"`
			   EarlyBPS int `json:"early_bps"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  cat := svc.Catalog()
		  if cat == nil {
			   mw.WriteProblem(c, ErrUnavailable)
			  return
		  }
		  got, err := cat.SetEarlyBPS(c.Request.Context(), body.ProductID, body.EarlyBPS)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{"product": got, "persisted": cat.sqlOn()})
	   })
	  v1.GET("/ops/earn/positions", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		  }
		  uid := strings.TrimSpace(c.Query("user_id"))
		  status := strings.TrimSpace(c.Query("status"))
		   page := validate.Page(atoiDefault(c.Query("currentPage"), 1))
		   size := validate.Limit(atoiDefault(c.Query("pageSize"), 50), 50, 200)
		  items, total, pageOut, sizeOut, totalPage, err := svc.ListPaged(c.Request.Context(), uid, status, page, size)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, gin.H{
			  "positions":   items,
			   "currentPage": pageOut, "pageSize": sizeOut, "totalNum": total, "totalPage": totalPage,
			   "user_id": uid, "status": status,
			  "health": svc.Health(),
			  "host":   "account-book.kucoin.net",
		   })
	   })
	   v1.GET("/ops/earn/health", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		   }
		  EmitProductAuditMetrics(c.Request.Context(), svc.Catalog())
		   c.JSON(200, gin.H{"earn": svc.Health(), "host": "account-book.kucoin.net"})
	  })
   }

  func atoiDefault(s string, def int) int {
	   s = strings.TrimSpace(s)
	  if s == "" {
		   return def
	  }
	  n := 0
	   for _, r := range s {
		  if r < '0' || r > '9' {
			   return def
		  }
		  n = n*10 + int(r-'0')
		  if n > 1000000 {
			  return def
		  }
	   }
	  return n
  }
