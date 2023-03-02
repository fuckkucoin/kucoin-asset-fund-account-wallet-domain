package dualctl

import (
	"context"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

   func (b *Book) BindSQL(db *persist.SQLDB) *Book {
	   if b != nil {
		   b.sql = db
	   }
	  return b
  }

  func (b *Book) sqlOn() bool {
	  return b != nil && b.sql != nil && b.sql.DB != nil
   }

  func (b *Book) persistReq(r *Request) error {
	   if !b.sqlOn() || r == nil {
		  return nil
	  }
	  ctx := context.Background()
	   repo := sqlrepo.NewDualCtl(b.sql)
	  if err := repo.PutRequest(ctx, sqlrepo.DualRequest{
		  ID: r.ID, Kind: r.Kind, Subject: r.Subject, OpenedBy: r.OpenedBy, Need: r.Need,
		  State: string(r.State), Note: r.Note, ExpiresAt: r.ExpiresAt, Fence: r.Fence,
		  CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	   }); err != nil {
		  return err
	  }
	  for _, v := range r.Votes {
		  if err := repo.PutVote(ctx, sqlrepo.DualVote{RequestID: r.ID, Actor: v.Actor, Allow: v.Allow, At: v.At}); err != nil {
			   return err
		  }
	  }
	   return nil
   }

   func (b *Book) loadSQL(id string) (*Request, bool) {
	  if !b.sqlOn() {
		  return nil, false
	   }
	  repo := sqlrepo.NewDualCtl(b.sql)
	   got, err := repo.GetRequest(context.Background(), id)
	  if err != nil {
		   return nil, false
	  }
	  votes, _ := repo.ListVotes(context.Background(), id)
	   r := &Request{
		  ID: got.ID, Kind: got.Kind, Subject: got.Subject, OpenedBy: got.OpenedBy, Need: got.Need,
		   State: State(got.State),
		   Note: got.Note, ExpiresAt: got.ExpiresAt, Fence: got.Fence,
		   CreatedAt: got.CreatedAt, UpdatedAt: got.UpdatedAt,
	   }
	   for _, v := range votes {
		  r.Votes = append(r.Votes, Vote{Actor: v.Actor, Allow: v.Allow, At: v.At})
	  }
	   return r, true
  }
