package opsaudit

import "time"

const (
	KindKeyRotateArm = "key_rotate_arm"
	KindKeyRotateCut = "key_rotate_cut"
	KindKeyRotateSeal = "key_rotate_seal"
	KindKeyRotateBlock = "key_rotate_block"
	KindKeyAssertDeny  = "key_assert_deny"
	KindHMACRefuse     = "hmac_refuse"
	KindDemoKeyRefuse = "demo_key_refuse"
	KindOpsSensitive = "ops_sensitive"
)

// RotateRequired 一次轮换工单封窗后必须齐备的种类顺序。
   func RotateRequired() []string {
	  return []string{KindKeyRotateArm, KindKeyRotateCut, KindKeyRotateSeal}
  }

// Record 单条追加记录。
type Record struct {
	Seq int64 `json:"seq"`
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Actor    string `json:"actor"`
	Subject  string `json:"subject,omitempty"`
	TicketID string `json:"ticket_id,omitempty"`
	IdemKey  string `json:"idem_key,omitempty"`
	Body     string `json:"body"`
	PrevHash string `json:"prev_hash"`
	Hash string `json:"hash"`
	At time.Time `json:"at"`
}

   func (r Record) clone() Record {
	  return r
  }

// AppendReq 追加请求。IdemKey 非空时同键重入须字段一致。
type AppendReq struct {
	Kind  string
	Actor string
	Subject string
	TicketID string
	IdemKey string
	Body    string
}

// Completeness 工单种类齐备性。
type Completeness struct {
	TicketID string `json:"ticket_id"`
	Required []string `json:"required"`
	Present []string `json:"present"`
	OK      bool     `json:"ok"`
}
