package recon

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/metrics"
	"github.com/shopspring/decimal"
)

const (
	tripBridgeInflate = "bridge_inflate"
	tripHotBelowUser  = "hot_below_user"
	tripUnexplained   = "unexplained_book"
	tripUnread = "hot_unreadable"
)

// TripDrift 单资产上的一条差异。
type TripDrift struct {
	Kind string `json:"kind"`
	Asset  string `json:"asset"`
	UserID string `json:"user_id,omitempty"`
	Book   string `json:"book,omitempty"`
	Note   string `json:"note"`
	Class string `json:"class,omitempty"`
	Severity string `json:"severity,omitempty"`
	Tolerance string `json:"tolerance,omitempty"`
}

// TripLeg 一个资产的三方取值，开单和读面都用这些字段。

type TripLeg struct {
	Asset        string `json:"asset"`
	HotObserved  string `json:"hot_observed"`
	WarmObserved string `json:"warm_observed"`
	ColdObserved string `json:"cold_observed"`
	Bridge       string `json:"bridge"`
	WarmBook string `json:"warm_book"`
	ColdBook string `json:"cold_book"`
	UserTotal string `json:"user_total"`
	InTransit string `json:"in_transit"`
	Inbound string `json:"inbound"`
	BridgeJournal string      `json:"bridge_journal"`
	OK            bool        `json:"ok"`
	Drifts        []TripDrift `json:"drifts,omitempty"`
}

// TripReport 一轮三方对账结果。

type TripReport struct {
	OK bool `json:"ok"`
	Blocked bool `json:"blocked"`
	Legs    []TripLeg   `json:"legs"`
	Drifts  []TripDrift `json:"drifts,omitempty"`
	Tickets []Ticket    `json:"tickets,omitempty"`
	At      time.Time   `json:"at"`
}

type tripHold struct {
	mu    sync.Mutex
	users map[string]struct{}
	last  TripReport
}

var tripState tripHold

// LastTriparty 最近一轮结果。尚未跑过时 OK 为空报告。
   func LastTriparty() TripReport {
	  tripState.mu.Lock()
	   defer tripState.mu.Unlock()
	   return tripState.last
  }

