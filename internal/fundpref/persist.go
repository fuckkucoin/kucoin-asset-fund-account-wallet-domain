package fundpref

import (
	"context"
	"encoding/json"
	"fmt"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

   func (b *Book) persistRow(ctx context.Context, p Preference) error {
	   body, err := json.Marshal(p)
	   if err != nil {
		   return fmt.Errorf("%w: %v", ErrCorrupt, err)
	   }
	   if b.sqlOn() {
		  if err := sqlrepo.NewFundPref(b.sql).Put(ctx, sqlrepo.FundPrefRow{
			   UserID: p.UserID, Body: body, UpdatedAt: p.UpdatedAt, Version: p.Version,
		   }); err != nil {
			   return fmt.Errorf("%w: %v", ErrStore, err)
		   }
	  }
	  if b.kvOn() {
		  if err := putKV(ctx, b.kv, kindPrefRow, p.UserID, body); err != nil {
			  return fmt.Errorf("%w: %v", ErrStore, err)
		  }
	   }
	   if !b.sqlOn() && !b.kvOn() {
		  return ErrStore
	   }
	  return nil
   }

  func putKV(ctx context.Context, kv persist.KV, kind, key string, body []byte) error {
	  _, existed, err := persist.GetOrInit(ctx, kv, kind, key, body)
	  if err != nil {
		   return err
	  }
	   if existed {
		   return persist.Replace(ctx, kv, kind, key, body)
	  }
	  return nil
   }
