package cvquotelease

import (
	"context"
	"strings"
)

   func (e *Engine) sealProofLocked(ctx context.Context, leaseID, quoteID, user, from, to string, kind DriftKind, oldGen, newGen int64, oldRate, newRate, oldBook, newBook, oldPrint, newPrint, detail string) CvQuoteDriftProof {
	  p := CvQuoteDriftProof{
		   ID: newID(),
		   LeaseID: leaseID,
		   QuoteID: quoteID,
		   UserID:  user,
		  FromAsset: NormalizeAsset(from),
		  ToAsset:   NormalizeAsset(to),
		   Kind: kind,
		  LeaseMarkGen: oldGen,
		   LiveMarkGen:  newGen,
		   LeaseRate:    oldRate,
		  LiveRate:     newRate,
		   LeaseBook:    oldBook,
		  LiveBook:   newBook,
		   LeasePrint: oldPrint,
		   LivePrint:  newPrint,
		  Blocked:   true,
		  Sealed:    true,
		   Detail:    detail,
		   CreatedAt: e.now(),
	  }
	  if e.store != nil {
		  saved, err := e.store.SaveProof(ctx, p)
		   if err == nil {
			  return saved
		   }
	   }
	  return p
   }

  func (e *Engine) ExportProofs(ctx context.Context, req ProofExportReq) (ProofExportResp, error) {
	  if err := e.require(); err != nil {
		   return ProofExportResp{}, err
	  }
	  limit := req.Limit
	   var proofs []CvQuoteDriftProof
	   var err error
	   switch {
	  case strings.TrimSpace(req.LeaseID) != "":
		   proofs, err = e.store.ListProofsByLease(ctx, req.LeaseID, limit)
	  case strings.TrimSpace(req.QuoteID) != "":
		   proofs, err = e.store.ListProofsByQuote(ctx, req.QuoteID, limit)
	  case strings.TrimSpace(req.UserID) != "":
		  proofs, err = e.store.ListProofsByUser(ctx, req.UserID, limit)
	  default:
		  // 按用户空串列出最近证明不可靠；返回空以免跨用户泄漏
		  proofs = nil
	   }
	   if err != nil {
		  return ProofExportResp{}, err
	   }
	   return ProofExportResp{Proofs: proofs, Count: len(proofs)}, nil
  }

  func (e *Engine) GetProof(ctx context.Context, id string) (CvQuoteDriftProof, error) {
	  if err := e.require(); err != nil {
		  return CvQuoteDriftProof{}, err
	  }
	   return e.store.GetProof(ctx, id)
   }
