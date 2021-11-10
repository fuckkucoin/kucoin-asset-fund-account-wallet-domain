package regioinv

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Aggregate 把本区的全部观测汇聚成一条 RegionInv 并写入存储。
// 同一 source 取最新一条；不同 source 的可用与冻结叠加。
   func Aggregate(ctx context.Context, st Store, regionID, tier, asset, chain string, now time.Time) (RegionInv, error) {
	  obsList, err := st.ListObs(ctx, regionID, tier, asset, chain)
	  if err != nil {
		   return RegionInv{}, err
	  }
	  if len(obsList) == 0 {
		  return RegionInv{}, ErrNotFound
	  }
	  // 按 source 去重取最新
	   bySource := map[string]Observation{}
	  for _, o := range obsList {
		  prev, ok := bySource[o.Source]
		  if !ok || o.Seq > prev.Seq {
			   bySource[o.Source] = o
		  }
	  }
	  totAvail := money.Zero()
	   totFroz := money.Zero()
	   for _, o := range bySource {
		  a, err := money.ParseBook(o.Available)
		  if err != nil {
			   return RegionInv{}, err
		   }
		   f, err := money.ParseBook(o.Frozen)
		  if err != nil {
			   return RegionInv{}, err
		   }
		   totAvail, err = totAvail.Add(a)
		   if err != nil {
			  return RegionInv{}, err
		   }
		   totFroz, err = totFroz.Add(f)
		  if err != nil {
			  return RegionInv{}, err
		   }
	   }
	  obs, err := totAvail.Add(totFroz)
	  if err != nil {
		   return RegionInv{}, err
	  }
	  // 读已有行或新建
	   row,
		   err := st.FindInv(ctx, regionID, tier, asset, chain)
	   if err != nil {
		   row = RegionInv{
			  ID: newID("ri"),
			  RegionID: regionID,
			  Tier:  tier,
			  Asset: strings.ToUpper(asset),
			   Chain: strings.ToLower(chain),
		   }
	   }
	   row.Available = totAvail.String()
	   row.Frozen = totFroz.String()
	  row.Observed = obs.String()
	  row.Seq++
	  row.ObservedAt = now
	   row.UpdatedAt = now
	  if err := st.SaveInv(ctx, row); err != nil {
		   return RegionInv{}, err
	   }
	   return row, nil
  }

// AggregateAll 汇聚某区全部已有观测条目（按独有 tier/asset/chain 组合）。
  func AggregateAll(ctx context.Context, st Store, regionID string, now time.Time) ([]RegionInv, error) {
	  obsList, err := st.ListObs(ctx, regionID, "", "", "")
	   if err != nil {
		  return nil, err
	   }
	   type key struct{ tier, asset, chain string }
	  seen := map[key]bool{}
	  for _, o := range obsList {
		  seen[key{o.Tier, strings.ToUpper(o.Asset), strings.ToLower(o.Chain)}] = true
	   }
	  var out []RegionInv
	   for k := range seen {
		   row, err := Aggregate(ctx, st, regionID, k.tier, k.asset, k.chain, now)
		  if err != nil {
			  return nil, err
		   }
		   out = append(out, row)
	   }
	   return out, nil
  }
