package riskgate

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/policypack"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu   sync.Mutex
	halt bool
	eng   *policypack.Engine
	wins  []Window
	edges map[string]map[string]int
	first map[string]map[string]bool
	clock func() time.Time
}

   func New() *Service {
	   return &Service{
		  eng:   policypack.DefaultEngine(),
		   wins:  []Window{},
		   edges: map[string]map[string]int{},
		  first: map[string]map[string]bool{},
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

   func (s *Service) BindEngine(e *policypack.Engine) *Service {
	   if s != nil && e != nil {
		  s.eng = e
	   }
	  return s
   }

  func (s *Service) SetHalt(on bool) {
	   if s == nil {
		   return
	  }
	  s.mu.Lock()
	  s.halt = on
	   s.mu.Unlock()
  }

  func (s *Service) Halted() bool {
	  if s == nil {
		  return false
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  return s.halt
  }

   func (s *Service) Engine() *policypack.Engine {
	   if s == nil || s.eng == nil {
		   return policypack.DefaultEngine()
	   }
	   return s.eng
  }

   func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		   return time.Now().UTC()
	   }
	   return s.clock()
   }

// Screen 先记窗再评包。缺包/金额坏/halt 失败即关。
   func (s *Service) Screen(req ScreenReq) (Verdict, error) {
	   if s == nil {
		   return Verdict{}, ErrUnavailable
	  }
	   if s.Halted() {
		   return Verdict{Reason: "halted"}, ErrHalt
	  }
	   uid,
		   err := validate.UserID(req.UserID)
	   if err != nil {
		  return Verdict{}, ErrUser
	  }
	  as, err := validate.Asset(req.Asset)
	   if err != nil {
		  return Verdict{}, ErrAsset
	  }
	   amt, err := money.Parse(req.Amount)
	   if err != nil {
		   return Verdict{}, ErrAmount
	   }
	  pack := strings.TrimSpace(req.Pack)
	   if pack == "" {
		  pack = "kyc0"
	   }
	  kind := policy.Kind(strings.ToLower(strings.TrimSpace(req.Kind)))
	  if kind == "" {
		  kind = policy.KindWithdraw
	  }
	   // 与 compliance.NightWindowHook 一致：仅当 hour 非法时用当前 UTC 小时；0 点整是有效窗口。
	   hour := req.HourUTC
	   if hour < 0 || hour > 23 {
		   hour = s.now().Hour()
	   }
	  s.mu.Lock()
	   v1h, v24h := s.velocityLocked(uid, s.now())
	   destN, dest24 := s.destLocked(uid, req.Dest, s.now())
	  first := s.firstDestLocked(uid, req.Dest)
	   cluster := s.clusterLocked(req.Dest)
	   corr := s.correlatedLocked(uid,
		  req.Dest)
	   s.mu.Unlock()
	  in := policypack.Input{
		   Kind:   kind,
		   Amount: amt, TodaySum: money.Zero(),
		   Velocity1h: v1h, Velocity24h: v24h,
		  DestCount: destN, DestCount24h: dest24, FirstDest: first,
		   HourUTC: hour, Allowlisted: req.Allowlisted, Asset: as,
		  CheckDay: req.Weekday != "", Weekday: parseWeekday(req.Weekday),
		  ClusterSize: cluster, Correlated: corr,
	  }
	   d, err := s.Engine().Decide(pack, in)
	   if err != nil {
		  return Verdict{Pack: pack, Mode: string(s.Engine().Mode()), Reason: "eval failed"}, ErrEval
	   }
	   out := Verdict{
		  Allow: d.Allow, Reason: d.Reason, Pack: d.Pack,
		   Mode: string(s.Engine().Mode()), HoldReview: d.HoldReview,
		   ShadowHit: strings.HasPrefix(d.Reason, "shadow:"),
	   }
	  if !out.Allow {
		  return out, ErrDeny
	   }
	  s.mu.Lock()
	   s.recordLocked(uid, req.Dest, as, amt.String(), s.now())
	  s.mu.Unlock()
	  return out, nil
   }

   func (s *Service) velocityLocked(user string, now time.Time) (h, d int) {
	  hCut := now.Add(-time.Hour)
	   dCut := now.Add(-24 * time.Hour)
	  for _, w := range s.wins {
		   if w.UserID != user {
			  continue
		  }
		   if w.At.After(dCut) {
			   d++
		   }
		  if w.At.After(hCut) {
			  h++
		   }
	   }
	  return h, d
   }

   func (s *Service) destLocked(user, dest string, now time.Time) (all, d24 int) {
	  dest = strings.TrimSpace(dest)
	   if dest == "" {
		   return 0, 0
	   }
	   cut := now.Add(-24 * time.Hour)
	   seen := map[string]bool{}
	   seen24 := map[string]bool{}
	  for _, w := range s.wins {
		  if w.UserID != user || w.Dest == "" {
			   continue
		   }
		  seen[w.Dest] = true
		  if w.At.After(cut) {
			   seen24[w.Dest] = true
		   }
	   }
	   if !seen[dest] {
		   seen[dest] = true
	   }
	  if !seen24[dest] {
		  seen24[dest] = true
	  }
	   return len(seen), len(seen24)
   }

   func (s *Service) firstDestLocked(user, dest string) bool {
	  dest = strings.TrimSpace(dest)
	   if dest == "" {
		   return false
	   }
	  m := s.first[user]
	   if m == nil {
		  return true
	  }
	   return !m[dest]
  }

   func (s *Service) clusterLocked(dest string) int {
	  dest = strings.TrimSpace(dest)
	   if dest == "" {
		  return 0
	  }
	   root := dest
	   if i := strings.Index(dest, ":"); i > 0 {
		  root = dest[:i]
	   }
	   n := 1
	   if m := s.edges[root]; m != nil {
		  n = len(m) + 1
	   }
	  return n
   }

   func (s *Service) correlatedLocked(user, dest string) int {
	  if dest == "" {
		  return 0
	   }
	   n := 0
	   for _, w := range s.wins {
		  if w.Dest == dest && w.UserID != user {
			  n++
		   }
	  }
	  return n
  }

   func (s *Service) recordLocked(user, dest, asset, amount string, at time.Time) {
	   s.wins = append(s.wins,
		  Window{UserID: user, Dest: dest, Asset: asset, At: at, Amount: amount})
	  if dest != "" {
		  if s.first[user] == nil {
			   s.first[user] = map[string]bool{}
		  }
		   s.first[user][dest] = true
		   root := dest
		  if i := strings.Index(dest, ":"); i > 0 {
			   root = dest[:i]
		  }
		  if s.edges[root] == nil {
			  s.edges[root] = map[string]int{}
		   }
		  s.edges[root][dest]++
	   }
   }

   func (s *Service) Windows() []Window {
	   if s == nil {
		   return nil
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]Window, len(s.wins))
	   copy(out, s.wins)
	   return out
   }

   func parseWeekday(s string) time.Weekday {
	   switch s {
	   case "Saturday":
		  return time.Saturday
	   case "Sunday":
		  return time.Sunday
	   case "Monday":
		   return time.Monday
	  default:
		  return time.Thursday
	   }
  }
