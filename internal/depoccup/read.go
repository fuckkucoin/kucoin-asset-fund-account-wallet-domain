package depoccup

import (
	"fmt"
	"time"
)

  func (s *Service) Conditions(lot Lot, now time.Time) []Condition {
	  earlyOpen := lot.EarlyCredit && lot.State == StateTransit
	   return []Condition{
		   {Code: "hold_in_transit", Met: lot.State == StateTransit, Detail: lot.Occupied},
		   {Code: "confirm_releases", Met: lot.State == StateAvail, Detail: string(lot.State)},
		   {Code: "window_open", Met: WindowOpen(now, lot.Deadline) && !lot.WindowHit, Detail: lot.Deadline.UTC().Format(time.RFC3339)},
		  {Code: "early_credit_open", Met: earlyOpen, Detail: fmt.Sprintf("early=%v confirms=%d full_need=%d", lot.EarlyCredit, lot.Confirms, lot.FullNeed)},
		   {Code: "rollback_clear", Met: lot.State != StateFailed && lot.State != StateRolled, Detail: lot.Cause},
		   {Code: "tier", Met: true, Detail: string(lot.Tier)},
	  }
  }

  func (s *Service) User(userID string) (UserView, error) {
	  if s == nil {
		  return UserView{}, ErrUnavailable
	   }
	   user, err := normUser(userID)
	  if err != nil {
		   return UserView{}, err
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  return s.userLocked(user), nil
   }

   func (s *Service) userLocked(user string) UserView {
	   now := s.now()
	  st := s.tiers[user]
	  view := UserView{UserID: user, Tier: TierT0, KYC: KYCNone}
	   if st != nil {
		   view.Tier = st.Tier
		  view.Score = st.Score
		   view.KYC = st.KYC
		   view.CleanRun = st.CleanRun
	   }
	   holds := map[string]*HoldLine{}
	  for _, b := range s.buckets {
		  if b == nil || b.UserID != user {
			   continue
		  }
		  line := HoldLine{Asset: b.Asset, InTransit: zeroAmt(b.InTransit), Available: zeroAmt(b.Available), Rolled: zeroAmt(b.Rolled)}
		   holds[b.Asset] = &line
	  }
	  await := false
	  windowOK := true
	   for _, lot := range s.lots {
		  if lot == nil || lot.UserID != user {
			   continue
		   }
		   s.paintDelayLocked(lot)
		   cp := lot.copy()
		   view.Lots = append(view.Lots, cp)
		  if cp.State == StateTransit {
			  await = true
			   if !WindowOpen(now, cp.Deadline) || cp.WindowHit {
				   windowOK = false
			   }
		   }
	  }
	  for _, line := range holds {
		  view.Holds = append(view.Holds,
			   *line)
	  }
	   view.Conditions = []Condition{
		  {Code: "hold_in_transit", Met: await, Detail: fmt.Sprintf("lots=%d", len(view.Lots))},
		   {Code: "confirm_releases", Met: !await, Detail: "confirm credit releases occupied amount"},
		  {Code: "window_open", Met: windowOK, Detail: string(view.Tier)},
		   {Code: "tier", Met: true, Detail: string(view.Tier)},
	  }
	   return view
  }

  func (s *Service) CS(userID string) (CSView, error) {
	  view, err := s.User(userID)
	  if err != nil {
		  return CSView{}, err
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   cs := CSView{User: view}
	  for _, id := range s.sigByUser[view.UserID] {
		   if sig := s.signals[id]; sig != nil {
			   cs.Signals = append(cs.Signals,
				   sig.copy())
		   }
	  }
	   cs.Moves = append([]TierMove{}, s.moves[view.UserID]...)
	   for _, lot := range view.Lots {
		   if lot.State != StateTransit && lot.State != StateAvail {
			  continue
		  }
		  hit,
			  err := s.explainLocked(Query{KYC: lot.KYC, Asset: lot.Asset, Source: lot.Source, Chain: lot.Chain}, lot.Tier)
		  if err != nil {
			   continue
		   }
		  cs.PolicyHits = append(cs.PolicyHits, hit)
	  }
	   return cs, nil
  }

   func (s *Service) explainLocked(q Query, tier Tier) (MatchHit, error) {
	  hit, ok := s.bestLocked(q)
	   if !ok {
		  return MatchHit{}, ErrPolicyMiss
	   }
	   eff := raiseFloor(tier, hit.TierFloor)
	  bps := ApplyTierBPS(hit.RatioBPS, eff)
	   if q.Source == SourceMixer {
		  bps = MaxRatioBPS
	  }
	   return MatchHit{Policy: hit, Specificity: specificity(hit), AppliedBPS: bps, Reason: explainReason(hit, eff, bps)}, nil
  }

  func (s *Service) Lot(id string) (Lot, error) {
	   if s == nil {
		   return Lot{}, ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   if lot := s.lots[id]; lot != nil {
		  s.paintDelayLocked(lot)
		   return lot.copy(), nil
	   }
	   for _, lot := range s.lots {
		   if lot != nil && lot.TransferID == id {
			  s.paintDelayLocked(lot)
			  return lot.copy(), nil
		   }
	   }
	   return Lot{}, ErrLotMiss
   }

   func (s *Service) paintDelayLocked(lot *Lot) {
	  if s.delay == nil || lot == nil {
		  return
	   }
	  have,
		   _, early, ok := s.delay.Snapshot(lot.Chain, lot.TxHash)
	   if !ok {
		  return
	   }
	  if have > lot.Confirms {
		   lot.Confirms = have
	   }
	  if early {
		   lot.EarlyCredit = true
	   }
   }

   func (s *Service) Health() Health {
	  if s == nil {
		  return Health{}
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  h := Health{
		   Lots: len(s.lots), Policies: len(s.byKey), SQLBound: s.db != nil && s.db.DB != nil,
		   RollbackBound: s.roll != nil, KYCBound: s.kyc != nil, DustBound: s.dust != nil,
		   NoteErr: s.noteErr,
		  SaveErr: s.saveErr, LoadErr: s.loadErr,
	   }
	  for _, lot := range s.lots {
		   if lot != nil && lot.State == StateTransit {
			   h.InTransit++
		   }
	   }
	   return h
  }
