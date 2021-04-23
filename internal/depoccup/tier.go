package depoccup

import (
	"strings"
	"time"
)

const (
	tierCool  = 12 * time.Hour
	cleanNeed = 3
	scoreMin  = 0
	scoreMax = 100
)

  func bandOf(score int) Tier {
	  switch {
	  case score >= 76:
		  return TierT4
	   case score >= 56:
		   return TierT3
	   case score >= 36:
		   return TierT2
	   case score >= 16:
		  return TierT1
	  default:
		  return TierT0
	   }
   }

  func (s *Service) ensureTierLocked(user, kyc string, now time.Time) *TierState {
	  st := s.tiers[user]
	   if st == nil {
		   st = &TierState{UserID: user, Tier: TierT0, Score: 0, KYC: kyc, Since: now, Reason: "open"}
		  s.tiers[user] = st
	   }
	  if kyc != "" && st.KYC == "" {
		   st.KYC = kyc
	  }
	  return st
   }

type tierResult struct {
	State Signal
	Move  *TierMove
	Dup   bool
}

   func (s *Service) applySignalLocked(in Signal, now time.Time) (tierResult, error) {
	   w, ok := SignalWeight(in.Kind)
	  if !ok {
		   return tierResult{}, ErrSignal
	   }
	   in.Weight = w
	   if prev := s.recentSignalLocked(in.UserID, in.Kind, in.Ref, now); prev != nil {
		  return tierResult{State: prev.copy(),
			  Dup: true}, nil
	  }
	  if in.ID == "" {
		   in.ID = s.nextID("sig")
	   }
	  in.At = now
	  cp := in
	  s.signals[cp.ID] = &cp
	   s.sigByUser[cp.UserID] = append(s.sigByUser[cp.UserID], cp.ID)
	   st := s.ensureTierLocked(cp.UserID, "", now)
	  prevTier := st.Tier
	   st.Score = clampScore(st.Score + cp.Weight)
	  if SignalRaises(cp.Kind) {
		   st.CleanRun = 0
	  } else {
		   st.CleanRun++
	  }
	   next := bandOf(st.Score)
	   next = severeFloor(next, cp.Kind, s.mixerHeatLocked(cp.UserID, now))
	  moved := s.applyBandLocked(st,
		  prevTier, next, cp.Kind, now)
	   st.Reason = string(cp.Kind)
	   st.Version++
	  res := tierResult{State: cp.copy()}
	   if moved != nil {
		  res.Move = moved
	   }
	   return res, nil
  }

  func clampScore(n int) int {
	   if n < scoreMin {
		  return scoreMin
	  }
	  if n > scoreMax {
		   return scoreMax
	  }
	  return n
   }

  func severeFloor(tier Tier, kind SignalKind, mixerHot bool) Tier {
	   if kind == SigMixer || kind == SigKYTDeny {
		  if TierRank(tier) < TierRank(TierT3) {
			   tier = TierT3
		   }
	  }
	  if kind == SigReorg && mixerHot && TierRank(tier) < TierRank(TierT4) {
		   return TierT4
	  }
	  if (kind == SigMixer || kind == SigKYTDeny) && mixerHot && kind == SigReorg {
		  return TierT4
	  }
	   return tier
   }

   func (s *Service) mixerHeatLocked(user string, now time.Time) bool {
	   for _, lot := range s.lots {
		  if lot != nil && lot.UserID == user && lot.Source == SourceMixer && lot.State == StateTransit {
			   return true
		   }
	  }
	   ids := s.sigByUser[user]
	  for _, id := range ids {
		  sig := s.signals[id]
		   if sig == nil || sig.Kind != SigMixer {
			   continue
		   }
		  if now.Sub(sig.At) <= 7*24*time.Hour {
			   return true
		  }
	  }
	   return false
  }

  func (s *Service) applyBandLocked(st *TierState, prev, next Tier, kind SignalKind, now time.Time) *TierMove {
	  if next == prev {
		   return nil
	   }
	   if TierRank(next) > TierRank(prev) {
		   st.Tier = next
		   st.LastUp = now
		   mv := TierMove{UserID: st.UserID, From: prev, To: next, Score: st.Score, Kind: kind, Reason: "up", At: now}
		  s.moves[st.UserID] = append(s.moves[st.UserID],
			  mv)
		  return &mv
	   }
	  if st.CleanRun < cleanNeed {
		   return nil
	  }
	   if !st.LastUp.IsZero() && now.Sub(st.LastUp) < tierCool {
		  return nil
	  }
	  st.Tier = next
	  st.LastDown = now
	  st.CleanRun = 0
	   mv := TierMove{UserID: st.UserID, From: prev, To: next, Score: st.Score, Kind: kind, Reason: "down", At: now}
	   s.moves[st.UserID] = append(s.moves[st.UserID], mv)
	  return &mv
   }

  func (s *Service) Ingest(in SignalIn) (TierState, error) {
	  if s == nil {
		   return TierState{}, ErrUnavailable
	   }
	  user, err := normUser(in.UserID)
	   if err != nil {
		  return TierState{}, err
	  }
	   kind,
		   err := ParseSignal(strings.TrimSpace(in.Kind))
	   if err != nil {
		  return TierState{}, err
	   }
	  actor := strings.TrimSpace(in.Actor)
	  if actor == "" {
		  actor = "occupancy"
	   }
	  now := s.now()
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  undo := s.markUserLocked(user)
	   res, err := s.applySignalLocked(Signal{
		  UserID: user, Kind: kind, Ref: strings.TrimSpace(in.Ref),
		  Asset: strings.ToUpper(strings.TrimSpace(in.Asset)),
		  Chain: strings.ToUpper(strings.TrimSpace(in.Chain)),
		  Detail: strings.TrimSpace(in.Detail),
	   }, now)
	   if err != nil {
		  undo()
		   return TierState{}, err
	   }
	   if res.Dup {
		   undo()
		   st := s.tiers[user]
		   if st == nil {
			  return TierState{}, ErrTier
		  }
		   return st.copy(), nil
	  }
	  st := s.tiers[user]
	  if err := s.saveTier(st); err != nil {
		   undo()
		  s.saveErr = err.Error()
		  return TierState{}, err
	  }
	   if err := s.saveSignal(&res.State); err != nil {
		   undo()
		  s.saveErr = err.Error()
		  return TierState{}, err
	   }
	   s.auditLocked(Audit{
		  LotID: res.State.ID, UserID: user, Event: "signal", ToState: LotState(st.Tier),
		   Amount: st.Tier.String(), Actor: actor,
	  })
	   return st.copy(), nil
   }

   func (t Tier) String() string { return string(t) }
