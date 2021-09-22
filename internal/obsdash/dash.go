package obsdash

import (
	"context"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Snapshot struct {
	Disclaimer string  `json:"disclaimer"`
	Counters   []Point `json:"counters"`
	Burns       []BurnAlert `json:"burns"`
	OpenFreezes int         `json:"open_freezes"`
	BridgeUSDT  string      `json:"bridge_usdt"`
	UserSpotUSDT string   `json:"user_spot_usdt"`
	Alerts       []string `json:"alerts"`
}

  func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	   if s == nil {
		  return Snapshot{}, ErrUnavailable
	  }
	  out := Snapshot{
		  Disclaimer: "in-process fund-safety aggregation; not a public reserve attestation",
		   Counters: s.ctr.Snapshot(),
		  Burns: s.burn.List(),
	   }
	   for _, b := range out.Burns {
		  if b.Alert {
			  out.Alerts = append(out.Alerts, "slo:"+b.Name)
		  }
	  }
	  if s.led != nil && s.store != nil {
		   tr, err := s.led.List(ctx, "", 200)
		  if err == nil {
			  for _, row := range tr {
				  if row != nil && row.State.Open() && row.Direction.Freezes() {
					   out.OpenFreezes++
				  }
			   }
		   }
		   if acc, err := s.store.GetAccount(ctx, "system", ledger.AccountBridge, "USDT"); err == nil && acc != nil {
			  out.BridgeUSDT = acc.Available.String()
		  }
		   if s.user != "" {
			   if acc, err := s.store.GetAccount(ctx, s.user, ledger.AccountSpot, "USDT"); err == nil && acc != nil {
				  out.UserSpotUSDT = acc.Available.String()
			  }
		  }
	  }
	  if out.BridgeUSDT != "" && out.UserSpotUSDT != "" {
		  br, err1 := money.ParseBook(out.BridgeUSDT)
		  us, err2 := money.ParseBook(out.UserSpotUSDT)
		  if err1 == nil && err2 == nil && br.IsZero() && us.IsPositive() {
			  out.Alerts = append(out.Alerts, "bridge_empty_with_user_float")
		   }
	   }
	  return out, nil
   }
