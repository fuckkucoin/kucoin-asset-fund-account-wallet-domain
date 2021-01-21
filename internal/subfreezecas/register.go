package subfreezecas

import (
	"context"
	"net/http"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/subacct"
)

   func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrInvalidInput, http.StatusBadRequest)
	   apierr.Register(ErrParent, http.StatusBadRequest)
	   apierr.Register(ErrChild, http.StatusBadRequest)
	  apierr.Register(ErrCASNotFound, http.StatusNotFound)
	  apierr.Register(ErrCASFrozen, http.StatusConflict)
	  apierr.Register(ErrCASThawed, http.StatusConflict)
	  apierr.Register(ErrCASStale,
		  http.StatusConflict)
	   apierr.Register(ErrCASMissing, http.StatusForbidden)
	   apierr.Register(ErrCASMismatch, http.StatusConflict)
	  apierr.Register(ErrFingerprint, http.StatusConflict)
	   apierr.Register(ErrGateMisaligned, http.StatusConflict)
	  apierr.Register(ErrIllegalTransit, http.StatusConflict)
	   apierr.Register(ErrVelocityRevoked, http.StatusConflict)
	  apierr.Register(ErrPunchDetected, http.StatusConflict)
	   apierr.Register(ErrProofNotFound, http.StatusNotFound)
	  apierr.Register(ErrProofAcked, http.StatusConflict)
	   apierr.Register(ErrUnfreezeChild, http.StatusConflict)
	  apierr.Register(ErrClosed, http.StatusConflict)
	  apierr.Register(ErrIdemConflict, http.StatusConflict)
	  apierr.Register(ErrCompensate, http.StatusConflict)
	  apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	   apierr.Register(ErrAsset,
		   http.StatusBadRequest)
	   apierr.Register(ErrRevokeNotFound, http.StatusNotFound)
	   apierr.Register(ErrBooks, http.StatusConflict)
  }

// Register 供 cmd/sub-freeze-cas 挂载。
   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  var books *subacct.Service
	   if rt != nil {
		   var db = rt.SQL
		  if rt.Mem {
			  db = nil
		  }
		  books = subacct.NewWithKV(rt.KV).BindSQL(db)
	  } else {
		   books = subacct.New()
	  }
	   var (
		   cas CASRepo
		   proofs  ProofRepo
		  revokes RevokeRepo
		  audits  AuditRepo
		  journals JournalRepo
	  )
	   useSQL := rt != nil && rt.SQL != nil && !rt.Mem
	  prod, storage := false, ""
	  if rt != nil && rt.Cfg != nil {
		  prod = authn.ProductionProfile(rt.Cfg.Auth.Kind, rt.Cfg.Profile)
		   storage = rt.Cfg.Storage
	   }
	   if err := persist.RefuseMemRegister(prod, storage, !useSQL); err != nil {
		   if rt != nil {
			   rt.Log.Fatal().Err(err).Msg("subfreezecas register store")
		  }
		   panic(err)
	  }
	   if useSQL {
		   cas = NewSQLCASRepo(rt.SQL)
		   proofs = NewSQLProofRepo(rt.SQL)
		   revokes = NewSQLRevokeRepo(rt.SQL)
		   audits = NewSQLAuditRepo(rt.SQL)
		  journals = NewSQLJournalRepo(rt.SQL)
	  } else {
		  cas = NewMemCASRepo()
		  proofs = NewMemProofRepo()
		   revokes = NewMemRevokeRepo()
		  audits = NewMemAuditRepo()
		   journals = NewMemJournalRepo()
	   }
	  eng := NewEngine(books, cas, proofs, revokes, audits, journals)
	  RegisterRoutes(v1, eng)
	   StartWorkers(context.Background(), eng, WorkerConfig{})
  }
