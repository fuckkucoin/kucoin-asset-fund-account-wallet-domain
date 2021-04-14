package disputepaygate

import (
	"context"
	"net/http"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/disputedesk"
)

   func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrInvalidInput, http.StatusBadRequest)
	   apierr.Register(ErrCaseNotFound, http.StatusNotFound)
	  apierr.Register(ErrLeaseNotFound,
		  http.StatusNotFound)
	  apierr.Register(ErrLeaseExpired, http.StatusConflict)
	  apierr.Register(ErrLeaseRevoked, http.StatusConflict)
	   apierr.Register(ErrLeaseConsumed, http.StatusConflict)
	   apierr.Register(ErrIllegalTransition, http.StatusConflict)
	   apierr.Register(ErrNoLeaseForCredit, http.StatusForbidden)
	   apierr.Register(ErrNoLeaseForDebit, http.StatusForbidden)
	  apierr.Register(ErrEvidenceDrift, http.StatusConflict)
	  apierr.Register(ErrGenMismatch, http.StatusConflict)
	  apierr.Register(ErrAssetMismatch, http.StatusConflict)
	   apierr.Register(ErrDecisionMismatch, http.StatusConflict)
	  apierr.Register(ErrDesk,
		   http.StatusBadRequest)
   }

// Register 供 cmd/dispute-pay-gate 挂载。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  desk := disputedesk.New()
	  var (
		  pays PayLeaseRepo
		  claws ClawLeaseRepo
		  proofs ProofRepo
		   audits AuditRepo
		   gens GenRepo
	  )
	   if rt != nil && rt.SQL != nil && !rt.Mem {
		   pays = NewSQLPayLeaseRepo(rt.SQL)
		   claws = NewSQLClawLeaseRepo(rt.SQL)
		  proofs = NewSQLProofRepo(rt.SQL)
		   audits = NewSQLAuditRepo(rt.SQL)
		   gens = NewSQLGenRepo(rt.SQL)
	   } else {
		  pays = NewMemPayLeaseRepo()
		  claws = NewMemClawLeaseRepo()
		   proofs = NewMemProofRepo()
		   audits = NewMemAuditRepo()
		   gens = NewMemGenRepo()
	   }
	  eng := NewEngine(desk, pays, claws, proofs, audits, gens)
	  binder := NewBinder(eng, NewMemoryPoster())
	  disputedesk.SetPayResolveHook(func(id, actor, decision, note string) (disputedesk.Case, string, error) {
		   res, err := eng.ResolveWithPayLease(context.Background(), ResolveReq{
			  CaseID: id, Actor: actor, Decision: decision, Note: note,
		  })
		   if res == nil {
			   return disputedesk.Case{}, "", err
		   }
		   return res.Case, res.PayLeaseID, err
	  })
	   disputedesk.RegisterWith(v1, rt, desk)
	   RegisterRoutes(v1, eng, binder)
	   StartWorkers(context.Background(), eng, WorkerConfig{})
   }
