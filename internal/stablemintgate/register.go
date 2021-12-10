package stablemintgate

import (
	"context"
	"net/http"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/stablebr"
)

  func init() {
	  apierr.Register(ErrUnavailable,
		   http.StatusServiceUnavailable)
	  apierr.Register(ErrInvalidInput, http.StatusBadRequest)
	   apierr.Register(ErrLeaseNotFound,
		  http.StatusNotFound)
	  apierr.Register(ErrLeaseExpired, http.StatusConflict)
	   apierr.Register(ErrLeaseRevoked, http.StatusConflict)
	   apierr.Register(ErrLeaseConsumed, http.StatusConflict)
	  apierr.Register(ErrLeasePosting,
		   http.StatusConflict)
	  apierr.Register(ErrIllegalTransition, http.StatusConflict)
	  apierr.Register(ErrNoLease, http.StatusForbidden)
	  apierr.Register(ErrAttestIncomplete, http.StatusConflict)
	  apierr.Register(ErrAttestExpired, http.StatusConflict)
	  apierr.Register(ErrFingerprintDrift,
		   http.StatusConflict)
	   apierr.Register(ErrGenMismatch,
		  http.StatusConflict)
	  apierr.Register(ErrKindMismatch, http.StatusConflict)
	   apierr.Register(ErrAssetMismatch, http.StatusConflict)
	  apierr.Register(ErrActorMismatch,
		   http.StatusConflict)
	  apierr.Register(ErrIdemReplay, http.StatusConflict)
	   apierr.Register(ErrHalfOpen, http.StatusConflict)
	   apierr.Register(ErrHalfNotFound, http.StatusNotFound)
	   apierr.Register(ErrProofNotFound, http.StatusNotFound)
	  apierr.Register(ErrBridge,
		   http.StatusBadRequest)
	   apierr.Register(ErrLedger, http.StatusConflict)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
  }

// Register 供 cmd/stable-mint-gate 挂载。

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   br := stablebr.New()
	   if rt != nil && rt.Ledger != nil {
		   br.BindLedger(rt.Ledger)
	  }
	   var (
		   leases LeaseRepo
		   halves HalfRepo
		   audits AuditRepo
		   gens   GenRepo
		   proofs ProofRepo
	   )
	  if rt != nil && rt.SQL != nil && !rt.Mem {
		  leases = NewSQLLeaseRepo(rt.SQL)
		  halves = NewSQLHalfRepo(rt.SQL)
		  audits = NewSQLAuditRepo(rt.SQL)
		  gens = NewSQLGenRepo(rt.SQL)
		  proofs = NewSQLProofRepo(rt.SQL)
	  } else {
		   leases = NewMemLeaseRepo()
		   halves = NewMemHalfRepo()
		  audits = NewMemAuditRepo()
		  gens = NewMemGenRepo()
		   proofs = NewMemProofRepo()
	   }
	  eng := NewEngine(br, leases, halves, audits, gens, proofs)
	   RegisterRoutes(v1, eng)
	   StartWorkers(context.Background(), eng, WorkerConfig{})
   }
