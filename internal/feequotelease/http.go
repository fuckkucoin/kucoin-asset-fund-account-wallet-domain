package feequotelease

import (
	"errors"
	"net/http"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/feeeng"
	"account-assets.kucoin.net/funding/internal/mw"
	"github.com/gin-gonic/gin"
)

// Register 将 feequotelease 路由挂载到 v1 RouterGroup。

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  book := feeeng.DefaultBook()
	   if rt != nil && !rt.Mem && rt.SQL != nil {
		   book.BindSQL(rt.SQL)
	   }
	   adapter := NewFeeEngAdapter(book)
	  var store Store
	   if rt != nil && !rt.Mem && rt.SQL != nil {
		  store = NewSQLStore(rt.SQL)
	  } else {
		   store = NewInMemStore()
	  }
	   svc := NewWithStore(store, adapter)
	   g := v1.Group("/fees")
	  {
		   g.POST("/quote-lease", makeQuoteLease(svc))
		  g.POST("/apply-bound", makeApplyBound(svc))
		  g.POST("/put-schedule", makePutSchedule(svc, book))
		   g.GET("/leases/:id", makeGetLease(svc))
		   g.GET("/drift-proofs", makeListProofs(svc))
		  g.POST("/drift-proofs/export", makeExportProofs(svc))
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// Handler builders
// ─────────────────────────────────────────────────────────────────────────────

  func makeQuoteLease(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		  var body struct {
			  Kind  string `json:"kind"`
			   Asset string `json:"asset"`
			  Gross string `json:"gross"`
			  Volume     string `json:"volume"`
			   TTLSeconds int    `json:"ttl_seconds"`
			  IdempotencyKey string `json:"idempotency_key"`
			   Tiers []struct {
				   Notional string `json:"notional"`
				  BPS      int    `json:"bps"`
			  } `json:"tiers"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   tiers := make([]Tier,
			   0, len(body.Tiers))
		  for _, t := range body.Tiers {
			  tiers = append(tiers, Tier{Notional: t.Notional, BPS: t.BPS})
		  }
		  req := QuoteLeaseReq{
			  Kind:  body.Kind,
			  Asset: body.Asset,
			   Gross:  body.Gross,
			   Volume: body.Volume,
			   TTLSeconds: body.TTLSeconds,
			  IdempotencyKey: body.IdempotencyKey,
		   }
		  lease, err := svc.QuoteLease(c.Request.Context(), req, tiers)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(http.StatusOK, lease)
	   }
  }

  func makeApplyBound(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   var body struct {
			   LeaseID string `json:"lease_id"`
			   IdempotencyKey string `json:"idempotency_key"`
			  Tiers []struct {
				  Notional string `json:"notional"`
				   BPS      int    `json:"bps"`
			  } `json:"tiers"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  tiers := make([]Tier,
			   0, len(body.Tiers))
		   for _, t := range body.Tiers {
			  tiers = append(tiers, Tier{Notional: t.Notional, BPS: t.BPS})
		  }
		  req := ApplyBoundReq{
			  LeaseID: body.LeaseID,
			  IdempotencyKey: body.IdempotencyKey,
		  }
		   result, err := svc.ApplyBound(c.Request.Context(), req, tiers)
		   if err != nil {
			   statusCode := http.StatusUnprocessableEntity
			  if errors.Is(err, ErrNoLease) {
				   statusCode = http.StatusForbidden
			  } else if errors.Is(err, ErrLeaseNotFound) {
				   statusCode = http.StatusNotFound
			   }
			   c.JSON(statusCode, gin.H{
				   "error": err.Error(),
				  "code": mapErrCode(err),
				   "host": "fee-lease.kucoin.net",
			   })
			   return
		  }
		  c.JSON(http.StatusOK, result)
	  }
  }

   func makePutSchedule(svc *Service, book *feeeng.Book) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   var body struct {
			   Kind   string `json:"kind"`
			  Asset  string `json:"asset"`
			  BPS    int    `json:"bps"`
			  MinFee string `json:"min_fee"`
			   MaxFee string `json:"max_fee"`
			  Flat string `json:"flat"`
			   Active bool `json:"active"`
			   PutBy string `json:"put_by"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  // 先更新 feeeng.Book（只读适配器会读到新值）
		  if err := book.Put(feeeng.Schedule{
			  Kind:  feeeng.Kind(body.Kind),
			  Asset: body.Asset,
			  BPS: body.BPS,
			   MinFee: body.MinFee,
			   MaxFee: body.MaxFee,
			  Flat:   body.Flat,
			   Active: body.Active,
		   }); err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  req := SchedulePutReq{
			  Kind: body.Kind,
			   Asset: body.Asset,
			  BPS:    body.BPS,
			   MinFee: body.MinFee,
			   MaxFee: body.MaxFee,
			  Flat:   body.Flat,
			   Active: body.Active,
			  PutBy: body.PutBy,
		  }
		   gen, revoked, err := svc.PutSchedule(c.Request.Context(), req)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(http.StatusOK, gin.H{
			  "generation":    gen,
			   "revoked_count": revoked,
			   "host":          "fee-lease.kucoin.net",
		  })
	   }
  }

   func makeGetLease(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   id := c.Param("id")
		   lease, err := svc.GetLease(c.Request.Context(), id)
		   if err != nil {
			   if errors.Is(err, ErrLeaseNotFound) {
				  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				  return
			   }
			  mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, lease)
	   }
   }

   func makeListProofs(svc *Service) gin.HandlerFunc {
	   return func(c *gin.Context) {
		   proofs, err := svc.store.ListProofs(c.Request.Context(), ProofExportBatchSize)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(http.StatusOK, gin.H{
			  "proofs": proofs,
			  "count":  len(proofs),
			   "host": "fee-lease.kucoin.net",
		  })
	   }
  }

   func makeExportProofs(svc *Service) gin.HandlerFunc {
	  return func(c *gin.Context) {
		   proofs,
			  err := svc.ExportProofs(c.Request.Context())
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{
			  "exported": len(proofs),
			  "proofs": proofs,
			   "host":   "fee-lease.kucoin.net",
		   })
	  }
  }

   func mapErrCode(err error) string {
	   switch {
	   case errors.Is(err, ErrNoLease):
		   return "no_lease"
	  case errors.Is(err,
		  ErrLeaseExpired):
		  return "lease_expired"
	  case errors.Is(err, ErrLeaseRevoked):
		  return "lease_revoked"
	   case errors.Is(err, ErrLeaseConsumed):
		   return "lease_consumed"
	   case errors.Is(err, ErrSchGenDrift):
		   return "sch_gen_drift"
	  case errors.Is(err, ErrFingerprintMismatch):
		   return "fingerprint_mismatch"
	   default:
		   return "internal_error"
	  }
   }
