package pumpmarkfence

import (
	"context"
	"net/http"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/persist"
)

  func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrInvalidInput,
		   http.StatusBadRequest)
	  apierr.Register(ErrHandler, http.StatusBadRequest)
	  apierr.Register(ErrGenMissing, http.StatusForbidden)
	   apierr.Register(ErrGenNotFound, http.StatusNotFound)
	  apierr.Register(ErrGenExpired,
		   http.StatusConflict)
	  apierr.Register(ErrGenStale, http.StatusConflict)
	  apierr.Register(ErrGenMismatch, http.StatusConflict)
	   apierr.Register(ErrFingerprint,
		  http.StatusConflict)
	   apierr.Register(ErrIllegalTransit, http.StatusConflict)
	   apierr.Register(ErrAlreadyApplied, http.StatusConflict)
	  apierr.Register(ErrDupEffect, http.StatusConflict)
	   apierr.Register(ErrHalfMarkOpen, http.StatusConflict)
	   apierr.Register(ErrHalfMarkTaken, http.StatusConflict)
	   apierr.Register(ErrHalfMarkClosed, http.StatusConflict)
	  apierr.Register(ErrHalfMarkNotFound, http.StatusNotFound)
	   apierr.Register(ErrProofNotFound, http.StatusNotFound)
	   apierr.Register(ErrLagReadonly, http.StatusForbidden)
	  apierr.Register(ErrCASFailed, http.StatusConflict)
	  apierr.Register(ErrPoisonReplay, http.StatusConflict)
	  apierr.Register(ErrMarkDenied, http.StatusForbidden)
	   apierr.Register(ErrActor,
		   http.StatusBadRequest)
	  apierr.Register(ErrOutbox, http.StatusConflict)
	  apierr.Register(ErrIdemConflict, http.StatusConflict)
	  apierr.Register(ErrTTL, http.StatusBadRequest)
	   apierr.Register(ErrEffectKey, http.StatusNotFound)
	   apierr.Register(ErrResumeToken, http.StatusConflict)
	  apierr.Register(ErrDeliveredOnly, http.StatusConflict)
  }

// Register 供 cmd/outbox-pump-mark-fence 挂载。
   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  var box *persist.Outbox
	  if rt != nil && rt.KV != nil {
		   box = persist.NewOutbox(rt.KV)
	  } else {
		   box = persist.NewOutbox(nil)
	  }
	  var (
		   gens GenRepo
		   halves HalfRepo
		  proofs  ProofRepo
		  applied AppliedRepo
		   audits AuditRepo
	  )
	  if rt != nil && rt.SQL != nil && !rt.Mem {
		   gens = NewSQLGenRepo(rt.SQL)
		  halves = NewSQLHalfRepo(rt.SQL)
		  proofs = NewSQLProofRepo(rt.SQL)
		  applied = NewSQLAppliedRepo(rt.SQL)
		  audits = NewSQLAuditRepo(rt.SQL)
	   } else {
		   gens = NewMemGenRepo()
		  halves = NewMemHalfRepo()
		   proofs = NewMemProofRepo()
		  applied = NewMemAppliedRepo()
		   audits = NewMemAuditRepo()
	   }
	   eng := NewEngine(box, gens, halves, proofs, applied, audits)
	   RegisterRoutes(v1, eng)
	   StartWorkers(context.Background(), eng, WorkerConfig{})
   }
