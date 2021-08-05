package keyrotatefence

import "time"

// Phase 失效窗口相位。合法转换：armed → cut → sealed；任意非终态可进入 blocked。

type Phase string

const (
	PhaseArmed Phase = "armed"
	PhaseCut Phase = "cut"
	PhaseSealed  Phase = "sealed"
	PhaseBlocked Phase = "blocked"
)

// Window 一次轮换工单对应的旧钥失效窗口。
type Window struct {
	TicketID string `json:"ticket_id"`
	IdemKey  string `json:"idem_key"`
	OldKeyID string `json:"old_key_id"`
	NewKeyID string `json:"new_key_id"`
	OldFingerprint string `json:"old_fingerprint"`
	NewFingerprint string `json:"new_fingerprint,omitempty"`
	Phase          Phase  `json:"phase"`
	AuthKind string `json:"auth_kind,omitempty"`
	Profile  string `json:"profile,omitempty"`
	KeySource string `json:"key_source,omitempty"`
	NewKeyPath  string `json:"new_key_path,omitempty"`
	OpenedBy    string `json:"opened_by"`
	BlockReason string `json:"block_reason,omitempty"`
	OpenedAt time.Time `json:"opened_at"`
	CutAt    time.Time `json:"cut_at,omitempty"`
	SealedAt time.Time `json:"sealed_at,omitempty"`
	BlockedAt time.Time `json:"blocked_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

  func (w *Window) clone() *Window {
	  if w == nil {
		  return nil
	  }
	  cp := *w
	   return &cp
  }

   func (w *Window) terminal() bool {
	  if w == nil {
		  return false
	  }
	   return w.Phase == PhaseSealed || w.Phase == PhaseBlocked
   }

// ArmReq 打开失效窗口。同 ticket 重入须字段一致。
type ArmReq struct {
	TicketID string
	IdemKey  string
	OldKeyID string
	NewKeyID string
	OldFingerprint string
	AuthKind       string
	Profile    string
	KeySource  string
	NewKeyPath string
	OpenedBy   string
}

// CutReq 切旧：旧钥立即失效。新钥材料未就绪则进入 blocked。
type CutReq struct {
	TicketID string
	Actor string
}

// SealReq 封窗：新钥指纹必须与工单一致，禁止材料。
type SealReq struct {
	TicketID string
	NewFingerprint string
	Actor          string
}

// UseReq 签名/HMAC 材料断言。

type UseReq struct {
	TicketID string
	KeyID    string
	KeyPath  string
	Source string
	AuthKind string
	Profile  string
}
