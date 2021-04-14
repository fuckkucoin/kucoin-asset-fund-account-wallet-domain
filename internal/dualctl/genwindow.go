package dualctl

import (
	"fmt"
	"sync"
	"time"
)

// GenBinding 世代绑定记录：一个 Request 绑定到某次放行世代。

type GenBinding struct {
	RequestID string
	Gen int64
	BoundAt time.Time
	// LeaseToken 放行互斥租约 token，用于交叉校验。
	LeaseToken string
}

// GenRegistry 内存世代绑定注册表（线程安全）。
// 场景中，世代状态由 dual_release_leases 表持久化，此注册表作为进程内快速校验层。
type GenRegistry struct {
	mu sync.Mutex
	bindings map[string]*GenBinding // key: requestID
}

// NewGenRegistry 初始化世代注册表。
  func NewGenRegistry() *GenRegistry {
	   return &GenRegistry{bindings: map[string]*GenBinding{}}
  }

// Bind 将 requestID 绑定到 gen；重复绑定同 gen 幂等，绑定到更高 gen 则覆盖，旧 gen 返回 ErrStale。

   func (r *GenRegistry) Bind(requestID string, gen int64, leaseToken string) error {
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   cur, ok := r.bindings[requestID]
	  if ok {
		   if cur.Gen > gen {
			   return fmt.Errorf("dualctl gen registry: stale gen %d, current=%d", gen, cur.Gen)
		  }
	  }
	  r.bindings[requestID] = &GenBinding{
		  RequestID: requestID,
		  Gen:       gen,
		   BoundAt:    time.Now().UTC(),
		   LeaseToken: leaseToken,
	  }
	   return nil
   }

// Check 校验 requestID 当前世代与 gen 是否吻合。
   func (r *GenRegistry) Check(requestID string, gen int64) error {
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   cur, ok := r.bindings[requestID]
	  if !ok {
		   return fmt.Errorf("dualctl gen registry: no binding for %s", requestID)
	  }
	   if cur.Gen != gen {
		   return fmt.Errorf("%w: request %s expected gen %d got %d", ErrStale, requestID, cur.Gen, gen)
	   }
	   return nil
  }

// Current 返回 requestID 当前世代；未注册返回 0, false。
   func (r *GenRegistry) Current(requestID string) (int64, bool) {
	   r.mu.Lock()
	  defer r.mu.Unlock()
	  b, ok := r.bindings[requestID]
	   if !ok {
		  return 0, false
	   }
	  return b.Gen, true
  }

// ──────────────────────────────────────────────────
// TicketAlive 票证活跃性检查（只读）
// ──────────────────────────────────────────────────
// TicketAlive 检查 dualctl.Request 是否仍在 TTL 窗口内，不修改状态机。
// 调用方在 Execute/放行前调用此函数以提前拒绝过期票证。
   func TicketAlive(r *Request, now time.Time) bool {
	   if r == nil {
		  return false
	   }
	   return now.Before(r.ExpiresAt)
  }

// TicketAliveError 与 TicketAlive 相同，但过期时返回包含详情的错误（供放行引擎记录审计）。
   func TicketAliveError(r *Request, now time.Time) error {
	   if r == nil {
		  return fmt.Errorf("dualctl: request is nil")
	  }
	   if !now.Before(r.ExpiresAt) {
		   return fmt.Errorf("dualctl: ticket %s expired at %s (now=%s)", r.ID, r.ExpiresAt.Format(time.RFC3339), now.Format(time.RFC3339))
	  }
	   return nil
   }

// ──────────────────────────────────────────────────
// GenExecute 集成世代校验的 Execute 入口
// ──────────────────────────────────────────────────
// GenExecuteParams GenExecute 入参。

type GenExecuteParams struct {
	RequestID string
	Actor string
	Epoch int64
	Token string
	// Gen 调用方持有的世代号；校验后方可执行。
	Gen int64
	// Registry 世代注册表（可空；空则跳过进程内世代校验）。
	Registry *GenRegistry
}

// GenExecute 在口径 Execute 之前插入世代校验。
// 校验失败直接返回 ErrStale（等价于 dualrelease.ErrStaleGen），不调底层 Execute。

  func (b *Book) GenExecute(p GenExecuteParams) (*Request, error) {
	  if p.Registry != nil {
		   if err := p.Registry.Check(p.RequestID, p.Gen); err != nil {
			   return nil, err
		  }
	   }
	  return b.Execute(p.RequestID, p.Actor, p.Epoch, p.Token)
   }

// ──────────────────────────────────────────────────
// TicketSummary 票证概要（供放行 API 只读展示）
// ──────────────────────────────────────────────────
// TicketSummary 票证只读摘要，供放行 API 返回。

type TicketSummary struct {
	ID string `json:"id"`
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	State State `json:"state"`
	OpenedBy string `json:"opened_by"`
	Need     int    `json:"need"`
	AllowN   int    `json:"allow_n"`
	ExpiresAt time.Time `json:"expires_at"`
	Alive bool `json:"alive"`
}

// Summarize 从 dualctl.Request 生成只读摘要；now 用于计算 Alive。

  func Summarize(r *Request, now time.Time) TicketSummary {
	  return TicketSummary{
		   ID: r.ID,
		   Kind: r.Kind,
		  Subject: r.Subject,
		  State: r.State,
		  OpenedBy: r.OpenedBy,
		   Need: r.Need,
		  AllowN: AllowCount(*r),
		   ExpiresAt: r.ExpiresAt,
		   Alive: TicketAlive(r, now),
	  }
   }
