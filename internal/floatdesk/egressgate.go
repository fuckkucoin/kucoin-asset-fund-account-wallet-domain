package floatdesk

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/validate"
)

// EgressAction 门控建议动作。

type EgressAction string

const (
	EgressAllow     EgressAction = "allow"
	EgressWarn      EgressAction = "warn"
	EgressDefer     EgressAction = "defer_batch"
	EgressHaltBatch EgressAction = "halt_batch"
)

// EgressGateEvent 门控事件。
// 调用方须尊重 Blocked 字段；Blocked=true 时不得接着构造出金批次。

type EgressGateEvent struct {
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	Blocked     bool        `json:"blocked"`
	BreachLevel BreachLevel `json:"breach_level"`
	BreachLabel string      `json:"breach_label"`
	Action  EgressAction `json:"action"`
	Deficit string       `json:"deficit,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// EgressGate 出金预检门控，内部缓存最新 BreachGrade 快照，线程安全。

type EgressGate struct {
	mu     sync.RWMutex
	grades map[string]BreachGrade // key = "CHAIN|NATIVE"
	clock  func() time.Time
}

// NewEgressGate 构造出金预检门控。
  func NewEgressGate() *EgressGate {
	  return &EgressGate{
		  grades: map[string]BreachGrade{},
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// Refresh 用最新的 BreachGrade 列表更新门控快照。
// 通常由定时任务在 GradeAll 后调用，保持门控决策与水位评估同步。
  func (eg *EgressGate) Refresh(grades []BreachGrade) {
	   if eg == nil {
		  return
	   }
	  next := make(map[string]BreachGrade, len(grades))
	  for _, g := range grades {
		  next[egressKey(g.Chain,
			   g.Native)] = g
	  }
	  eg.mu.Lock()
	  eg.grades = next
	  eg.mu.Unlock()
  }

// Check 对指定链的 gas 原生资产执行出金预检门控。
// 未登记 Tank 链视为 OK（不阻断）；手动停仓旗标先于水位评估。
  func (eg *EgressGate) Check(chain, asset string) EgressGateEvent {
	   if eg == nil {
		   return EgressGateEvent{Action: EgressAllow}
	  }
	  now := eg.clock()
	  ch := strings.ToUpper(strings.TrimSpace(chain))
	   as := strings.ToUpper(strings.TrimSpace(asset))
	   eg.mu.RLock()
	   grade, ok := eg.grades[egressKey(ch, as)]
	  eg.mu.RUnlock()
	   if !ok {
		  return EgressGateEvent{
			  Chain: ch, Asset: as,
			  Blocked: false, BreachLevel: BreachOK, BreachLabel: BreachOK.String(),
			   Action: EgressAllow, CheckedAt: now,
		  }
	  }
	  return buildEvent(ch, as, grade, now)
  }

// egressKey 生成 BreachGrade 的查找键。

   func egressKey(chain, native string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToUpper(strings.TrimSpace(native))
  }

// buildEvent 从 BreachGrade 构造 EgressGateEvent。
   func buildEvent(chain, asset string, grade BreachGrade, now time.Time) EgressGateEvent {
	   evt := EgressGateEvent{
		   Chain: chain,
		  Asset: asset,
		   BreachLevel: grade.Level,
		  BreachLabel: grade.Label,
		  Deficit:     grade.Deficit,
		   CheckedAt: now,
	  }
	  switch grade.Level {
	   case BreachHalt:
		   evt.Blocked = true
		  evt.Action = EgressHaltBatch
		  metrics.EgressGateBlocked.WithLabelValues(chain, asset, grade.Label).Inc()
	   case BreachCrit:
		   evt.Blocked = true
		  evt.Action = EgressDefer
		   metrics.EgressGateBlocked.WithLabelValues(chain,
			   asset, grade.Label).Inc()
	  case BreachWarn:
		  evt.Blocked = false
		   evt.Action = EgressWarn
	   default:
		  evt.Blocked = false
		  evt.Action = EgressAllow
	  }
	  return evt
  }

// CheckEgress 无状态版：直接从 grades 切片和链/资产名称判断门控事件。
// 匹配规则：在 grades 中查找 chain 相同、native == asset 的条目。

  func CheckEgress(grades []BreachGrade, chain, asset string, now time.Time) EgressGateEvent {
	   if now.IsZero() {
		  now = time.Now().UTC()
	  }
	   ch, err := validate.Chain(chain)
	   if err != nil {
		  ch = strings.ToUpper(strings.TrimSpace(chain))
	  }
	   as, err := validate.Asset(asset)
	   if err != nil {
		  as = strings.ToUpper(strings.TrimSpace(asset))
	  }
	   for i := range grades {
		  g := &grades[i]
		  if g.Chain == ch && g.Native == as {
			   return buildEvent(ch, as, *g, now)
		   }
	  }
	  return EgressGateEvent{
		  Chain: ch, Asset: as,
		  Blocked:     false,
		   BreachLevel: BreachOK, BreachLabel: BreachOK.String(),
		  Action: EgressAllow, CheckedAt: now,
	   }
   }

// CheckEgressFromTanks 从 Tanks+Flags 实时计算门控事件，免去外部 GradeAll。
// 手动停仓旗标（Flags.Check 失败）直接返回 Halt 事件，先于水位计算。
  func CheckEgressFromTanks(tanks *Tanks, halt *Flags, chain, asset string, now time.Time) (EgressGateEvent, error) {
	   if now.IsZero() {
		  now = time.Now().UTC()
	   }
	   ch, err := validate.Chain(chain)
	  if err != nil {
		   return EgressGateEvent{}, ErrChain
	  }
	  as, err := validate.Asset(asset)
	   if err != nil {
		   return EgressGateEvent{}, ErrAsset
	   }
	   // 手动停仓先检查。
	  if halt != nil {
		   if herr := halt.Check(ch, as); herr != nil {
			   evt := EgressGateEvent{
				  Chain: ch, Asset: as,
				   Blocked: true, BreachLevel: BreachHalt, BreachLabel: BreachHalt.String(),
				  Action:    EgressHaltBatch,
				  CheckedAt: now,
			  }
			  metrics.EgressGateBlocked.WithLabelValues(ch, as, BreachHalt.String()).Inc()
			  return evt, nil
		  }
	   }
	  // gas 仓水位评估。
	  t, err := tanks.Get(ch)
	  if err != nil {
		  // 未登记 Tank → 放行。
		  return EgressGateEvent{
			  Chain: ch, Asset: as,
			  Blocked: false, BreachLevel: BreachOK, BreachLabel: BreachOK.String(),
			  Action: EgressAllow, CheckedAt: now,
		  }, nil
	   }
	   grade, err := GradeTank(t, now)
	   if err != nil {
		  return EgressGateEvent{}, err
	  }
	  return buildEvent(ch, as, grade, now), nil
   }
