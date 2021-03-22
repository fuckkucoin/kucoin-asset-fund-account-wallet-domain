package attribrt

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/attribrt/collide"
	"account-assets.kucoin.net/funding/internal/attribrt/noise"
	"account-assets.kucoin.net/funding/internal/attribrt/policy"
	"account-assets.kucoin.net/funding/internal/attribrt/utxograph"
	"account-assets.kucoin.net/funding/internal/chainad"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// Engine 归因运行时。钩子关闭时观察线程不应调用它来挡贷记。
type Engine struct {
	mu  sync.Mutex
	tab *policy.Table
	rules []noise.Rule
	book  *collide.Book
	bind   Binder
	shared func(chain string) (string, bool)
	intel  func(chain, addr string) (contract, sanctioned bool)
	hot    []string
	deny   []string
	look []string
	contracts map[string]struct{}
	sql       *persist.SQLDB
	rate      *rateWin
	burst map[string][]time.Time
	graphs map[string]utxograph.Report
	decs   map[string]Decision
	byTx map[string]string
	envs []Envelope
	prev  map[string]string
	queue []QueueItem
	delay map[string]Sight
	order []string
	clock func() time.Time
	capDec int
}

// QueueItem 台席队列。drop 不入队，只留信封。

type QueueItem struct {
	ID         string `json:"id"`
	DecisionID string `json:"decision_id"`
	Chain      string `json:"chain"`
	TxHash     string `json:"tx_hash"`
	Output     int    `json:"output_index"`
	Kind string `json:"kind"`
	State     string    `json:"state"`
	Note      string    `json:"note,omitempty"`
	NotBefore time.Time `json:"not_before,omitempty"`
	At        time.Time `json:"at"`
}

// New 装载策略。目录不变量失败时返回错误，不得用空引擎放行。

  func New() (*Engine, error) {
	   tab, err := policy.Load()
	  if err != nil {
		  return nil, err
	  }
	   return &Engine{
		  tab: tab,
		   rules: noiseRules(),
		  book:   collide.NewBook(),
		   rate:   newRate(),
		   burst:  map[string][]time.Time{},
		   graphs: map[string]utxograph.Report{},
		  decs:   map[string]Decision{},
		   byTx: map[string]string{},
		  prev:      map[string]string{},
		  delay:     map[string]Sight{},
		  contracts: map[string]struct{}{},
		  clock:     func() time.Time { return time.Now().UTC() },
		   capDec:    4096,
	   }, nil
  }

  func noiseRules() []noise.Rule {
	   return noise.Strategy()
   }

// SetClock 固定时钟，回放用。

   func (e *Engine) SetClock(fn func() time.Time) {
	   if e == nil || fn == nil {
		  return
	  }
	   e.mu.Lock()
	  e.clock = fn
	  e.book.SetClock(fn)
	  e.mu.Unlock()
   }

   // SetBinder 只读绑定源。
  func (e *Engine) SetBinder(b Binder) {
	  if e == nil {
		   return
	  }
	   e.mu.Lock()
	   e.bind = b
	  e.mu.Unlock()
   }

// SetShared 链上共享地址。chain_addr 命名空间缺收款地址时用它补，补不到就失败关闭。

   func (e *Engine) SetShared(fn func(chain string) (string, bool)) {
	  if e == nil {
		  return
	   }
	  e.mu.Lock()
	  e.shared = fn
	  e.mu.Unlock()
   }

   // SetIntel 地址情报。返回值只影响噪声，不直接入账。
  func (e *Engine) SetIntel(fn func(chain, addr string) (bool, bool)) {
	  if e == nil {
		   return
	  }
	   e.mu.Lock()
	  e.intel = fn
	  e.mu.Unlock()
  }

   // AddHot 登记热钱包或找零地址。

  func (e *Engine) AddHot(addrs ...string) { e.addList(&e.hot, addrs) }

// AddDeny 空投来源拒绝名单。
   func (e *Engine) AddDeny(addrs ...string) { e.addList(&e.deny, addrs) }

