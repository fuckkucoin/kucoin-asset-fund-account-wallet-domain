package hotreserve

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Patrol 对一个资产做 reservation、冻结位、广播跟踪三段。
// 做什么：圈存合计必须等于冻结跟踪，兑付合计必须等于广播跟踪，分录回放必须对得上头寸。
// 外部观察一旦写入，就替代跟踪合计参与，便于发现「预留大于冻结」。
// 不变量：存在未关闭差异单时，该资产拒绝新预留与新签。一致后关闭差异单并解除阻挡。

  func (s *Service) Patrol(ctx context.Context, asset string) (AlignReport, error) {
	   if s == nil {
		  return AlignReport{}, ErrUnavailable
	   }
	   as, err := normAsset(asset)
	   if err != nil {
		  return AlignReport{}, err
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if err := s.guardLocked(); err != nil {
		   return AlignReport{}, err
	   }
	   return s.patrolLocked(ctx, as)
  }

// PatrolAll 巡检当前有预留、跟踪、观察或快照的全部资产。

   func (s *Service) PatrolAll(ctx context.Context) ([]AlignReport, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  if err := s.guardLocked(); err != nil {
		   return nil, err
	   }
	   seen := map[string]struct{}{}
	   for a := range s.forecast {
		  seen[a] = struct{}{}
	   }
	  for _, r := range s.reserves {
		  seen[r.Asset] = struct{}{}
	  }
	  for _, tr := range s.tracks {
		  seen[tr.Asset] = struct{}{}
	   }
	  for k := range s.observe {
		  seen[strings.SplitN(k, "|", 2)[0]] = struct{}{}
	  }
	  assets := make([]string, 0, len(seen))
	  for a := range seen {
		  assets = append(assets, a)
	  }
	   sort.Strings(assets)
	   out := make([]AlignReport, 0, len(assets))
	  for _, a := range assets {
		   rep,
			  err := s.patrolLocked(ctx, a)
		   if err != nil {
			   return nil, err
		   }
		  out = append(out, rep)
	  }
	  return out, nil
   }

  func (s *Service) patrolLocked(ctx context.Context, asset string) (AlignReport, error) {
	   open := s.sumState(asset, StateOpen)
	   ear := s.sumState(asset, StateEarmarked)
	   cons := s.sumState(asset, StateConsumed)
	  held := open.Add(ear)
	   freeze := s.alignFigure(asset, TrackFreeze)
	   bc := s.alignFigure(asset, TrackBroadcast)
	  kind := ""
	  note := ""
	   switch {
	   case !ear.Equal(freeze):
		   kind = "earmark_freeze"
		  note = "earmarked reservation does not match freeze track"
	   case !cons.Equal(bc):
		  kind = "consume_broadcast"
		   note = "consumed reservation does not match broadcast track"
	  default:
		  if k, n, bad := s.replayAsset(asset); bad {
			   kind, note = k, n
		   }
	  }
	   now := s.now()
	   rep := AlignReport{
		  ID: uuid.NewString(), Asset: asset, AsOf: now,
		  Held: held.String(), Earmarked: ear.String(), Consumed: cons.String(), Open: open.String(),
		  Freeze: freeze.String(), Broadcast: bc.String(),
		  Drift: kind != "", DriftKind: kind, Note: note,
		  EventID: "",
	   }
	   rep.Blocks = rep.Drift
	  if rep.Drift {
		   tk := s.openDriftLocked(asset, kind, held, freeze, bc, note, rep.ID, now)
		  rep.TicketID = tk.ID
	   } else {
		   s.closeDriftLocked(ctx, asset, now)
	  }
	   rep.EventID = "hr.align." + rep.ID
	   if err := s.persistAlign(ctx, rep); err != nil {
		   return AlignReport{}, err
	   }
	  if err := s.persistDriftState(ctx, asset); err != nil {
		  return AlignReport{}, err
	   }
	  s.reports = append(s.reports, rep)
	  if len(s.reports) > 200 {
		   s.reports = s.reports[len(s.reports)-200:]
	  }
	  noteDrift(asset, rep.Drift)
	   return rep, nil
   }

   func (s *Service) replayAsset(asset string) (string, string, bool) {
	  for _, r := range s.reserves {
		   if r.Asset != asset {
			   continue
		   }
		  st, err := Replay(r.Amount, s.journals[r.ID])
		  if err != nil || st != r.State {
			   return "journal_head", "reservation journal does not replay to head", true
		  }
		  if r.State == StateOpen && s.trackAmt(r.ID, TrackFreeze).IsPositive() {
			  return "open_frozen", "open reservation carries freeze track", true
		  }
		  if r.State == StateEarmarked && !s.trackAmt(r.ID, TrackFreeze).Equal(mustPos(r.Amount)) {
			   return "earmark_freeze", "earmarked row freeze track mismatch", true
		   }
		  if r.State == StateConsumed && !s.trackAmt(r.ID, TrackBroadcast).Equal(mustPos(r.Amount)) {
			  return "consume_broadcast", "consumed row broadcast track mismatch", true
		  }
		   if r.State == StateConsumed && s.trackAmt(r.ID, TrackFreeze).IsPositive() {
			  return "consume_broadcast", "consumed row still frozen", true
		  }
	  }
	   return "", "", false
   }

   func mustPos(s string) decimal.Decimal {
	  d, err := parsePos(s)
	  if err != nil {
		   return decimal.Zero
	  }
	   return d
  }

   func (s *Service) openDriftLocked(asset, kind string, held, freeze, bc decimal.Decimal, note, reportID string, now time.Time) *DriftTicket {
	  if id := s.driftOpen[asset]; id != "" {
		   tk := s.drifts[id]
		   if tk != nil {
			   tk.Kind = kind
			   tk.Held = held.String()
			   tk.Freeze = freeze.String()
			  tk.Broadcast = bc.String()
			  tk.Note = note
			  tk.ReportID = reportID
			  return tk
		   }
	  }
	  tk := &DriftTicket{
		  ID: uuid.NewString(), Asset: asset, Kind: kind, State: "open",
		  Held: held.String(), Freeze: freeze.String(), Broadcast: bc.String(),
		   Note: note, ReportID: reportID, OpenedAt: now,
	   }
	  s.drifts[tk.ID] = tk
	  s.driftOpen[asset] = tk.ID
	  return tk
  }

   func (s *Service) closeDriftLocked(ctx context.Context, asset string, now time.Time) {
	  id := s.driftOpen[asset]
	  if id == "" {
		  return
	   }
	   tk := s.drifts[id]
	  if tk != nil && tk.State == "open" {
		   tk.State = "closed"
		  tk.ClosedAt = now
		   _ = ctx
	  }
	   delete(s.driftOpen, asset)
  }

// Reports 最近一致报告。
  func (s *Service) Reports(asset string) []AlignReport {
	   if s == nil {
		   return nil
	  }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]AlignReport, 0)
	   for _, r := range s.reports {
		   if asset == "" || r.Asset == asset {
			   out = append(out, r)
		  }
	  }
	  return out
  }

// OpenDrifts 未关闭的差异单。
   func (s *Service) OpenDrifts() []DriftTicket {
	   if s == nil {
		   return nil
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]DriftTicket, 0)
	  for _, id := range s.driftOpen {
		  if tk := s.drifts[id]; tk != nil {
			   out = append(out, *tk)
		   }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].Asset < out[j].Asset })
	  return out
   }

// Verify 回放单笔预留，供兑付后确认没有双计。

   func (s *Service) Verify(id string) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   r := s.reserves[strings.TrimSpace(id)]
	  if r == nil {
		   return ErrNotFound
	  }
	   st, err := Replay(r.Amount, s.journals[r.ID])
	  if err != nil {
		  return err
	   }
	   if st != r.State {
		   return fmt.Errorf("%w: replay %s head %s", ErrState, st, r.State)
	  }
	  return nil
   }
