package invcheck

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/custody"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

type pair struct {
	hot, cold, frozen money.Amount
}

// Run 读账本 system.bridge/cold、托管库存、热账观察。任一侧金额对不上失败即关。
   func (s *Service) Run(ctx context.Context, actor string) (Report, error) {
	  if s == nil {
		   return Report{}, ErrUnavailable
	   }
	  act := ""
	  if strings.TrimSpace(actor) != "" {
		   var err error
		   act, err = requireActor(actor)
		  if err != nil {
			   return Report{}, err
		   }
	  }
	   if s.store == nil {
		  return Report{}, ErrStore
	   }
	   led, err := loadLedger(ctx, s.store)
	   if err != nil {
		   return Report{}, err
	   }
	   custRows := []custody.InventoryRow{}
	  if s.cust != nil {
		   custRows, err = s.cust.Inventory(ctx)
		  if err != nil {
			  return Report{}, err
		   }
	   }
	  invRows := []hotinv.Snapshot{}
	   book := s.inv
	   if book == nil {
		   book = hotinv.FromHotMax(map[string]string{"USDT": "500000", "BTC": "80", "ETH": "800"})
	   }
	  invRows, err = book.Observe(ctx, s.store)
	   if err != nil {
		   return Report{}, err
	  }
	   plans := 0
	   if s.tre != nil {
		   ps, perr := s.tre.Plan(ctx, s.store)
		   if perr != nil {
			  return Report{}, perr
		   }
		  if err := CheckPlans(ps, led); err != nil {
			   return Report{}, err
		  }
		   plans = len(ps)
	   }
	  rep,
		  err := Align(led, custRows, invRows)
	  rep.Actor = act
	   rep.PlanCount = plans
	   rep.At = s.now()
	   if rep.ID != "" {
		  s.mu.Lock()
		  cp := rep
		   s.reps[cp.ID] = &cp
		   s.mu.Unlock()
	  }
	   if err != nil {
		  return rep.Copy(), err
	  }
	   return rep.Copy(), nil
   }

   func loadLedger(ctx context.Context, st ledger.Store) (map[string]pair, error) {
	   accs, err := st.ListUserBooks(ctx, "system")
	  if err != nil {
		  return nil, err
	   }
	   out := map[string]pair{}
	  for _, a := range accs {
		   as,
			   err := validate.Asset(a.Asset)
		  if err != nil {
			  return nil, ErrAsset
		  }
		  av,
			   err := money.ParseBook(a.Available.String())
		   if err != nil {
			   return nil, ErrAmount
		  }
		   fr, err := money.ParseBook(a.Frozen.String())
		   if err != nil {
			  return nil, ErrAmount
		   }
		  cur := out[as]
		   switch a.AccountType {
		  case ledger.AccountBridge:
			  cur.hot = av
			   cur.frozen = fr
		   case ledger.AccountCold:
			  cur.cold = av
		  default:
			   continue
		  }
		  out[as] = cur
	   }
	  return out, nil
  }

   func Align(led map[string]pair, cust []custody.InventoryRow, inv []hotinv.Snapshot) (Report, error) {
	  custBy := map[string]pair{}
	  for _, r := range cust {
		   as, err := validate.Asset(r.Asset)
		   if err != nil {
			   return Report{}, ErrAsset
		   }
		  av, err := parseAmt(r.Available)
		  if err != nil {
			  return Report{}, err
		  }
		   fr, err := parseAmt(r.Frozen)
		   if err != nil {
			   return Report{}, err
		   }
		  cur := custBy[as]
		  switch r.Side {
		   case "hot":
			  cur.hot = av
			  cur.frozen = fr
		  case "cold":
			  cur.cold = av
		  }
		  custBy[as] = cur
	  }
	   invBy := map[string]pair{}
	   for _, r := range inv {
		  as, err := validate.Asset(r.Asset)
		  if err != nil {
			  return Report{}, ErrAsset
		   }
		  h, err := parseAmt(r.HotAvail)
		  if err != nil {
			  return Report{}, err
		  }
		   c, err := parseAmt(r.ColdAvail)
		   if err != nil {
			   return Report{}, err
		   }
		  fr, err := parseAmt(r.HotFrozen)
		  if err != nil {
			   return Report{}, err
		  }
		  invBy[as] = pair{hot: h, cold: c, frozen: fr}
	   }
	  assets := map[string]struct{}{}
	   for k := range led {
		   assets[k] = struct{}{}
	   }
	   for k := range custBy {
		  assets[k] = struct{}{}
	   }
	  for k := range invBy {
		  assets[k] = struct{}{}
	  }
	  lines := make([]Line, 0, len(assets))
	  ok := true
	  reason := ""
	   for as := range assets {
		  l := led[as]
		  c := custBy[as]
		   i := invBy[as]
		  line := Line{
			   Asset: as, LedgerHot: l.hot.String(), LedgerCold: l.cold.String(),
			   CustHot: c.hot.String(), CustCold: c.cold.String(),
			   InvHot: i.hot.String(), InvCold: i.cold.String(), Matched: true,
		   }
		   if len(cust) > 0 {
			   if !l.hot.Equal(c.hot) || !l.cold.Equal(c.cold) {
				  line.Matched = false
				  line.Reason = "custody"
				  ok = false
				   if reason == "" {
					  reason = "custody"
				  }
			   }
			   sumL, err := l.hot.Add(l.cold)
			   if err != nil {
				  return Report{}, err
			  }
			  sumC, err := c.hot.Add(c.cold)
			  if err != nil {
				   return Report{}, err
			   }
			   if err := money.Conserve(sumL, sumC); err != nil && reason == "" {
				  line.Matched = false
				  line.Reason = "conserve"
				  ok = false
				   reason = "conserve"
			   }
		   }
		  if len(inv) > 0 {
			   if !l.hot.Equal(i.hot) || !l.cold.Equal(i.cold) {
				   line.Matched = false
				  line.Reason = "hotinv"
				  ok = false
				  if reason == "" {
					  reason = "hotinv"
				  }
			  }
		  }
		  lines = append(lines, line)
	   }
	  rep := Report{
		   ID: uuid.NewString(), Lines: lines, Balanced: ok, Disclaimer: disclaimer,
	   }
	   if !ok {
		  if reason == "conserve" {
			   return rep, ErrImbalance
		   }
		   if reason == "" {
			  return rep, ErrMismatch
		   }
		  return rep, ErrMismatch
	   }
	   return rep, nil
  }

  func orZ(s string) string {
	  if strings.TrimSpace(s) == "" {
		  return "0"
	  }
	  return s
   }

  func parseAmt(s string) (money.Amount, error) {
	   s = orZ(s)
	   for _, r := range s {
		  if r == 'e' || r == 'E' {
			  return money.Zero(), ErrAmount
		  }
	   }
	   got, err := money.ParseBook(s)
	   if err != nil {
		  return money.Zero(), ErrAmount
	  }
	  return got, nil
   }
