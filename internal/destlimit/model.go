package destlimit

import "time"

const (
	HoldReserved = "reserved"
	HoldConsumed = "consumed"
	HoldReleased = "released"
	HoldExpired  = "expired"
	OvPending = "pending"
	OvActive  = "active"
	OvExpired = "expired"
	OvRevoked = "revoked"
	AuditReserve = "hold_reserve"
	AuditConsume = "hold_consume"
	AuditRelease = "hold_release"
	AuditExpire = "hold_expire"
	AuditDeny   = "limit_deny"
	AuditPolicy = "policy_put"
	AuditReload  = "policy_reload"
	AuditPropose = "override_propose"
	AuditConfirm = "override_confirm"
	AuditOvExpire = "override_expire"
	SignalOverLimit = "over_limit"
)

// Policy 用户×资产×簇的动态限额策略。乘数按风险档缩放簇/用户/地址三道上限。
type Policy struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Asset      string `json:"asset"`
	ClusterMax string `json:"cluster_max"`
	UserMax    string `json:"user_max"`
	DestMax   string `json:"dest_max"`
	WindowSec int64  `json:"window_sec"`
	LowMul    string `json:"low_mul"`
	MidMul  string `json:"mid_mul"`
	HighMul string `json:"high_mul"`
	SevereMul string `json:"severe_mul"`
	Version int64 `json:"version"`
	Enabled bool  `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Hold 一笔出金对限额的占用。Request 成功后保持 reserved，终态再消耗或释放。
type Hold struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Chain     string `json:"chain"`
	Dest      string `json:"dest"`
	ClusterID string `json:"cluster_id"`
	Amount    string `json:"amount"`
	PolicyID string `json:"policy_id"`
	Tier      string `json:"tier"`
	State     string `json:"state"`
	RequestID string `json:"request_id"`
	Limit     string `json:"limit"`
	Reason    string `json:"reason,omitempty"`
	ReservedAt time.Time `json:"reserved_at"`
	Deadline   time.Time `json:"deadline"`
	ClosedAt   time.Time `json:"closed_at,omitempty"`
	// Reused 为真表示本次返回的是已有占用，调用方不得在后续失败时把它释放掉。
	Reused bool `json:"-"`
}

// Override 桌面覆写。ActorB 为空表示尚未双人确认。

type Override struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Asset     string `json:"asset"`
	ClusterID string `json:"cluster_id,omitempty"`
	Extra     string    `json:"extra"`
	ActorA    string    `json:"actor_a"`
	ActorB    string    `json:"actor_b,omitempty"`
	Reason    string    `json:"reason"`
	State     string    `json:"state"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	ConfirmedAt time.Time `json:"confirmed_at,omitempty"`
}

// Audit 限额侧审计。与图谱衰减审计分开存放，便于按种类回放。

type Audit struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Actor  string `json:"actor,omitempty"`
	Detail string `json:"detail,omitempty"`
	At time.Time `json:"at"`
}

// Signal 超限信号。只广播事实，不实现熔断状态机。
type Signal struct {
	Kind   string `json:"kind"`
	Code   string `json:"code"`
	UserID string `json:"user_id"`
	Asset     string `json:"asset"`
	ClusterID string `json:"cluster_id,omitempty"`
	Amount string    `json:"amount,omitempty"`
	Limit  string    `json:"limit,omitempty"`
	At     time.Time `json:"at"`
}

// AdmitReq 预检入参。KYTHint 只作边特征，空表示不改节点特征。

type AdmitReq struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Chain  string `json:"chain"`
	Dest   string `json:"dest"`
	Idem    string `json:"idempotency_key"`
	KYTHint string `json:"kyt_hint,omitempty"`
}

// Quote 一次求值的限额与已占用，便于审计与接口回显。

type Quote struct {
	PolicyID string `json:"policy_id"`
	Tier string `json:"tier"`
	Score        string `json:"score"`
	ClusterID    string `json:"cluster_id"`
	Multiplier   string `json:"multiplier"`
	ClusterLimit string `json:"cluster_limit"`
	UserLimit string `json:"user_limit"`
	DestLimit string `json:"dest_limit"`
	ClusterUsed string `json:"cluster_used"`
	UserUsed    string `json:"user_used"`
	DestUsed    string `json:"dest_used"`
	Extra       string `json:"extra"`
	OverrideID  string `json:"override_id,omitempty"`
}

  func (p Policy) mul(tier string) string {
	  switch tier {
	  case "mid":
		  return p.MidMul
	  case "high":
		  return p.HighMul
	   case "severe":
		  return p.SevereMul
	   default:
		  return p.LowMul
	  }
  }

   func copyHold(h *Hold) Hold {
	  if h == nil {
		  return Hold{}
	  }
	   return *h
  }

  func copyPolicy(p *Policy) Policy {
	  if p == nil {
		  return Policy{}
	   }
	   return *p
   }

   func copyOverride(o *Override) Override {
	   if o == nil {
		   return Override{}
	   }
	  return *o
   }
