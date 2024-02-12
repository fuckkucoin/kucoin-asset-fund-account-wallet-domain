package airgap

import "time"

// QuorumBind 法定人数绑定世代记录。

type QuorumBind struct {
	BindID string `json:"bind_id"` // UUID
	SessionID string `json:"session_id"`
	LeaseID string `json:"lease_id"`
	BindGen string `json:"bind_gen"` // 同 air_import_gen
	Threshold int `json:"threshold"` // Session.Threshold 快照
	ActualQuorum int  `json:"actual_quorum"` // 实际不同签名人数
	QuorumPassed bool `json:"quorum_passed"` // actual_quorum >= threshold
	BoundBy string `json:"bound_by"` // actor
	BoundAt time.Time `json:"bound_at"`
	CreatedAt time.Time `json:"created_at"`
}

// Copy 深拷贝 QuorumBind。

  func (q QuorumBind) Copy() QuorumBind {
	   return q
  }

// CheckBind 验证法定人数是否满足，不满足返回 ErrQuorum。

   func CheckBind(actual, threshold int) error {
	   if actual < threshold {
		   return ErrQuorum
	   }
	  return nil
  }
