package hmacwritelease

import (
	"context"
	"net/http"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/hmacfence"
)

  func init() {
	  apierr.Register(ErrNonceReplay, http.StatusUnauthorized)
	   apierr.Register(ErrLeaseNotFound, http.StatusNotFound)
	  apierr.Register(ErrLeaseExpired, http.StatusConflict)
	  apierr.Register(ErrLeaseRevoked, http.StatusConflict)
	  apierr.Register(ErrHaltGenMismatch,
		  http.StatusConflict)
	  apierr.Register(ErrLeaseConsumed, http.StatusConflict)
	   apierr.Register(ErrIllegalTransition,
		   http.StatusInternalServerError)
	   apierr.Register(ErrInvalidInput, http.StatusBadRequest)
	  apierr.Register(ErrHalted, http.StatusForbidden)
	  apierr.Register(ErrNoLeaseForWrite, http.StatusForbidden)
  }

// Register 挂载 hmacwritelease API 路由并启动后台 Worker。
// 供 cmd/hmac-write-lease 传入 svcboot.Main。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  var (
		   leases LeaseRepo
		  nonces NonceSeenRepo
		   proofs DriftProofRepo
		   gens   HaltGenRepo
	  )
	   if rt.SQL != nil && !rt.Mem {
		  leases = NewSQLLeaseRepo(rt.SQL)
		   nonces = NewSQLNonceSeenRepo(rt.SQL)
		  proofs = NewSQLDriftProofRepo(rt.SQL)
		  gens = NewSQLHaltGenRepo(rt.SQL)
	   } else {
		  leases = NewMemLeaseRepo()
		   nonces = NewMemNonceSeenRepo()
		   proofs = NewMemDriftProofRepo()
		   gens = NewMemHaltGenRepo()
	  }
	  // 构造 hmacfence.Service（只读引用 KeyRing）。
	  skew := 30 * time.Second
	  if rt.Cfg != nil && rt.Cfg.Auth.MaxSkew > 0 {
		   skew = rt.Cfg.Auth.MaxSkew
	  }
	  fence := hmacfence.New(skew)
	  var keys map[string]string
	  if rt.Cfg != nil {
		   keys = rt.Cfg.Auth.HMACKeys
	  }
	  ring := hmacfence.NewKeyRing(keys)
	  eng := NewEngine(fence, ring, leases, nonces, proofs, gens)
	  RegisterRoutes(v1, eng)
	   ctx := context.Background()
	   // 后台 Worker：租约过期扫描。
	   expiryWorker := NewLeaseExpiryWorker(eng, WorkerConfig{})
	  go expiryWorker.Run(ctx)
	  // 后台 Worker：Halt 翻转批量吊销。
	  haltWorker := NewHaltRevokeWorker(eng, WorkerConfig{})
	   go haltWorker.Run(ctx)
	   // 后台 Worker：nonce 窗清理。
	  nonceWorker := NewNonceCleanWorker(nonces, WorkerConfig{})
	   go nonceWorker.Run(ctx)
   }
