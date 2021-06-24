package fundpath

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindPath = "fundpath"

   func (s *Service) Save(ctx context.Context, p Path) (Path, error) {
	  if s == nil || s.kv == nil {
		   return Path{}, ErrUnavailable
	   }
	   body, err := json.Marshal(p)
	   if err != nil {
		  return Path{}, err
	  }
	  err = s.kv.InTx(ctx, func(ctx context.Context) error {
		  cur, ferr := s.kv.Get(ctx, kindPath, p.Idem)
		   if ferr != nil && !errors.Is(ferr, persist.ErrNotFound) {
			   return ferr
		   }
		  exp := persist.Fence{}
		  if ferr == nil {
			  var old Path
			   if err := json.Unmarshal(cur.Body, &old); err != nil {
				   return err
			  }
			  if old.UserID != p.UserID || old.Chain != p.Chain || old.Asset != p.Asset {
				   return persist.ErrConflict
			   }
			   exp = cur.Fence
		   }
		   return s.kv.Put(ctx, persist.Row{Kind: kindPath, Key: p.Idem, Body: body}, exp)
	  })
	  if err != nil {
		  if errors.Is(err, persist.ErrStaleFence) {
			   return Path{}, ErrFence
		   }
		   if errors.Is(err, persist.ErrConflict) {
			  return Path{}, ErrIdem
		  }
		  return Path{}, err
	  }
	   got, err := s.kv.Get(ctx, kindPath, p.Idem)
	  if err != nil {
		  return Path{}, err
	  }
	   var out Path
	   if err := json.Unmarshal(got.Body, &out); err != nil {
		   return Path{}, err
	  }
	   out.Fence = got.Fence
	   return out, nil
   }

   func (s *Service) Load(ctx context.Context, idem string) (Path, error) {
	   if s == nil || s.kv == nil {
		  return Path{}, ErrUnavailable
	  }
	   row, err := s.kv.Get(ctx, kindPath, strings.TrimSpace(idem))
	   if err != nil {
		  return Path{}, err
	  }
	  var out Path
	   if err := json.Unmarshal(row.Body, &out); err != nil {
		  return Path{}, err
	  }
	   out.Fence = row.Fence
	   return out, nil
   }

  func DualHint() []persist.RegionDSN {
	  return persist.DualRegion()
   }
