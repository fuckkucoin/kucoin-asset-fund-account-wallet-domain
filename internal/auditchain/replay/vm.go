package replay

import (
	"sort"
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

// Run 执行一条轨迹。效果位未知则整单失败，不产出半截报告当成功。

   func Run(in Input) (Report, error) {
	  rep := Report{
		  UserID: strings.TrimSpace(in.UserID), Asset: strings.ToUpper(strings.TrimSpace(in.Asset)),
		   From: in.From, To: in.To, Verdict: "replay-only",
	   }
	  evs := append([]Event(nil), in.Events...)
	  sort.SliceStable(evs, func(i, j int) bool {
		   if !evs[i].OccurredAt.Equal(evs[j].OccurredAt) {
			   return evs[i].OccurredAt.Before(evs[j].OccurredAt)
		  }
		   return evs[i].Seq < evs[j].Seq
	   })
	   avail := decimal.Zero
	  frozen := decimal.Zero
	   seen := map[string]struct{}{}
	  short := false
	   for _, ev := range evs {
		  if !Known(ev.Effect) {
			   return Report{}, errEffect(ev.Effect)
		  }
		  if ev.IdemKey != "" {
			   seen[ev.IdemKey] = struct{}{}
		   }
		   step, nextA, nextF, err := apply(avail, frozen, ev)
		   if err != nil {
			  return Report{}, err
		  }
		   if step.Note == "short" {
			  short = true
		   }
		   avail, frozen = nextA, nextF
		  step.Available = avail.String()
		  step.Frozen = frozen.String()
		   step.Ord = len(rep.Steps) + 1
		   rep.Steps = append(rep.Steps, step)
	  }
	   rep.ReplayAvailable = avail.String()
	  rep.ReplayFrozen = frozen.String()
	   for _, id := range in.Expect {
		  id = strings.TrimSpace(id)
		   if id == "" {
			   continue
		  }
		   if _, ok := seen[id]; !ok {
			   rep.Missing = append(rep.Missing, id)
		   }
	   }
	  if in.Snap == nil {
		  if len(rep.Missing) > 0 || short {
			  rep.Verdict = "diff"
			  rep.CaseOpened = true
			  rep.Note = "trail incomplete"
		  }
		   return rep, nil
	  }
	   bookA, err := decimal.NewFromString(zero(in.Snap.Available))
	   if err != nil {
		   return Report{}, err
	  }
	  bookF, err := decimal.NewFromString(zero(in.Snap.Frozen))
	  if err != nil {
		  return Report{}, err
	   }
	   rep.BookAvailable = bookA.String()
	   rep.BookFrozen = bookF.String()
	  rep.DiffAvailable = avail.Sub(bookA).String()
	   rep.DiffFrozen = frozen.Sub(bookF).String()
	  if len(rep.Missing) > 0 || short || !avail.Equal(bookA) || !frozen.Equal(bookF) {
		   rep.Verdict = "diff"
		   rep.CaseOpened = true
		  rep.Note = "replay book mismatch"
		   return rep, nil
	  }
	  rep.Verdict = "match"
	   rep.Note = "replay matches book"
	   return rep, nil
  }

  func zero(s string) string {
	   s = strings.TrimSpace(s)
	   if s == "" {
		  return "0"
	  }
	   return s
   }

  func apply(avail, frozen decimal.Decimal, ev Event) (Step, decimal.Decimal, decimal.Decimal, error) {
	   step := Step{Kind: ev.Kind, RefID: ev.RefID, IdemKey: ev.IdemKey, Amount: zero(ev.Amount), Effect: ev.Effect, At: ev.OccurredAt}
	  amt, err := decimal.NewFromString(step.Amount)
	  if err != nil {
		   return Step{}, avail, frozen, err
	  }
	  if amt.IsNegative() {
		   return Step{}, avail, frozen, errEffect("negative")
	   }
	  if ev.Effect == EffNone || amt.IsZero() {
		  if ev.Effect != EffNone && amt.IsZero() {
			  step.Note = "marker"
		  }
		  return step, avail, frozen, nil
	  }
	  switch ev.Effect {
	  case EffCredit, EffIn:
		   return step,
			   avail.Add(amt), frozen, nil
	  case EffDebit, EffOut:
		   if avail.Cmp(amt) < 0 {
			  step.Note = "short"
			   return step, avail, frozen, nil
		  }
		  return step, avail.Sub(amt), frozen, nil
	  case EffFreeze:
		   if avail.Cmp(amt) < 0 {
			  step.Note = "short"
			  return step, avail, frozen, nil
		   }
		   return step, avail.Sub(amt), frozen.Add(amt), nil
	  case EffUnfreeze:
		   moved := amt
		   if frozen.Cmp(amt) < 0 {
			   moved = frozen
			   step.Note = "clamped"
		   }
		  return step,
			  avail.Add(moved), frozen.Sub(moved), nil
	  default:
		   return Step{},
			  avail, frozen, errEffect(ev.Effect)
	   }
  }

type effectErr string

  func errEffect(s string) error { return effectErr(s) }

   func (e effectErr) Error() string { return "audit replay effect " + string(e) }

// Window 丢掉窗外事件。含端点。

   func Window(evs []Event, from, to time.Time) []Event {
	   if from.IsZero() && to.IsZero() {
		   return append([]Event(nil),
			   evs...)
	  }
	  out := make([]Event, 0, len(evs))
	  for _, ev := range evs {
		  if !from.IsZero() && ev.OccurredAt.Before(from) {
			  continue
		  }
		  if !to.IsZero() && ev.OccurredAt.After(to) {
			   continue
		  }
		  out = append(out, ev)
	  }
	   return out
  }

// SameSubject 只留同一用户同一资产。空用户的系统事实不进个人轨迹。

   func SameSubject(evs []Event,
	   user, asset string) []Event {
	  user = strings.TrimSpace(user)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   out := make([]Event, 0, len(evs))
	   for _, ev := range evs {
		  if ev.UserID != user {
			  continue
		   }
		  if asset != "" && strings.ToUpper(ev.Asset) != asset {
			   continue
		  }
		   out = append(out, ev)
	  }
	  return out
   }
