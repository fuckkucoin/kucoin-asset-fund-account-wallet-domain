package casestore

import (
	"context"
	"account-assets.kucoin.net/funding/internal/cases"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

  func (r *Repo) BindSQL(db *persist.SQLDB) *Repo {
	   if r != nil {
		   r.sql = db
	   }
	  return r
  }

   func (r *Repo) sqlOn() bool {
	  return r != nil && r.sql != nil && r.sql.DB != nil
  }

   func (r *Repo) loadSQL(ctx context.Context, movementID string) (rec, persist.Fence, error) {
	   got, err := sqlrepo.NewCases(r.sql).GetByMovement(ctx, movementID)
	   if err != nil {
		   return rec{}, persist.Fence{}, err
	   }
	  out := rec{Case: cases.Case{
		  ID: got.ID, MovementID: got.MovementID, UserID: got.UserID, Kind: got.Kind, Reason: got.Reason,
		   State: cases.State(got.State),
		  Assignee: got.Assignee, DecidedBy: got.DecidedBy, PolicyVer: got.PolicyVer,
		   FenceEpoch: got.Fence.Epoch, FenceToken: got.Fence.Token, CreatedAt: got.CreatedAt, UpdatedAt: got.UpdatedAt,
	  }}
	   aud, err := sqlrepo.NewCases(r.sql).ListAudit(ctx, got.ID)
	   if err == nil {
		  for _, a := range aud {
			   out.Audit = append(out.Audit, cases.AuditEvent{
				   ID: a.ID, CaseID: a.CaseID, Actor: a.Actor, Event: a.Event,
				  From: cases.State(a.From), To: cases.State(a.To), Note: a.Note, At: a.At,
			  })
		  }
	  }
	   return out, got.Fence, nil
   }

   func (r *Repo) saveSQL(ctx context.Context, cur rec, fence persist.Fence) error {
	  repo := sqlrepo.NewCases(r.sql)
	  if err := repo.Put(ctx, sqlrepo.CaseRow{
		  ID: cur.Case.ID, MovementID: cur.Case.MovementID, UserID: cur.Case.UserID, Kind: cur.Case.Kind,
		  Reason: cur.Case.Reason, State: string(cur.Case.State), Assignee: cur.Case.Assignee, DecidedBy: cur.Case.DecidedBy,
		  PolicyVer: cur.Case.PolicyVer,
		   Fence:     fence, CreatedAt: cur.Case.CreatedAt, UpdatedAt: cur.Case.UpdatedAt,
	  }); err != nil {
		   return err
	  }
	   if n := len(cur.Audit); n > 0 {
		   last := cur.Audit[n-1]
		   _ = repo.PutAudit(ctx, sqlrepo.CaseAuditRow{
			   ID: last.ID, CaseID: last.CaseID, Actor: last.Actor, Event: last.Event,
			   From: string(last.From), To: string(last.To), Note: last.Note, At: last.At,
		  })
	   }
	  return nil
   }
