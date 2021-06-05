package fundcircuit

import (
	"fmt"
	"strings"
	"time"
)

// 在途冻结相位。与出金域的 FreezeState 取值一致，仅作只读，不反向改相位。

const (
	inflightScreening = "screening"
	inflightApproved  = "approved"
	inflightSigning    = "signing"
	inflightBroadcast  = "broadcast"
	inflightConfirming = "confirming"
	inflightSigned  = "signed"
	inflightReorged = "reorged"
	inflightConfirmed = "confirmed"
)

// InflightStates 受保护的在途相位清单，供读面展示熔断。

  func InflightStates() []string {
	  return []string{
		  inflightScreening, inflightApproved, inflightSigning,
		   inflightBroadcast, inflightConfirming, inflightSigned,
		   inflightReorged, inflightConfirmed,
	  }
  }

// protectedPhase 相位是否属于在途保护范围。
   func protectedPhase(phase string) bool {
	   phase = strings.ToLower(strings.TrimSpace(phase))
	  if phase == "" {
		  return false
	  }
	  for _, s := range InflightStates() {
		   if s == phase {
			   return true
		  }
	  }
	  return false
   }

// RefuseAutoUnfreeze 在途单解冻禁令。
// 功能：熔断跳闸、冷却推进、半开探针与解熔断都不得自动解冻在途单。
// 重点：筛查中、待签、签名中、已广播、等确认，以及已有链上 hash 的单一律拒绝。
// 风险点：跳闸时解冻已签名单会丢掉冻结，链上已出账而账面未扣，须人工核对后再处理。
  func RefuseAutoUnfreeze(ref InflightRef) error {
	   if protectedPhase(ref.FreezeState) {
		   return fmt.Errorf("%w: freeze=%s", ErrUnfreezeForbidden, strings.ToLower(strings.TrimSpace(ref.FreezeState)))
	   }
	  if protectedPhase(ref.Broadcast) {
		  return fmt.Errorf("%w: broadcast=%s", ErrUnfreezeForbidden, strings.ToLower(strings.TrimSpace(ref.Broadcast)))
	   }
	  if strings.TrimSpace(ref.TxHash) != "" {
		   return fmt.Errorf("%w: live hash", ErrUnfreezeForbidden)
	   }
	   return nil
   }

// UnfreezeAllowed 恒为 false：本总线没有解冻能力，读面据此展示。
   func UnfreezeAllowed() bool { return false }

// Decide 单个熔断器对某入口的决策。
// 功能：closed 放行；open 按矩阵拦截；half_open 仅凭有效探针放行。
// 重点：决策只读熔断器，不改状态；AutoUnfreeze 恒 false、InflightPreserved 恒 true。
// 风险点：矩阵摘掉入口后本函数会放行，回归夹具须覆盖矩阵缺项分支。

   func Decide(pol BlockPolicy, c Circuit, path Path, probe *Probe, now time.Time) Decision {
	   if now.IsZero() {
		   now = time.Now().UTC()
	   }
	  d := Decision{
		   Path: path, Allow: true, CircuitKey: c.Key, Scope: c.Scope, State: c.State,
		  Score: c.Score, Severity: c.Severity, PolicyVer: pol.Version, CaseID: c.CaseID,
		   AutoUnfreeze: false, InflightPreserved: true, At: now.UTC(),
	   }
	   if !path.Valid() {
		  d.Allow = false
		  d.Reason = ErrPath.Error()
		  return d
	  }
	   if !c.State.Blocking() {
		  return d
	   }
	  if !pol.ScopeEnabled(c.Scope) {
		  d.Reason = "scope gate off"
		  return d
	  }
	  if !pol.BlocksPath(path,
		  c.Severity) {
		  d.Reason = "matrix allows path at " + c.Severity.String()
		   return d
	   }
	   if c.State == StateHalfOpen {
		  if probe != nil && probe.Pending() && probe.Path == path && now.Before(probe.ExpiresAt) {
			  d.ProbeID = probe.ID
			   d.Reason = "half_open probe"
			   return d
		  }
		  d.Allow = false
		  d.ProbeID = ""
		   d.Reason = "half_open: probe required"
		  return d
	   }
	  d.Allow = false
	  d.Reason = blockReason(c, path)
	   return d
  }

   func blockReason(c Circuit, path Path) string {
	  reason := c.Reason
	   if reason == "" {
		   reason = "fund abuse circuit open"
	  }
	  return fmt.Sprintf("%s scope=%s target=%s path=%s severity=%s", reason, c.Scope, c.Target(), path, c.Severity)
   }

// BlockBoard 阻断读面：某目标在各入口上的当前结论。
type BlockBoard struct {
	UserID    string `json:"user_id,omitempty"`
	Chain     string `json:"chain,omitempty"`
	Asset     string `json:"asset,omitempty"`
	PolicyVer int    `json:"policy_ver"`
	Rows []BoardRow `json:"rows"`
	Matrix []MatrixRow `json:"matrix"`
	Inflight []string `json:"protected_inflight_states"`
	AutoUnfreeze bool      `json:"auto_unfreeze"`
	Keys         []string  `json:"keys"`
	At           time.Time `json:"at"`
	Decisions []Decision `json:"decisions,omitempty"`
	Circuits []Circuit `json:"circuits,omitempty"`
	Scopes     []Scope      `json:"scopes"`
	SourceKeys []SourceNote `json:"source_keys,omitempty"`
}

// BoardRow 读面一行：入口 → 是否被挡与命中的熔断键。
type BoardRow struct {
	Path    Path `json:"path"`
	Blocked bool `json:"blocked"`
	Reason string `json:"reason,omitempty"`
	CircuitKey string       `json:"circuit_key,omitempty"`
	Scope      Scope        `json:"scope,omitempty"`
	State      CircuitState `json:"state,omitempty"`
	Severity Severity `json:"severity,omitempty"`
	Score int `json:"score"`
}

// SourceNote 原域键共存说明。Mutated 恒为 false：总线不改写原键。
type SourceNote struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	Mutated bool   `json:"mutated"`
}
