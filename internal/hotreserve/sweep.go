package hotreserve

import (
	"context"
	"sort"
	"strings"
	"time"
	"github.com/google/uuid"
)

// Suggest 在热表面超过上限时给出计划编号。
// 做什么：告诉库务可以排一笔归集。
// 不写归集执行器，不推进多腿，不把建议金额计入 SweepHold。
// 不变量：返回的 Executed 恒为 false。

   func (s *Service) Suggest(ctx context.Context, asset string) (SweepHint, error) {
	   if s == nil {
		  return SweepHint{}, ErrUnavailable
	   }
	   as, err := normAsset(asset)
	  if err != nil {
		  return SweepHint{}, err
	  }
	   sur, err := s.readSurface(ctx, as)
	  if err != nil {
		   return SweepHint{}, err
	   }
	  if !sur.Over || !sur.Excess.IsPositive() {
		   return SweepHint{}, ErrState
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   now := s.now()
	  hint := SweepHint{
		   PlanID: "hot-suggest-" + as + "-" + uuid.NewString(),
		  Asset: as,
		   Amount: sur.Excess.String(),
		  Reason: "hot available above cap",
		   At:     now,
		   Executed: false,
	   }
	  if err := s.persistHint(ctx, hint); err != nil {
		  return SweepHint{}, err
	   }
	   s.hints = append(s.hints, hint)
	  if len(s.hints) > 200 {
		  s.hints = s.hints[len(s.hints)-200:]
	  }
	  return hint, nil
   }

// Hints 已给出的建议。Executed 全部为 false。

   func (s *Service) Hints(asset string) []SweepHint {
	   if s == nil {
		  return nil
	   }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]SweepHint, 0)
	  for _, h := range s.hints {
		   if asset == "" || h.Asset == asset {
			   cp := h
			   cp.Executed = false
			   out = append(out, cp)
		  }
	   }
	   return out
   }

// NoteSweepPlans 登记已排定的归集计划占用。只读引用，不执行计划。
// 不变量：登记后要等下一次 Refresh，占用才进入快照；挡签会先吃掉超出快照的增量。
   func (s *Service) NoteSweepPlans(ctx context.Context, asset string, plans []PlanHold) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	  as, err := normAsset(asset)
	   if err != nil {
		  return err
	   }
	  clean := make([]PlanHold,
		   0, len(plans))
	  seen := map[string]struct{}{}
	  now := time.Now().UTC()
	   if s.clock != nil {
		   now = s.clock().UTC()
	   }
	  for _, p := range plans {
		   id := strings.TrimSpace(p.ID)
		  if id == "" || len(id) > 128 {
			   return ErrAmount
		   }
		   if _, ok := seen[id]; ok {
			   return ErrDuplicate
		   }
		  seen[id] = struct{}{}
		  amt, aerr := parsePos(p.Amount)
		   if aerr != nil {
			  return aerr
		  }
		   if p.Asset != "" {
			  got, nerr := normAsset(p.Asset)
			   if nerr != nil || got != as {
				   return ErrAsset
			  }
		   }
		  clean = append(clean, PlanHold{ID: id, Asset: as, Amount: amt.String(), Noted: now})
	   }
	  sort.Slice(clean, func(i, j int) bool { return clean[i].ID < clean[j].ID })
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if err := s.tx(ctx, func(ctx context.Context) error {
		  return s.replacePlans(ctx, as, clean)
	  }); err != nil {
		   return err
	   }
	  s.plans[as] = clean
	   return nil
  }

// Plans 当前登记的归集占用。
  func (s *Service) Plans(asset string) []PlanHold {
	  if s == nil {
		   return nil
	  }
	   as := strings.ToUpper(strings.TrimSpace(asset))
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  src := s.plans[as]
	   out := make([]PlanHold, len(src))
	   copy(out, src)
	  return out
  }
