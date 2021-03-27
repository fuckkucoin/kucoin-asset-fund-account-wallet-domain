package creditoutgate

import (
	"sync"
	"time"
)

// CreditOutboxReader fundsafety CreditOutbox 只读接口。
// 本包只消费 CreditOutbox 数据，不回写 fundsafety 内部状态。
type CreditOutboxReader interface {
	// CreditByKey 按 credit_key 读取出箱记录（只读）。
	// 返回 (key, epoch, token, amount, sentAt, ok)。
	CreditByKey(key string) (epoch int64, token string, amount string, delivered bool, ok bool)
}

// GenReader 供外部包只读消费本包世代视图的接口（credit_deliver_gen 公开接口）。

type GenReader interface {
	// GenOf 读取指定板卡的当前世代快照。
	GenOf(boardKey string) (GenView, bool)
	// IsGenValid 检查给定的 world_gen/lane_gen 是否仍等于当前世代（未被翻转）。
	IsGenValid(boardKey string, worldGen, laneGen int64) bool
}

// Service 贷记出箱投递围栏世代服务。
// 线程安全；所有公开方法均持 mu 操作内存状态。
type Service struct {
	mu sync.Mutex
	// gens 板卡世代索引：boardKey → BoardGeneration。
	gens map[string]*BoardGeneration
	// leases 投递租约索引：credit_key → CreditDeliverLease。
	leases map[string]*CreditDeliverLease
	// proofs 投递证明：credit_key → CreditProof（仅已 delivered 的）。
	proofs map[string]*CreditProof
	// cases 事故证明（只追加）：id → CreditFenceCase。
	cases map[string]*CreditFenceCase
	// casesByKey 按 credit_key 的事故证明索引（快速查重）。
	casesByKey map[string][]*CreditFenceCase
	// casesByBoard 按 board_key 的事故证明索引。
	casesByBoard map[string][]*CreditFenceCase
	// outbox 外部 CreditOutbox 只读口（可为 nil，时内联）。
	outbox CreditOutboxReader
	// clock 时钟函数，可替换。
	clock func() time.Time
	// seq 单调 ID 序号（用于生成伪 UUID）。
	seq uint64
}

// New 创建服务实例。
  func New() *Service {
	   return &Service{
		  gens:   map[string]*BoardGeneration{},
		  leases: map[string]*CreditDeliverLease{},
		  proofs: map[string]*CreditProof{},
		   cases:      map[string]*CreditFenceCase{},
		  casesByKey: map[string][]*CreditFenceCase{},
		  casesByBoard: map[string][]*CreditFenceCase{},
		   clock:        func() time.Time { return time.Now().UTC() },
	   }
   }

// BindOutbox 注入 CreditOutbox 只读口（可选；未注入时 Deliver 跳过外部围栏复核）。

   func (s *Service) BindOutbox(r CreditOutboxReader) *Service {
	  if s != nil {
		  s.outbox = r
	   }
	  return s
   }

// WithClock 替换时钟（仅用途）。
  func (s *Service) WithClock(fn func() time.Time) *Service {
	  if s != nil && fn != nil {
		  s.clock = fn
	   }
	   return s
  }

  // now 返回当前时刻（UTC）。

   func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		  return time.Now().UTC()
	   }
	   return s.clock()
  }

// genID 生成单调递增的伪 UUID 字符串（无外部依赖，可预测）。
   func (s *Service) genID(prefix string) string {
	   s.seq++
	  return genIDFromSeq(prefix, s.seq)
  }

