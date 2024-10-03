package solvency

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// ObserveLedger 汇总 system 的 bridge（热）、warm（温）、cold（冷）。
// 冻结余额仍属于托管持仓，计入该层合计。用户科目不进观察。

  func ObserveLedger(ctx context.Context, books Books) ([]ReserveLeg, error) {
	  if books == nil {
		   return nil, ErrUnavailable
	   }
	   accs, err := books.ListUserBooks(ctx, "system")
	   if err != nil {
		  return nil, err
	  }
	  type acc struct{ hot, warm, cold string }
	   by := map[string]*acc{}
	   for _, a := range accs {
		  asset := strings.ToUpper(strings.TrimSpace(a.Asset))
		   if asset == "" {
			   continue
		  }
		   held, err := addAmt(a.Available.String(), a.Frozen.String())
		  if err != nil {
			  return nil, err
		   }
		  row := by[asset]
		  if row == nil {
			  row = &acc{hot: "0", warm: "0", cold: "0"}
			  by[asset] = row
		   }
		   switch a.AccountType {
		  case ledger.AccountBridge:
			   row.hot, err = addAmt(row.hot, held)
		   case ledger.AccountWarm:
			  row.warm, err = addAmt(row.warm, held)
		   case ledger.AccountCold:
			   row.cold, err = addAmt(row.cold, held)
		   default:
			  continue
		  }
		  if err != nil {
			  return nil, err
		   }
	  }
	   out := make([]ReserveLeg, 0, len(by))
	   for asset, row := range by {
		  out = append(out, ReserveLeg{
			   Asset: asset,
			   Hot:   row.hot, Warm: row.warm, Cold: row.cold, Source: SourceCustody,
		  })
	   }
	  sortLegs(out)
	   return out, nil
  }

   func sortLegs(in []ReserveLeg) {
	  for i := 1; i < len(in); i++ {
		   j := i
		  for j > 0 && in[j].Asset < in[j-1].Asset {
			   in[j], in[j-1] = in[j-1], in[j]
			  j--
		  }
	   }
  }

  func (p *Pipeline) observe(ctx context.Context) ([]ReserveLeg, string, error) {
	   p.mu.Lock()
	   on := p.overrideOn
	   ov := append([]ReserveLeg(nil), p.override...)
	   p.mu.Unlock()
	   if on {
		  for i := range ov {
			  if ov[i].Source == "" {
				  ov[i].Source = SourceOverride
			  }
			   ov[i].Asset = strings.ToUpper(strings.TrimSpace(ov[i].Asset))
			   var err error
			   if ov[i].Hot, err = normAmt(ov[i].Hot); err != nil {
				   return nil, "", err
			  }
			  if ov[i].Warm, err = normAmt(ov[i].Warm); err != nil {
				   return nil, "", err
			   }
			  if ov[i].Cold, err = normAmt(ov[i].Cold); err != nil {
				  return nil, "", err
			   }
		  }
		  sortLegs(ov)
		  return ov, SourceOverride, nil
	  }
	   legs, err := ObserveLedger(ctx, p.books)
	  if err != nil {
		   return nil, SourceUnreadable, err
	  }
	  return legs, SourceCustody, nil
   }

// SetReserveOverride 用调用方给出的观察替换托管读数，账面本身不改。
// 仅内存剖面允许。剖面拒绝，避免把人为压低的观察写成可发布覆盖结论。

  func (p *Pipeline) SetReserveOverride(legs []ReserveLeg) error {
	   if p == nil {
		   return ErrUnavailable
	  }
	   if legs != nil && !p.allowOverride {
		  return ErrOverride
	  }
	  p.mu.Lock()
	   defer p.mu.Unlock()
	   if legs == nil {
		  p.overrideOn = false
		  p.override = nil
		  return nil
	  }
	  p.overrideOn = true
	   p.override = append([]ReserveLeg(nil), legs...)
	   return nil
   }
