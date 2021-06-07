package fundcircuit

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// 策略默认值。调参须同步 docs/product/fund-circuit-bus.md。

const (
	DefaultWindow = 5 * time.Minute
	DefaultSignalTTL  = 30 * time.Minute
	DefaultWatchScore = 40
	DefaultWarnScore  = 70
	DefaultTripScore = 100
	DefaultHaltScore = 220
	DefaultCooldown = 10 * time.Minute
	DefaultCooldownMax    = 2 * time.Hour
	DefaultHalfOpenProbes = 1
	DefaultProbeTTL = 2 * time.Minute
	DefaultCrossNum = 3
	DefaultCrossDen = 2
	DefaultPathFloor = 5
)

// 作用域跳闸广度下限：资产 / 链 / 全局须有多名用户参与，单人刷量只挡到自己。
const (
	DefaultMinUsersUser  = 1
	DefaultMinUsersAsset = 3
	DefaultMinUsersChain = 3
	DefaultMinUsersGlobal = 5
)

// BlockPolicy 一版阻断策略。

type BlockPolicy struct {
	Version int `json:"version"`
	// Window 信号聚合窗口。
	Window time.Duration `json:"window"`
	// SignalTTL 信号台账保留时长；应不小于 Window。
	SignalTTL time.Duration `json:"signal_ttl"`
	// WatchScore / WarnScore / TripScore / HaltScore 分档阈值，须严格递增。
	WatchScore int `json:"watch_score"`
	WarnScore int `json:"warn_score"`
	TripScore int `json:"trip_score"`
	HaltScore int `json:"halt_score"`
	// Cooldown open→half_open 的冷却；CooldownMax 为多次跳闸后的退避上限。
	Cooldown    time.Duration `json:"cooldown"`
	CooldownMax time.Duration `json:"cooldown_max"`
	// HalfOpenProbes 半开期探针配额；ProbeTTL 单枚探针有效期。
	HalfOpenProbes int           `json:"half_open_probes"`
	ProbeTTL       time.Duration `json:"probe_ttl"`
	// CrossPathNum / CrossPathDen 跨入口放大系数（分子/分母）。
	CrossPathNum int `json:"cross_path_num"`
	CrossPathDen int `json:"cross_path_den"`
	// PathFloor 窗口内每入口的免评笔数。占用表速度超过此值才折成信号，避免正常频次误判。
	PathFloor int `json:"path_floor"`
	// Matrix 阻断矩阵：入口 → 触发阻断的最低分档。缺项表示该入口不受本版策略拦截。
	Matrix map[Path]Severity `json:"matrix"`
	// ScopeGate 作用域开关：关掉的作用域即使跳闸也不拦截入口（仍留案件）。
	ScopeGate map[Scope]bool `json:"scope_gate"`
	// ScopeMinUsers 各作用域跳闸所需的去重用户数下限，防止单人刷量挡掉整条资产或链。
	ScopeMinUsers map[Scope]int `json:"scope_min_users"`
	// AutoUnfreeze 必须为 false。矩阵没有解冻动作，置 true 一律拒绝落版。
	AutoUnfreeze bool `json:"auto_unfreeze"`
	// EscalateHalt 达到 halt 档时是否升级到托管全局停充提。
	EscalateHalt bool `json:"escalate_halt"`
	Note      string    `json:"note,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DefaultPolicy 首版策略：三条入口在跳闸档统一挡新写，半开只放一枚探针。
  func DefaultPolicy() BlockPolicy {
	  return BlockPolicy{
		  Version: 1,
		   Window:  DefaultWindow,
		   SignalTTL: DefaultSignalTTL,
		  WatchScore: DefaultWatchScore,
		  WarnScore:  DefaultWarnScore,
		   TripScore:  DefaultTripScore,
		  HaltScore: DefaultHaltScore,
		  Cooldown: DefaultCooldown,
		  CooldownMax: DefaultCooldownMax,
		  HalfOpenProbes: DefaultHalfOpenProbes,
		   ProbeTTL:       DefaultProbeTTL,
		   CrossPathNum: DefaultCrossNum,
		  CrossPathDen: DefaultCrossDen,
		   PathFloor:    DefaultPathFloor,
		   Matrix: map[Path]Severity{
			  PathDepositObserve:  SevTrip,
			  PathWithdrawRequest: SevTrip,
			   PathTransferHop:     SevTrip,
		   },
		  ScopeGate: map[Scope]bool{
			  ScopeUser: true, ScopeAsset: true, ScopeChain: true, ScopeGlobal: true,
		  },
		   ScopeMinUsers: map[Scope]int{
			  ScopeUser: DefaultMinUsersUser, ScopeAsset: DefaultMinUsersAsset,
			   ScopeChain: DefaultMinUsersChain, ScopeGlobal: DefaultMinUsersGlobal,
		   },
		   AutoUnfreeze: false,
		  EscalateHalt: true,
		   Note:         "资金域统一限流熔断首版：dep/wd/hop 三入口跨路径联动",
	  }
  }

// withDefaults 补齐未配置字段，保证求值路径不依赖零值。
   func (p BlockPolicy) withDefaults() BlockPolicy {
	   if p.Window <= 0 {
		   p.Window = DefaultWindow
	   }
	  if p.SignalTTL <= 0 {
		  p.SignalTTL = DefaultSignalTTL
	  }
	  if p.WatchScore <= 0 {
		  p.WatchScore = DefaultWatchScore
	  }
	   if p.WarnScore <= 0 {
		  p.WarnScore = DefaultWarnScore
	   }
	   if p.TripScore <= 0 {
		   p.TripScore = DefaultTripScore
	  }
	   if p.HaltScore <= 0 {
		  p.HaltScore = DefaultHaltScore
	  }
	   if p.Cooldown <= 0 {
		  p.Cooldown = DefaultCooldown
	   }
	  if p.CooldownMax <= 0 {
		   p.CooldownMax = DefaultCooldownMax
	  }
	   if p.HalfOpenProbes <= 0 {
		   p.HalfOpenProbes = DefaultHalfOpenProbes
	   }
	  if p.ProbeTTL <= 0 {
		   p.ProbeTTL = DefaultProbeTTL
	   }
	  if p.CrossPathNum <= 0 || p.CrossPathDen <= 0 {
		  p.CrossPathNum, p.CrossPathDen = DefaultCrossNum, DefaultCrossDen
	   }
	   if p.PathFloor <= 0 {
		   p.PathFloor = DefaultPathFloor
	  }
	  if len(p.Matrix) == 0 {
		   p.Matrix = DefaultPolicy().Matrix
	  }
	   if len(p.ScopeGate) == 0 {
		  p.ScopeGate = DefaultPolicy().ScopeGate
	  }
	  if len(p.ScopeMinUsers) == 0 {
		   p.ScopeMinUsers = DefaultPolicy().ScopeMinUsers
	  }
	  return p
   }

// Validate 落版前校验。
// 功能：阈值递增、冷却退避合理、探针配额至少一枚、矩阵入口合法。
// 重点：AutoUnfreeze 为 true 直接返回 ErrUnfreezeForbidden——矩阵不允许出现解冻动作。
// 风险点：把跳闸阈值配得低于观察阈值会让熔断长期贴在拦截态，值班须核对读面分数。
  func (p BlockPolicy) Validate() error {
	  if p.AutoUnfreeze {
		   return ErrUnfreezeForbidden
	  }
	   q := p.withDefaults()
	  if !(q.WatchScore < q.WarnScore && q.WarnScore < q.TripScore && q.TripScore <= q.HaltScore) {
		  return ErrPolicy
	   }
	   if q.SignalTTL < q.Window {
		  return ErrPolicy
	  }
	  if q.CooldownMax < q.Cooldown {
		   return ErrPolicy
	  }
	  if q.HalfOpenProbes < 1 || q.HalfOpenProbes > 8 {
		  return ErrPolicy
	  }
	   if q.ProbeTTL <= 0 || q.ProbeTTL > q.CooldownMax {
		  return ErrPolicy
	   }
	   if q.CrossPathNum < q.CrossPathDen {
		  return ErrPolicy
	   }
	   if q.PathFloor < 1 || q.PathFloor > 1000 {
		  return ErrPolicy
	   }
	   for path, sev := range q.Matrix {
		  if !path.Valid() {
			  return ErrPath
		  }
		   if !sev.Valid() || sev.Rank() < SevWatch.Rank() {
			   return ErrSeverity
		  }
	   }
	   for scope := range q.ScopeGate {
		  if !scope.Valid() {
			   return ErrScope
		   }
	  }
	   for scope, n := range q.ScopeMinUsers {
		   if !scope.Valid() {
			  return ErrScope
		  }
		  if n < 1 {
			  return ErrPolicy
		  }
	   }
	   return nil
   }

// MinUsers 该作用域跳闸所需的去重用户数下限。未配置的作用域按 1 处理。

   func (p BlockPolicy) MinUsers(scope Scope) int {
	   if len(p.ScopeMinUsers) == 0 {
		   return 1
	   }
	   n, ok := p.ScopeMinUsers[scope]
	  if !ok || n < 1 {
		  return 1
	  }
	  return n
  }

// BlocksPath 指定分档下该入口是否被矩阵拦截。矩阵缺项视为不拦截。

   func (p BlockPolicy) BlocksPath(path Path, sev Severity) bool {
	  need, ok := p.Matrix[path]
	   if !ok {
		   return false
	  }
	  if !need.Valid() {
		  return false
	  }
	  return sev.Rank() >= need.Rank()
   }

// ScopeEnabled 该作用域是否参与拦截。未列出的作用域按开启处理。
   func (p BlockPolicy) ScopeEnabled(scope Scope) bool {
	  if len(p.ScopeGate) == 0 {
		   return true
	   }
	   on, ok := p.ScopeGate[scope]
	  if !ok {
		   return true
	   }
	  return on
  }

// CooldownFor 第 n 次跳闸的冷却时长。按 2 的幂退避，封顶 CooldownMax。
   func (p BlockPolicy) CooldownFor(trips int) time.Duration {
	   q := p.withDefaults()
	  if trips < 1 {
		   trips = 1
	  }
	  if trips > 16 {
		  trips = 16
	   }
	  d := q.Cooldown
	   for i := 1; i < trips; i++ {
		  d *= 2
		  if d >= q.CooldownMax {
			  return q.CooldownMax
		   }
	  }
	   if d > q.CooldownMax {
		   return q.CooldownMax
	   }
	   return d
  }

// MatrixRow 矩阵读面一行。

type MatrixRow struct {
	Path        Path     `json:"path"`
	MinSeverity Severity `json:"min_severity"`
	Blocks       bool   `json:"blocks_new_writes"`
	AutoUnfreeze bool   `json:"auto_unfreeze"`
	Note         string `json:"note,omitempty"`
}

// MatrixView 阻断矩阵读面。AutoUnfreeze 恒为 false，便于值班核对熔断。

   func (p BlockPolicy) MatrixView() []MatrixRow {
	   out := make([]MatrixRow, 0, len(Paths()))
	   for _, path := range Paths() {
		   sev, ok := p.Matrix[path]
		   row := MatrixRow{Path: path, MinSeverity: sev, Blocks: ok, AutoUnfreeze: false}
		  if !ok {
			  row.MinSeverity = SevNone
			   row.Note = "本版策略不拦截该入口"
		   }
		  out = append(out, row)
	  }
	   return out
   }

// PolicyRegistry 策略代次表。Current 为最新落版；旧版留档供回放。
type PolicyRegistry struct {
	mu sync.Mutex
	rows map[int]BlockPolicy
	cur  int
	clock func() time.Time
	onPut func(BlockPolicy)
}

// NewPolicyRegistry 建代次表并落首版默认策略。
   func NewPolicyRegistry() *PolicyRegistry {
	  r := &PolicyRegistry{
		  rows: map[int]BlockPolicy{}, clock: func() time.Time { return time.Now().UTC() },
	  }
	  doc := DefaultPolicy().withDefaults()
	  doc.UpdatedAt = r.clock()
	  r.rows[doc.Version] = doc
	   r.cur = doc.Version
	  return r
   }

// WithClock 注入时钟。

  func (r *PolicyRegistry) WithClock(c func() time.Time) *PolicyRegistry {
	  if r != nil && c != nil {
		  r.mu.Lock()
		  r.clock = c
		   r.mu.Unlock()
	   }
	   return r
   }

   // OnPut 落版回调，用于持久化。回调在锁外调用。
   func (r *PolicyRegistry) OnPut(fn func(BlockPolicy)) *PolicyRegistry {
	  if r != nil {
		   r.mu.Lock()
		  r.onPut = fn
		   r.mu.Unlock()
	   }
	   return r
   }

   // Current 当前策略。代次表为空时返回默认策略，避免闸门无策略可用。

  func (r *PolicyRegistry) Current() BlockPolicy {
	   if r == nil {
		   return DefaultPolicy().withDefaults()
	  }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	   doc, ok := r.rows[r.cur]
	  if !ok {
		   return DefaultPolicy().withDefaults()
	   }
	  return doc
  }

// Get 取指定代次。

  func (r *PolicyRegistry) Get(version int) (BlockPolicy, error) {
	   if r == nil {
		   return BlockPolicy{}, ErrPolicyVersion
	   }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  doc, ok := r.rows[version]
	   if !ok {
		   return BlockPolicy{}, ErrPolicyVersion
	   }
	   return doc, nil
  }

// Put 落新版策略。代次号由表递增，调用方给的 Version 只作留档参考。
// 功能：运营改窗口、阈值、冷却与矩阵后立即对新写生效，旧版留档可回放。
// 重点：actor 必填；校验不过不落版，闸门接着用上一版。
// 风险点：把入口从矩阵里摘掉等于放开该入口，须在 reason 写明依据。

   func (r *PolicyRegistry) Put(actor string, doc BlockPolicy) (BlockPolicy, error) {
	  if r == nil {
		   return BlockPolicy{}, ErrStore
	   }
	  actor = strings.TrimSpace(actor)
	   if actor == "" || strings.EqualFold(actor, "system") {
		   return BlockPolicy{}, ErrActor
	   }
	  doc = doc.withDefaults()
	  if err := doc.Validate(); err != nil {
		  return BlockPolicy{}, err
	   }
	  r.mu.Lock()
	  doc.Version = r.cur + 1
	  doc.Actor = actor
	   doc.UpdatedAt = r.clock()
	   r.rows[doc.Version] = doc
	  r.cur = doc.Version
	  fn := r.onPut
	  r.mu.Unlock()
	   if fn != nil {
		   fn(doc)
	  }
	   return doc, nil
  }

// Load 装回已落库代次（重启 Hydrate 用）。不做代次递增，但会推进 Current。
   func (r *PolicyRegistry) Load(doc BlockPolicy) error {
	   if r == nil {
		  return ErrStore
	   }
	   doc = doc.withDefaults()
	   if doc.Version <= 0 {
		   return ErrPolicyVersion
	  }
	   if err := doc.Validate(); err != nil {
		   return err
	  }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   r.rows[doc.Version] = doc
	  if doc.Version > r.cur {
		  r.cur = doc.Version
	   }
	  return nil
   }

// Versions 已落版代次号，升序。
   func (r *PolicyRegistry) Versions() []int {
	  if r == nil {
		  return nil
	  }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   out := make([]int, 0, len(r.rows))
	   for v := range r.rows {
		  out = append(out, v)
	  }
	   sort.Ints(out)
	  return out
   }
