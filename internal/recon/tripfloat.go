package recon

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

const (
	tripTransitAged = "transit_aged"
	// transitAgedAfter 广播后仍未完结的时限。未到时限只解释缺口，到时限另开差异，仍不解冻。
	transitAgedAfter = 6 * time.Hour
	transitScanLimit = 500
)

type transitBook struct {
	amt  map[string]decimal.Decimal
	aged map[string][]TripDrift
}

   func (t transitBook) amount(asset string) decimal.Decimal {
	  if t.amt == nil {
		  return decimal.Zero
	   }
	   return t.amt[strings.ToUpper(strings.TrimSpace(asset))]
  }

// loadTransit 读在途出金。状态机不在这里推进。
   func loadTransit(ctx context.Context, st ledger.Store, now time.Time) (transitBook, error) {
	  out := transitBook{
		  amt:  map[string]decimal.Decimal{},
		   aged: map[string][]TripDrift{},
	  }
	   if st == nil {
		  return out, nil
	  }
	  if now.IsZero() {
		  now = time.Now().UTC()
	  }
	  states := []ledger.State{
		  ledger.StateWalletLocked,
		  ledger.StateChainSubmitted,
		  ledger.StateConfirmed,
		   ledger.StateHold,
		  ledger.StateFailed,
	   }
	   for _, stt := range states {
		   list, err := st.ListByState(ctx, stt, transitScanLimit)
		  if err != nil {
			   return transitBook{}, err
		   }
		  for _, tr := range list {
			  if !withdrawInFlight(tr) {
				   continue
			  }
			  asset := strings.ToUpper(strings.TrimSpace(tr.Asset))
			   out.amt[asset] = out.amt[asset].Add(tr.Amount)
			   at := tr.UpdatedAt
			  if at.IsZero() {
				  at = tr.CreatedAt
			  }
			   if at.IsZero() || now.Sub(at) < transitAgedAfter {
				   continue
			   }
			  out.aged[asset] = append(out.aged[asset], TripDrift{
				  Kind:  tripTransitAged,
				   Asset: asset,
				  UserID: tr.UserID,
				  Book: string(ledger.AccountWallet),
				  Note: fmt.Sprintf("triparty:transit_aged id=%s amount=%s since=%s",
					  tr.ID, tr.Amount.String(), at.UTC().Format(time.RFC3339)),
			   })
		   }
	  }
	  return out, nil
  }

// withdrawInFlight 已广播且未完结的钱包出金。没有链上 hash 的冻结仍在热侧，不能拿来填用户合计缺口。
  func withdrawInFlight(tr *ledger.Transfer) bool {
	  return transitEligible(tr)
   }