// GenOf 读取指定板卡的当前世代快照（实现 GenReader）。
  func (s *Service) GenOf(boardKey string) (GenView, bool) {
	   if s == nil {
		  return GenView{}, false
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   bg, ok := s.gens[boardKey]
	  if !ok {
		  return GenView{}, false
	  }
	  return GenView{
		  BoardKey: bg.BoardKey,
		  WorldGen: bg.WorldGen,
		  LaneGen: bg.LaneGen,
		  Closed: bg.Closed,
		   AsOf:   bg.UpdatedAt,
	  }, true
   }

// IsGenValid 检查给定的 world_gen/lane_gen 是否仍与当前板卡世代一致（实现 GenReader）。

  func (s *Service) IsGenValid(boardKey string, worldGen, laneGen int64) bool {
	  if s == nil {
		   return false
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   bg, ok := s.gens[boardKey]
	  if !ok {
		  return false
	  }
	  return bg.WorldGen == worldGen && bg.LaneGen == laneGen && !bg.Closed
   }

// initBoardLocked 初始化板卡世代（持 mu 调用）。若已存在则返回现有值。

  func (s *Service) initBoardLocked(boardKey string) *BoardGeneration {
	   if bg, ok := s.gens[boardKey]; ok {
		   return bg
	   }
	  now := s.now()
	   bg := &BoardGeneration{
		  BoardKey: boardKey,
		  WorldGen: 1,
		   LaneGen: 1,
		   Closed: false,
		  UpdatedAt: now,
	  }
	  s.gens[boardKey] = bg
	   return bg
  }

// appendCaseLocked 追加事故证明（持 mu 调用）。

   func (s *Service) appendCaseLocked(c *CreditFenceCase) {
	   s.cases[c.ID] = c
	   s.casesByKey[c.CreditKey] = append(s.casesByKey[c.CreditKey], c)
	  s.casesByBoard[c.BoardKey] = append(s.casesByBoard[c.BoardKey], c)
   }

// ActiveLeases 返回所有非终态（enqueued/delivering）的租约快照（与运维用）。

   func (s *Service) ActiveLeases() []CreditDeliverLease {
	   if s == nil {
		  return nil
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   var out []CreditDeliverLease
	   for _, l := range s.leases {
		   if l.State == LeaseEnqueued || l.State == LeaseDelivering {
			   out = append(out, *l)
		  }
	   }
	   return out
   }

// AllCases 返回全部事故证明快照（与运维用）。
   func (s *Service) AllCases() []CreditFenceCase {
	   if s == nil {
		  return nil
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]CreditFenceCase, 0, len(s.cases))
	  for _, c := range s.cases {
		  out = append(out, *c)
	  }
	   return out
  }

// AllProofs 返回全部投递证明快照（与运维用）。

  func (s *Service) AllProofs() []CreditProof {
	   if s == nil {
		   return nil
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]CreditProof, 0, len(s.proofs))
	   for _, p := range s.proofs {
		   out = append(out, *p)
	  }
	  return out
  }

// LeaseByKey 按 credit_key 读取租约快照（只读）。
  func (s *Service) LeaseByKey(creditKey string) (CreditDeliverLease, bool) {
	   if s == nil {
		   return CreditDeliverLease{}, false
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   l, ok := s.leases[creditKey]
	  if !ok {
		   return CreditDeliverLease{}, false
	  }
	  return *l, true
   }

// ProofByKey 按 credit_key 读取投递证明（只读）。

   func (s *Service) ProofByKey(creditKey string) (CreditProof, bool) {
	  if s == nil {
		   return CreditProof{}, false
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  p, ok := s.proofs[creditKey]
	   if !ok {
		   return CreditProof{}, false
	   }
	  return *p, true
   }

// CasesByKey 按 credit_key 读取全部事故证明（只读）。

   func (s *Service) CasesByKey(creditKey string) []CreditFenceCase {
	  if s == nil {
		  return nil
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   cs := s.casesByKey[creditKey]
	   out := make([]CreditFenceCase, len(cs))
	   for i, c := range cs {
		  out[i] = *c
	   }
	   return out
  }

// CasesByBoard 按 board_key 读取全部事故证明（只读）。

  func (s *Service) CasesByBoard(boardKey string) []CreditFenceCase {
	  if s == nil {
		  return nil
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   cs := s.casesByBoard[boardKey]
	  out := make([]CreditFenceCase, len(cs))
	   for i, c := range cs {
		  out[i] = *c
	  }
	   return out
  }
