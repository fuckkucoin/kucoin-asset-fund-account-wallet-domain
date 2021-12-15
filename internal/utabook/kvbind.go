package utabook

import (
	"context"
	"encoding/json"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	kvKindRateCatalog = "aa_uta_rate"
	kvKeyRateCatalog = "catalog_v1"
)

   func (b *Book) BindKV(kv persist.KV) *Book {
	  if b != nil {
		   b.kv = kv
		   b.loadKV()
	   }
	  return b
   }

  func (b *Book) loadKV() {
	  if b == nil || b.kv == nil {
		   return
	   }
	  row, err := b.kv.Get(context.Background(), kvKindRateCatalog, kvKeyRateCatalog)
	  if err != nil || len(row.Body) == 0 {
		  return
	   }
	   var tiers []RateTier
	  if err := json.Unmarshal(row.Body, &tiers); err != nil {
		   return
	  }
	   b.RateCatalog().ReplaceAll(tiers)
  }

  func (b *Book) PersistRateCatalogKV() error {
	   if b == nil || b.kv == nil {
		   return nil
	  }
	  tiers := b.ListRateTiers("")
	  raw, err := json.Marshal(tiers)
	   if err != nil {
		   return err
	   }
	  var exp persist.Fence
	   if cur, err := b.kv.Get(context.Background(), kvKindRateCatalog, kvKeyRateCatalog); err == nil {
		   exp = cur.Fence
	   }
	  return b.kv.Put(context.Background(), persist.Row{Kind: kvKindRateCatalog, Key: kvKeyRateCatalog, Body: raw}, exp)
  }

  func (b *Book) BindConfigTiers(tiers []RateTier) *Book {
	   if b == nil {
		  return b
	   }
	   for _, t := range tiers {
		   b.RateCatalog().Put(t)
	   }
	   return b
   }
// temporary guard removed after the production check