// AddLook 投毒地址。

   func (e *Engine) AddLook(addrs ...string) { e.addList(&e.look, addrs) }

// AddContract 标记合约地址，供空投分类器使用。
   func (e *Engine) AddContract(chain, addr string) {
	   if e == nil || strings.TrimSpace(addr) == "" {
		   return
	   }
	   e.mu.Lock()
	  e.contracts[strings.ToUpper(chain)+"|"+strings.ToLower(strings.TrimSpace(addr))] = struct{}{}
	   e.mu.Unlock()
  }

  func (e *Engine) addList(dst *[]string, addrs []string) {
	  if e == nil {
		  return
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  for _, a := range addrs {
		  a = strings.TrimSpace(a)
		  if a != "" {
			  *dst = append(*dst, a)
		  }
	  }
   }

// Policy 策略表。

   func (e *Engine) Policy() *policy.Table {
	  if e == nil {
		   return nil
	   }
	  return e.tab
  }

// Collisions 碰撞案簿。
  func (e *Engine) Collisions() *collide.Book {
	   if e == nil {
		   return nil
	  }
	  return e.book
   }

// Resolve 产出结论。CreditIntent 为真只表示允许观察线程接着，本函数不贷记。
  func (e *Engine) Resolve(s Sight) (Decision, error) {
	   if e == nil || e.tab == nil {
		   return Decision{CreditIntent: false, Disposition: "desk", Reasons: []string{"policy_closed"}}, ErrClosed
	   }
	   now := s.At
	   if now.IsZero() {
		   now = e.now()
	   }
	  s.Chain = strings.ToUpper(strings.TrimSpace(s.Chain))
	   s.Asset = strings.ToUpper(strings.TrimSpace(s.Asset))
	   s.TxHash = strings.TrimSpace(s.TxHash)
	   s.Memo = strings.TrimSpace(s.Memo)
	   s.Tag = strings.TrimSpace(s.Tag)
	  s.From = strings.TrimSpace(s.From)
	   s.To = strings.TrimSpace(s.To)
	   d := Decision{
		   ID: uuid.NewString(), Chain: s.Chain, TxHash: s.TxHash, OutputIndex: s.OutputIndex,
		  Asset: s.Asset, Amount: strings.TrimSpace(s.Amount), At: now, Replay: s.Replay,
		  Disposition: "pass", CreditIntent: false,
	   }
	  ev := e.tab.Evaluate(s.Chain, s.PolicyVersion, s.Replay)
	   d.Family = string(ev.Row.Family)
	   d.PolicyVersion = ev.Ver.ID
	  if ev.Ver.ID == "" {
		   d.PolicyVersion = s.PolicyVersion
	  }
	   view := chainad.ReadAttribView(s.Chain)
	  d.ChainadFamily = view.Family
	   if ev.Closed {
		   d.Disposition = "desk"
		  d.Reasons = []string{ev.Reason}
		   d.Band = "refuse"
		   if !s.Preview {
			   e.finish(&d, s)
		  }
		  return d, nil
	  }
	  if view.MemoMax > 0 && ev.MemoMax > view.MemoMax {
		   ev.MemoMax = view.MemoMax
	  }
	  dust := e.tab.AssetDust(s.Chain, s.Asset, ev.Dust)
	   shared, intel, bind := e.hooks()
	   if s.To == "" && ev.Row.Namespace == "chain_addr" && shared != nil {
		  if addr, ok := shared(s.Chain); ok {
			   s.To = addr
		   }
	  }
	  rawMemo := s.Memo
	   if rawMemo == "" {
		   rawMemo = s.Tag
	  }
	  numeric := ev.Row.MemoMode == "numeric" || ev.Row.MemoMode == "id"
	   if ev.Row.MemoRequired && rawMemo == "" {
		   d.Disposition = "desk"
		   d.Reasons = append(d.Reasons, "memo_required")
	   }
	   if ev.MemoMax > 0 && len([]rune(rawMemo)) > ev.MemoMax {
		  d.Reasons = append(d.Reasons, "memo_charset")
		   d.Disposition = "desk"
	  }
	  if ev.Row.MemoMin > 0 && rawMemo != "" && len([]rune(rawMemo)) < ev.Row.MemoMin {
		   d.Reasons = append(d.Reasons, "memo_charset")
		  d.Disposition = "desk"
	  }
	   fold := ""
	   if rawMemo != "" {
		   sk, ok := collide.FoldKey(rawMemo, numeric, ev.Row.CaseFold, ev.Row.StripZW, ev.Row.LeadingZero)
		  if !ok {
			  d.Reasons = append(d.Reasons, "memo_charset")
			  d.Disposition = "desk"
		  } else {
			   fold = sk
			   if ev.Row.Charset == "numeric" {
				   for _, r := range sk {
					   if r < '0' || r > '9' {
						  d.Reasons = append(d.Reasons, "memo_charset")
						   d.Disposition = "desk"
						  fold = ""
						   break
					  }
				  }
			   }
		   }
	   }
	  ns := collide.Namespace(ev.Row.Namespace,
		  s.Chain, strings.ToLower(s.To))
	  if ev.Row.Namespace == "chain_addr" && ns == "" && rawMemo != "" {
		  d.Reasons = append(d.Reasons, "namespace_incomplete")
		   d.Disposition = "desk"
	  }
	   contract, sanctioned := false, false
	   if intel != nil && s.From != "" {
		   contract, sanctioned = intel(s.Chain, s.From)
	   }
	  if e.contractListed(s.Chain, s.From) {
		  contract = true
	   }
	   burst := e.noteBurst(s.Chain, s.From, now, !s.Preview)
	   nz := noise.Run(noise.Input{
		  Chain: s.Chain, Asset: s.Asset, Amount: d.Amount, From: s.From, To: s.To,
		   Memo: s.Memo, Tag: s.Tag, Script: s.Script, Native: ev.Native, Dust: dust,
		   Decimals: ev.Row.Decimals, Enabled: ev.Row.Noise, Rules: e.rules,
		  Hot: e.snapshot(e.hot), Deny: e.snapshot(e.deny), Look: e.snapshot(e.look),
		  Contract: contract, Sanctioned: sanctioned, Burst: burst,
		   CaseFold: ev.Row.CaseFold, Numeric: numeric, StripZW: ev.Row.StripZW, LeadingZero: ev.Row.LeadingZero,
	   })
	   for _, h := range nz.Hits {
		  d.Reasons = append(d.Reasons, h.Reason)
	  }
	  if nz.Block && rankDisp(nz.Disposition) > rankDisp(d.Disposition) {
		   d.Disposition = nz.Disposition
	  }
	   rep, graphReason := e.graph(s, ev, dust, bind)
	   if graphReason != "" {
		   d.Reasons = append(d.Reasons, graphReason)
		   if d.Disposition == "pass" {
			  d.Disposition = "desk"
		   }
	  }
	  user,
		   kind := e.identify(s, ev, rep, bind)
	   d.UserID = user
	   d.Kind = kind
	   if kind == "mismatch" {
		  d.Reasons = append(d.Reasons, "user_mismatch")
		  d.Disposition = "desk"
		   d.Kind = ""
	  }
	  if label, ok := labeled(rep, s.OutputIndex); ok {
		   d.Label = label.Label
		  if label.UserID != "" {
			   if d.UserID == "" {
				   d.UserID = label.UserID
				   d.Kind = "utxo"
			  } else if d.UserID != label.UserID {
				   d.Reasons = append(d.Reasons, "user_mismatch")
				   d.Disposition = "desk"
			   }
		   }
		   if !label.Credit {
			  if label.Reason != "" {
				   d.Reasons = append(d.Reasons, label.Reason)
			   }
			   if d.Disposition == "pass" {
				   d.Disposition = "desk"
			  }
			  if label.Label == "change" || label.Label == "dust" || label.Label == "nulldata" {
				  d.Disposition = "drop"
			   }
		  }
	   } else if ev.Row.GraphRequired {
		   d.Reasons = append(d.Reasons, "utxo_graph_required")
		   d.Disposition = "desk"
	   }
	  if s.UserHint != "" && d.UserID != "" && s.UserHint != d.UserID {
		   d.Reasons = append(d.Reasons, "user_mismatch")
		  d.Disposition = "desk"
	   }
	   if d.UserID == "" && s.UserHint != "" && !ev.Row.MemoRequired && !ev.Row.GraphRequired {
		   d.UserID = s.UserHint
		  if d.Kind == "" {
			  d.Kind = "address"
		  }
	  }
	  if d.UserID == "" {
		  d.Reasons = append(d.Reasons, "unbound")
		   if d.Disposition == "pass" {
			   d.Disposition = "desk"
		   }
	   }
	   // 尘埃和字符集非法不写入碰撞索引，避免脏键占住折叠位。
	  indexOK := fold != "" && d.UserID != "" && !hasReason(d.Reasons, "noise_dust") && !hasReason(d.Reasons, "memo_charset") && ns != ""
	   if indexOK {
		  if s.Preview {
			   if e.book == nil || e.book.Peek(s.Chain, ns, fold, d.UserID) {
				   d.Reasons = append(d.Reasons, "collide_blocked")
				  d.Disposition = "desk"
			  }
		  } else {
			   c, conflict, err := e.book.Note(s.Chain, ns, rawMemo, fold, d.UserID, ev.Row.Collision)
			  if c.ID != "" {
				  e.saveCollision(c)
			  }
			   if err != nil {
				  d.Reasons = append(d.Reasons, "collide_blocked")
				   d.Disposition = "desk"
			   } else if conflict || (c.ID != "" && !c.Allows(d.UserID)) {
				   d.CollisionID = c.ID
				   d.Reasons = append(d.Reasons, "collide_blocked")
				   d.Disposition = "desk"
			   } else if c.ID != "" {
				  d.CollisionID = c.ID
				  if !c.Allows(d.UserID) {
					  d.Reasons = append(d.Reasons, "collide_blocked")
					   d.Disposition = "desk"
				   }
			   } else if !e.book.Allows(s.Chain, ns, fold, d.UserID) {
				  d.Reasons = append(d.Reasons, "collide_blocked")
				   d.Disposition = "desk"
			  }
		   }
	  }
	   conf := 10000 - nz.Penalty
	   if conf < 0 {
		   conf = 0
	   }
	  d.Confidence = conf
	  d.Band = BandOf(conf, ev.Floor)
	   if d.Band == "refuse" && d.Disposition == "pass" {
		   d.Disposition = "hold"
		  d.Reasons = append(d.Reasons, "confidence_low")
	  }
	   if !e.admit(s, now, ev.Row.RatePerMin, !s.Preview) {
		  d.Reasons = append(d.Reasons, "rate_limited")
		  if rankDisp(d.Disposition) < rankDisp("delay") {
			   d.Disposition = "delay"
		  }
	  }
	   d.CreditIntent = allowCredit(d)
	  if !s.Preview {
		   e.finish(&d, s)
	   }
	  return d, nil
   }

   func allowCredit(d Decision) bool {
	   if d.UserID == "" || d.Disposition != "pass" {
		  return false
	  }
	   for _, r := range d.Reasons {
		  switch r {
		  case "unbound", "utxo_change", "utxo_unbound", "utxo_conserve", "utxo_merge",
			  "utxo_graph_required", "collide_blocked", "policy_closed", "policy_stale",
			   "policy_family", "unknown_version", "unknown_chain", "user_mismatch",
			  "memo_required", "memo_charset", "namespace_incomplete", "confidence_low",
			  "rate_limited", "noise_dust", "noise_self", "noise_poison", "noise_airdrop",
			  "noise_decoy", "noise_spam", "noise_script", "noise_scale", "noise_homoglyph",
			   "noise_sweep", "output_missing":
			   return false
		  }
	  }
	  if d.Label == "change" || d.Label == "dust" || d.Label == "unbound" || d.Label == "nulldata" {
		   return false
	  }
	  return true
   }

  func (e *Engine) finish(d *Decision, s Sight) {
	   env := e.seal(*d)
	  d.EnvelopeID = env.ID
	   e.mu.Lock()
	   e.decs[d.ID] = *d
	   e.byTx[txKey(d.Chain,
		   d.TxHash, d.OutputIndex)] = d.ID
	  e.order = append(e.order, d.ID)
	   if len(e.order) > e.capDec {
		   old := e.order[0]
		  e.order = e.order[1:]
		  delete(e.decs, old)
	  }
	   e.envs = append(e.envs, env)
	  if len(e.envs) > e.capDec {
		  e.envs = e.envs[len(e.envs)-e.capDec:]
	  }
	   e.prev[d.Chain] = env.PayloadHash
	  if d.Disposition == "delay" {
		  e.delay[d.ID] = s
	   }
	  if d.Disposition == "desk" || d.Disposition == "hold" || d.Disposition == "delay" {
		  e.queue = append(e.queue, QueueItem{
			   ID:         uuid.NewString(),
			   DecisionID: d.ID, Chain: d.Chain, TxHash: d.TxHash,
			   Output: d.OutputIndex, Kind: d.Disposition, State: "open",
			  Note: strings.Join(d.Reasons, ","), At: d.At,
			  NotBefore: d.At.Add(time.Duration(delaySeconds(e, s)) * time.Second),
		   })
	  }
	  e.mu.Unlock()
	  e.persist(*d, env)
  }

  func delaySeconds(e *Engine, s Sight) int {
	  row, ok := e.tab.Row(s.Chain)
	   if !ok || row.DelaySec <= 0 {
		  return 60
	  }
	   return row.DelaySec
   }

   func (e *Engine) seal(d Decision) Envelope {
	   e.mu.Lock()
	   prev := e.prev[d.Chain]
	  e.mu.Unlock()
	  h := hashDecision(d, prev)
	  return Envelope{
		   ID: uuid.NewString(), DecisionID: d.ID, Chain: d.Chain, TxHash: d.TxHash,
		  OutputIndex: d.OutputIndex, Disposition: d.Disposition, CreditIntent: d.CreditIntent,
		   PolicyVersion: d.PolicyVersion, Reasons: strings.Join(d.Reasons, ","),
		   PrevHash:    prev,
		   PayloadHash: h, At: d.At.UTC().Format(time.RFC3339Nano),
	   }
  }

  func (e *Engine) graph(s Sight, ev policy.Eval, dust string, bind Binder) (utxograph.Report, string) {
	  if s.Graph != nil && len(s.Graph.Inputs) > 0 {
		   tx := utxograph.Tx{Chain: s.Chain,
			   TxID: s.TxHash, Asset: s.Asset, Fee: s.Graph.Fee}
		   for _, c := range s.Graph.Inputs {
			  tx.Inputs = append(tx.Inputs, utxograph.Coin{TxID: c.TxID, Vout: c.Vout, Amount: c.Amount, Address: c.Address, Script: c.Script})
		   }
		  for _, c := range s.Graph.Outputs {
			   txid := c.TxID
			   if txid == "" {
				   txid = s.TxHash
			  }
			  tx.Outputs = append(tx.Outputs,
				  utxograph.Coin{TxID: txid, Vout: c.Vout, Amount: c.Amount, Address: c.Address, Script: c.Script})
		  }
		   rep, err := utxograph.Eval(tx, utxograph.Limits{
			  Dust: dust, FeeMin: ev.Row.FeeMin, FeeMaxBps: ev.Row.FeeMaxBps,
			  Scripts: ev.Row.Scripts,
			  Hot:     e.snapshot(e.hot),
		   }, e.coinBind(s.Chain, bind))
		  if err != nil {
			  return utxograph.Report{}, "utxo_conserve"
		   }
		   if !s.Preview {
			  e.mu.Lock()
			   e.graphs[s.Chain+"|"+s.TxHash] = rep
			   e.mu.Unlock()
		   }
		   if rep.Incomplete {
			   return rep, "utxo_graph_required"
		   }
		   if !rep.ConserveOK {
			   return rep, "utxo_conserve"
		   }
		  if rep.Merge {
			   return rep, "utxo_merge"
		  }
		  return rep, ""
	   }
	  if !ev.Row.GraphRequired {
		  return utxograph.Report{}, ""
	  }
	  e.mu.Lock()
	   rep, ok := e.graphs[s.Chain+"|"+s.TxHash]
	   e.mu.Unlock()
	  if !ok {
		  return utxograph.Report{}, "utxo_graph_required"
	   }
	   if rep.Incomplete {
		  return rep, "utxo_graph_required"
	   }
	   if !rep.ConserveOK {
		   return rep, "utxo_conserve"
	   }
	  if rep.Merge {
		  return rep, "utxo_merge"
	  }
	  return rep, ""
   }

  func (e *Engine) coinBind(chain string, bind Binder) utxograph.Binder {
	  return func(address, coinKey string) (string, bool) {
		  if bind == nil {
			   return "", false
		  }
		   if address != "" {
			  if u, ok := bind.Lookup(chain, "address", address); ok {
				   return u, true
			  }
		  }
		  if coinKey != "" {
			   if u, ok := bind.Lookup(chain, "utxo", coinKey); ok {
				  return u, true
			   }
		  }
		   return "", false
	  }
   }

   func (e *Engine) identify(s Sight, ev policy.Eval, rep utxograph.Report, bind Binder) (string, string) {
	   if bind == nil {
		  return "", ""
	  }
	  var memoUser, tagUser, utxoUser, addrUser string
	   var memoOK, tagOK, utxoOK, addrOK bool
	  if s.Memo != "" {
		  memoUser, memoOK = bind.Lookup(s.Chain, "memo", s.Memo)
	  }
	  if s.Tag != "" {
		  tagUser, tagOK = bind.Lookup(s.Chain, "tag", s.Tag)
	  }
	  if memoOK && tagOK && memoUser != tagUser {
		   return "", "mismatch"
	   }
	  key := s.TxHash + ":" + itoa(s.OutputIndex)
	   utxoUser, utxoOK = bind.Lookup(s.Chain, "utxo", key)
	  if s.To != "" {
		  addrUser, addrOK = bind.Lookup(s.Chain, "address", s.To)
	   }
	   _ = rep
	   switch {
	  case memoOK:
		   return memoUser, "memo"
	  case tagOK:
		   return tagUser, "tag"
	   case utxoOK:
		   return utxoUser, "utxo"
	  case addrOK && !ev.Row.MemoRequired:
		   return addrUser, "address"
	  default:
		  return "", ""
	   }
   }

   func labeled(rep utxograph.Report, vout int) (utxograph.Labeled, bool) {
	  if len(rep.Coins) == 0 {
		   return utxograph.Labeled{}, false
	  }
	   return utxograph.Pick(rep, vout)
   }

   func (e *Engine) noteBurst(chain, from string, now time.Time, record bool) int {
	  if from == "" {
		  return 0
	   }
	   key, err := RateKey("burst", chain, from, "")
	  if err != nil {
		   return 0
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  cut := now.Add(-time.Minute)
	   cur := e.burst[key]
	   n := 0
	  for _, t := range cur {
		   if !t.Before(cut) {
			  n++
		  }
	   }
	   if !record {
		   return n + 1
	  }
	   keep := make([]time.Time, 0, n+1)
	   for _, t := range cur {
		   if !t.Before(cut) {
			   keep = append(keep, t)
		  }
	  }
	  keep = append(keep, now)
	   e.burst[key] = keep
	   return len(keep)
   }

  func (e *Engine) admit(s Sight, now time.Time, limit int, record bool) bool {
	   who := s.From
	  if who == "" {
		   who = s.TxHash
	  }
	  key, err := RateKey("resolve", s.Chain, who, s.Asset)
	   if err != nil {
		   return false
	   }
	  if !record {
		  if limit <= 0 {
			  return true
		   }
		  return e.rate.count(key,
			  time.Minute, now) < limit
	   }
	   return e.rate.admit(key, limit, time.Minute, now)
   }

   func (e *Engine) hooks() (func(string) (string, bool), func(string, string) (bool, bool), Binder) {
	  if e == nil {
		   return nil, nil, nil
	  }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   return e.shared, e.intel, e.bind
   }

  func (e *Engine) contractListed(chain, from string) bool {
	   if e == nil || from == "" {
		   return false
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  _, ok := e.contracts[chain+"|"+strings.ToLower(from)]
	  return ok
   }

   func (e *Engine) snapshot(xs []string) []string {
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  out := make([]string, len(xs))
	   copy(out, xs)
	  return out
   }

   func (e *Engine) now() time.Time {
	   if e == nil || e.clock == nil {
		   return time.Now().UTC()
	  }
	  return e.clock()
   }

   func txKey(chain, hash string, out int) string {
	  return chain + "|" + hash + "|" + itoa(out)
   }

   func hasReason(xs []string, want string) bool {
	  for _, s := range xs {
		  if s == want {
			   return true
		  }
	  }
	  return false
  }

   func rankDisp(s string) int {
	  switch s {
	  case "drop":
		  return 1
	  case "delay":
		   return 2
	  case "hold":
		  return 3
	   case "desk":
		   return 4
	   default:
		  return 0
	   }
  }

// Get 按结论编号读。
  func (e *Engine) Get(id string) (Decision, bool) {
	  if e == nil {
		  return Decision{}, false
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  d, ok := e.decs[id]
	   return d, ok
   }

// Find 按链、交易、输出读最近一次结论。
   func (e *Engine) Find(chain, hash string, output int) (Decision, bool) {
	  if e == nil {
		  return Decision{}, false
	  }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  id := e.byTx[txKey(strings.ToUpper(chain), strings.TrimSpace(hash), output)]
	  d, ok := e.decs[id]
	   return d, ok
   }

// Envelopes 最近的信封副本。

  func (e *Engine) Envelopes(n int) []Envelope {
	  if e == nil {
		   return nil
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   if n <= 0 || n > len(e.envs) {
		  n = len(e.envs)
	  }
	   out := make([]Envelope, n)
	   copy(out, e.envs[len(e.envs)-n:])
	   return out
  }

// Queue 台席队列副本。
  func (e *Engine) Queue() []QueueItem {
	   if e == nil {
		   return nil
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   out := make([]QueueItem, len(e.queue))
	  copy(out, e.queue)
	  return out
   }

// Books 热地址、拒绝名单、地址。
  func (e *Engine) Books() (hot, deny, look []string) {
	  if e == nil {
		  return nil, nil, nil
	  }
	  return e.snapshot(e.hot), e.snapshot(e.deny), e.snapshot(e.look)
  }

// DueDelays 到点的延迟观察，交给工人重跑。重跑仍不入账。
  func (e *Engine) DueDelays(now time.Time) []Sight {
	   if e == nil {
		  return nil
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  var out []Sight
	   for id, s := range e.delay {
		   d, ok := e.decs[id]
		  if !ok {
			   delete(e.delay, id)
			   continue
		  }
		   wait := time.Duration(delaySeconds(e,
			   s)) * time.Second
		   if now.Before(d.At.Add(wait)) {
			  continue
		   }
		  out = append(out, s)
		   delete(e.delay, id)
	  }
	  return out
  }