// RunTriparty 执行三方对账。
// observe 非空时按资产覆盖热/温/冷观察；空则只读 hotinv.Observe。
// 任一差异写工单、记平台挡板，并按用户 NoteReconcile，使新的冻结失败即关。

   func RunTriparty(ctx context.Context, led *ledger.Service, st ledger.Store, tickets *Book, actor string, hot *hotinv.Book, observe []hotinv.Snapshot) (TripReport, error) {
	   rep := TripReport{OK: true,
		   At: time.Now().UTC()}
	  if st == nil || led == nil {
		   return rep, ErrStore
	   }
	  if ctx == nil {
		   ctx = context.Background()
	   }
	  if strings.TrimSpace(actor) == "" {
		  actor = "triparty"
	   }
	   if err := actorOK(actor); err != nil {
		  return rep, err
	  }
	   if hot == nil {
		   hot = hotinv.New(nil)
	  }
	  snaps, err := hot.Observe(ctx, st)
	  if err != nil {
		  d := TripDrift{Kind: tripUnread, Asset: "*", UserID: "system", Book: "bridge", Note: "triparty:hot_unreadable " + err.Error()}
		   rep.OK = false
		   rep.Drifts = []TripDrift{d}
		  rep.Blocked = true
		  decorateDrifts(&rep)
		  led.SetPlatformDrift(true, blockSummary(rep.Drifts))
		  rep.Tickets = openTrip(tickets, actor, rep.Drifts)
		  storeTrip(rep, nil)
		  metrics.TripartyRuns.WithLabelValues("unread").Inc()
		  return rep, err
	   }
	  over := indexObserve(observe)
	  accs,
		  err := st.ListAccounts(ctx, 5000)
	  if err != nil {
		  return rep, err
	   }
	   type bucket struct {
		  bridge, warm, cold ledger.Account
		   hasB, hasW, hasC   bool
		  users              decimal.Decimal
		   holders map[string]struct{}
	   }
	  by := map[string]*bucket{}
	  touch := func(asset string) *bucket {
		  asset = strings.ToUpper(strings.TrimSpace(asset))
		   b := by[asset]
		   if b == nil {
			  b = &bucket{holders: map[string]struct{}{}}
			   by[asset] = b
		   }
		  return b
	   }
	  for _, a := range accs {
		  asset := strings.ToUpper(strings.TrimSpace(a.Asset))
		  if asset == "" {
			  continue
		  }
		  b := touch(asset)
		  total := a.Available.Add(a.Frozen)
		   if a.UserID == "system" {
			  switch a.AccountType {
			   case ledger.AccountBridge:
				  b.bridge, b.hasB = a, true
			  case ledger.AccountWarm:
				  b.warm, b.hasW = a, true
			  case ledger.AccountCold:
				   b.cold, b.hasC = a, true
			  }
			  continue
		   }
		  if !a.AccountType.UserFacing() {
			   continue
		  }
		   if total.IsPositive() {
			   b.users = b.users.Add(total)
			   b.holders[a.UserID] = struct{}{}
		  }
	   }
	   for _, s := range snaps {
		   touch(s.Asset)
	  }
	  for asset := range over {
		   touch(asset)
	   }
	   pipe, err := loadTransit(ctx, st, rep.At)
	  if err != nil {
		   return rep, err
	  }
	   for asset := range pipe.amt {
		   touch(asset)
	   }
	   for asset := range pipe.aged {
		   touch(asset)
	   }
	   inbound, err := loadInbound(ctx, st)
	  if err != nil {
		   return rep, err
	  }
	   for asset := range inbound {
		  touch(asset)
	  }
	  var drifts []TripDrift
	   var blockedUsers []string
	   for asset, b := range by {
		  leg := TripLeg{Asset: asset, OK: true}
		   hotObs, warmObs, coldObs, perr := obsOf(asset, snaps, over)
		   if perr != nil {
			   d := TripDrift{Kind: tripUnread, Asset: asset, UserID: "system", Book: "bridge", Note: "triparty:hot_unreadable " + perr.Error()}
			  leg.OK = false
			   leg.Drifts = append(leg.Drifts, d)
			  drifts = append(drifts, d)
			  rep.Legs = append(rep.Legs, leg)
			   continue
		  }
		  bridge := decimal.Zero
		  warmBook := decimal.Zero
		  coldBook := decimal.Zero
		   if b.hasB {
			   bridge = b.bridge.Available.Add(b.bridge.Frozen)
		  }
		  if b.hasW {
			  warmBook = b.warm.Available.Add(b.warm.Frozen)
		  }
		   if b.hasC {
			   coldBook = b.cold.Available.Add(b.cold.Frozen)
		   }
		   leg.HotObserved = hotObs.String()
		  leg.WarmObserved = warmObs.String()
		   leg.ColdObserved = coldObs.String()
		  leg.Bridge = bridge.String()
		   leg.WarmBook = warmBook.String()
		  leg.ColdBook = coldBook.String()
		  inT := pipe.amount(asset)
		  leg.UserTotal = b.users.String()
		  leg.InTransit = inT.String()
		  inB := inbound[asset]
		   leg.Inbound = inB.String()
		   custody := hotObs.Add(warmObs).Add(coldObs).Add(inT)
		  // 桥账面高于热观察，或桥账面无法被流水解释：桥膨胀。
		  bridgeGap, bridgeBad, gerr := ledger.BookGap(ctx, st, "system", ledger.AccountBridge, asset, bridge)
		  if gerr != nil {
			   return rep, gerr
		   }
		   leg.BridgeJournal = bridgeGap.String()
		  if materialAbove(asset, bridge, hotObs) || bridgeBad {
			   note := "triparty:bridge_inflate hot=" + hotObs.String() + " bridge=" + bridge.String() + " journal=" + bridgeGap.String() + " user=" + b.users.String()
			  d := TripDrift{Kind: tripBridgeInflate, Asset: asset, UserID: "system", Book: string(ledger.AccountBridge), Note: note}
			  leg.Drifts = append(leg.Drifts, d)
			   drifts = append(drifts, d)
			   leg.OK = false
		   }
		   // 热+温+冷观察加已广播在途，仍低于用户合计。未广播的冻结不在在途里。
		  if materialAbove(asset, b.users, custody) {
			  note := "triparty:hot_below_user hot=" + hotObs.String() + " warm=" + warmObs.String() + " cold=" + coldObs.String() + " transit=" + inT.String() + " user=" + b.users.String()
			  d := TripDrift{Kind: tripHotBelowUser, Asset: asset, UserID: "system", Book: "custody", Note: note}
			  leg.Drifts = append(leg.Drifts,
				   d)
			   drifts = append(drifts, d)
			   leg.OK = false
			   for uid := range b.holders {
				   blockedUsers = append(blockedUsers, uid)
			   }
		  }
		   if wbad := unexplained(ctx, st, "system", ledger.AccountWarm, asset, warmBook); wbad {
			  note := "triparty:unexplained_book book=warm asset=" + asset + " amount=" + warmBook.String()
			   d := TripDrift{Kind: tripUnexplained, Asset: asset, UserID: "system", Book: string(ledger.AccountWarm), Note: note}
			  leg.Drifts = append(leg.Drifts, d)
			  drifts = append(drifts, d)
			  leg.OK = false
		  }
		   if cbad := unexplained(ctx, st, "system", ledger.AccountCold, asset, coldBook); cbad {
			   note := "triparty:unexplained_book book=cold asset=" + asset + " amount=" + coldBook.String()
			  d := TripDrift{Kind: tripUnexplained, Asset: asset, UserID: "system", Book: string(ledger.AccountCold), Note: note}
			  leg.Drifts = append(leg.Drifts, d)
			   drifts = append(drifts, d)
			   leg.OK = false
		  }
		   for _, a := range accs {
			   if strings.ToUpper(a.Asset) != asset || a.UserID == "system" || !a.AccountType.UserFacing() {
				  continue
			   }
			  total := a.Available.Add(a.Frozen)
			  if !total.IsPositive() && !a.Available.IsNegative() && !a.Frozen.IsNegative() {
				   continue
			   }
			   bad := unexplained(ctx, st, a.UserID, a.AccountType, asset, total)
			  if a.Available.IsNegative() || a.Frozen.IsNegative() {
				  bad = true
			   }
			   if !bad {
				  continue
			  }
			  note := "triparty:unexplained_book book=" + string(a.AccountType) + " asset=" + asset + " amount=" + total.String()
			  d := TripDrift{Kind: tripUnexplained, Asset: asset, UserID: a.UserID, Book: string(a.AccountType), Note: note}
			  leg.Drifts = append(leg.Drifts, d)
			   drifts = append(drifts, d)
			  leg.OK = false
			   blockedUsers = append(blockedUsers,
				   a.UserID)
		   }
		  for _, d := range pipe.aged[asset] {
			  leg.Drifts = append(leg.Drifts, d)
			   drifts = append(drifts, d)
			   leg.OK = false
			  if d.UserID != "" && d.UserID != "system" {
				   blockedUsers = append(blockedUsers,
					  d.UserID)
			  }
		  }
		   if !leg.OK {
			   rep.OK = false
			   if materialAbove(asset, bridge, hotObs) || bridgeBad || materialAbove(asset, b.users, custody) {
				  for uid := range b.holders {
					   blockedUsers = append(blockedUsers,
						  uid)
				   }
			  }
		  }
		   rep.Legs = append(rep.Legs, leg)
	  }
	   rep.Drifts = drifts
	  if len(drifts) == 0 {
		  rep.OK = true
		  clearTrip(ctx, led)
		   storeTrip(rep, nil)
		  metrics.TripartyRuns.WithLabelValues("ok").Inc()
		  return rep, nil
	  }
	   decorateDrifts(&rep)
	  rep.Blocked = true
	  reason := blockSummary(rep.Drifts)
	  led.SetPlatformDrift(true, reason)
	   seen := map[string]struct{}{}
	  for _, uid := range blockedUsers {
		   if uid == "" {
			  continue
		   }
		  if _, ok := seen[uid]; ok {
			  continue
		  }
		  seen[uid] = struct{}{}
		   led.NoteReconcile(ctx, &ledger.ReconcileReport{
			  OK:     false,
			   UserID: uid, Blocked: true,
			   Mismatches: []ledger.Mismatch{{UserID: uid, Note: reason}},
		   })
	  }
	   rep.Tickets = openTrip(tickets,
		  actor, drifts)
	  storeTrip(rep, seen)
	   metrics.TripartyRuns.WithLabelValues("drift").Inc()
	   return rep, nil
  }

   func unexplained(ctx context.Context, st ledger.Store, user string, typ ledger.AccountType, asset string, total decimal.Decimal) bool {
	  if total.IsZero() {
		   return false
	  }
	   _,
		  bad, err := ledger.BookGap(ctx, st, user, typ, asset, total)
	  if err != nil {
		   return true
	   }
	   return bad
  }

  func obsOf(asset string, snaps []hotinv.Snapshot, over map[string]hotinv.Snapshot) (hot, warm, cold decimal.Decimal, err error) {
	  asset = strings.ToUpper(asset)
	   if s, ok := over[asset]; ok {
		  h,
			   e1 := parseObs(s.HotAvail, s.HotFrozen)
		   w, e2 := parseObs(s.WarmAvail, "")
		  c, e3 := parseObs(s.ColdAvail, "")
		   if e1 != nil || e2 != nil || e3 != nil {
			  if e1 != nil {
				  return decimal.Zero, decimal.Zero, decimal.Zero, e1
			   }
			  if e2 != nil {
				  return decimal.Zero, decimal.Zero, decimal.Zero, e2
			   }
			  return decimal.Zero, decimal.Zero, decimal.Zero, e3
		  }
		   return h, w, c, nil
	  }
	   for _, s := range snaps {
		  if strings.ToUpper(s.Asset) != asset {
			  continue
		   }
		  h, e1 := parseObs(s.HotAvail, s.HotFrozen)
		   w, e2 := parseObs(s.WarmAvail, "")
		  c, e3 := parseObs(s.ColdAvail, "")
		   if e1 != nil || e2 != nil || e3 != nil {
			   if e1 != nil {
				   return decimal.Zero, decimal.Zero, decimal.Zero, e1
			  }
			   if e2 != nil {
				  return decimal.Zero, decimal.Zero, decimal.Zero, e2
			   }
			   return decimal.Zero, decimal.Zero, decimal.Zero, e3
		  }
		   return h, w, c, nil
	   }
	  return decimal.Zero,
		   decimal.Zero, decimal.Zero, nil
   }

   func parseObs(a, b string) (decimal.Decimal, error) {
	   x, err := decField(a)
	  if err != nil {
		  return decimal.Zero, err
	   }
	  y, err := decField(b)
	  if err != nil {
		   return decimal.Zero, err
	   }
	  return x.Add(y), nil
   }

  func decField(s string) (decimal.Decimal, error) {
	   s = strings.TrimSpace(s)
	  if s == "" {
		  return decimal.Zero, nil
	  }
	  return decimal.NewFromString(s)
   }

  func indexObserve(rows []hotinv.Snapshot) map[string]hotinv.Snapshot {
	  out := map[string]hotinv.Snapshot{}
	  for _, s := range rows {
		  asset := strings.ToUpper(strings.TrimSpace(s.Asset))
		   if asset == "" {
			   continue
		  }
		   out[asset] = s
	   }
	  return out
   }

  func openTrip(tickets *Book, actor string, drifts []TripDrift) []Ticket {
	   if tickets == nil {
		  return nil
	   }
	  var out []Ticket
	   n := 0
	  for _, d := range drifts {
		  if n >= 200 {
			  break
		  }
		  user := d.UserID
		   if user == "" {
			  user = "system"
		  }
		   if tripOpen(tickets, user, d.Asset, d.Book, d.Kind) {
			   continue
		   }
		   note := d.Note
		   got,
			  err := tickets.Open(actor, user, d.Book, d.Asset, note)
		   if err != nil || got == nil {
			  continue
		  }
		  out = append(out, *got)
		  n++
	  }
	  return out
   }

   func tripOpen(tickets *Book, user, asset, book, kind string) bool {
	  needle := "triparty:" + kind
	   for _, t := range tickets.List(user, 200) {
		  if t.State != StateOpen && t.State != StateAcked {
			  continue
		  }
		   if t.Asset == asset && t.Book == book && strings.Contains(t.Note, needle) {
			  return true
		  }
	  }
	  return false
   }

   func storeTrip(rep TripReport, users map[string]struct{}) {
	   tripState.mu.Lock()
	   defer tripState.mu.Unlock()
	   tripState.last = rep
	  if users == nil {
		  return
	  }
	  if tripState.users == nil {
		  tripState.users = map[string]struct{}{}
	  }
	  for u := range users {
		  tripState.users[u] = struct{}{}
	   }
   }

  func clearTrip(ctx context.Context, led *ledger.Service) {
	  tripState.mu.Lock()
	  users := tripState.users
	  tripState.users = map[string]struct{}{}
	  tripState.mu.Unlock()
	  if led == nil {
		  return
	  }
	   led.SetPlatformDrift(false, "")
	   for uid := range users {
		   led.NoteReconcile(ctx,
			  &ledger.ReconcileReport{OK: true, UserID: uid})
	  }
  }
