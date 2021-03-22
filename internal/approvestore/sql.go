package approvestore

import (
	"context"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

  func (s *Store) BindSQL(db *persist.SQLDB) *Store {
	  if s != nil {
		  s.sql = db
	   }
	  return s
  }

   func (s *Store) sqlOn() bool {
	   return s != nil && s.sql != nil && s.sql.DB != nil
   }

  func (s *Store) loadSQL(ctx context.Context, id string) (Rec, persist.Fence, error) {
	  got, err := sqlrepo.NewApprovals(s.sql).Get(ctx, id)
	  if err != nil {
		  return Rec{}, persist.Fence{}, err
	  }
	   return Rec{Approval: custody.Approval{
		  ID: got.ID, Kind: custody.ApprovalKind(got.Kind), MovementID: got.MovementID, Chain: got.Chain,
		   To: got.Dest, Amount: got.Amount, RequestedBy: got.RequestedBy, ApprovedBy: got.ApprovedBy,
		  State: custody.ApprovalState(got.State), Note: got.Note, CreatedAt: got.CreatedAt, UpdatedAt: got.UpdatedAt,
	  }}, got.Fence, nil
  }

   func (s *Store) saveSQL(ctx context.Context, id string, r Rec, fence persist.Fence) error {
	  return sqlrepo.NewApprovals(s.sql).Put(ctx, sqlrepo.ApprovalRow{
		  ID: id, Kind: string(r.Approval.Kind), MovementID: r.Approval.MovementID, Chain: r.Approval.Chain,
		   Dest: r.Approval.To, Amount: r.Approval.Amount, RequestedBy: r.Approval.RequestedBy,
		   ApprovedBy: r.Approval.ApprovedBy, State: string(r.Approval.State), Note: r.Approval.Note,
		   Fence:     fence,
		   CreatedAt: r.Approval.CreatedAt, UpdatedAt: r.Approval.UpdatedAt,
	   })
  }
