package xferloop

import "time"

const (
	actionDrive = "drive"
	actionEscalate = "escalate"
	actionWait      = "wait"
	actionRecovered = "recovered"
	actionSkip      = "skip"
)

// Policy 一种半截单的自动收口策略。
type Policy struct {
	Name        string        `json:"name"`
	Kind        Kind          `json:"kind"`
	MaxAttempts int           `json:"max_attempts"`
	MinAge      time.Duration `json:"min_age"`
	SLA         time.Duration `json:"sla"`
	MinBeforeSLA int `json:"min_before_sla"`
	Severity    string `json:"severity"`
	AutoRecover bool   `json:"auto_recover"`
	// CompensateOnly 为真时，aborting 相位只允许补偿，不允许当成正向 hopped 去冲 overlay。
	CompensateOnly bool `json:"compensate_only"`
}

// Policies 当前生效的收口策略。工作台按种类取用，不另存一份空表。
  func Policies() []Policy {
	   return []Policy{policyOf(KindHalfHop), policyOf(KindInner), policyOf(KindCover)}
   }

  func policyOf(kind Kind) Policy {
	  switch kind {
	  case KindHalfHop:
		   return Policy{
			  Name: "hop-forward-or-compensate", Kind: kind,
			  MaxAttempts: 3,
			   MinAge: 14 * time.Second, SLA: 2 * time.Minute,
			  MinBeforeSLA: 1, Severity: "high", AutoRecover: true, CompensateOnly: true,
		  }
	   case KindInner:
		   return Policy{
			  Name: "inner-resume-once", Kind: kind,
			   MaxAttempts: 3, MinAge: 14 * time.Second, SLA: 90 * time.Second,
			  MinBeforeSLA: 1, Severity: "high", AutoRecover: true,
		   }
	  case KindCover:
		  return Policy{
			  Name: "cover-sweep", Kind: kind,
			  MaxAttempts: 3, MinAge: 14 * time.Second, SLA: 2 * time.Minute,
			   MinBeforeSLA: 1, Severity: "critical", AutoRecover: true,
		  }
	  default:
		  return Policy{
			   Name: "unknown", Kind: kind, MaxAttempts: 1, MinAge: 14 * time.Second,
			  SLA: time.Minute, MinBeforeSLA: 1, Severity: "high",
		   }
	  }
  }

   func applyPolicy(c *Case, now time.Time) {
	  if c == nil {
		   return
	  }
	  if now.IsZero() {
		   now = time.Now().UTC()
	   }
	  pol := policyOf(c.Kind)
	  c.Policy = pol.Name
	  c.Severity = pol.Severity
	  if c.DueAt.IsZero() {
		  base := c.SeenAt
		   if base.IsZero() {
			  base = now
		   }
		   c.DueAt = base.Add(pol.SLA)
	  }
	  c.SLABreach = !c.DueAt.IsZero() && now.After(c.DueAt)
  }

  func reopenSLA(c *Case, now time.Time) {
	  if c == nil {
		   return
	   }
	  if now.IsZero() {
		   now = time.Now().UTC()
	  }
	  pol := policyOf(c.Kind)
	  c.Policy = pol.Name
	   c.Severity = pol.Severity
	  c.DueAt = now.Add(pol.SLA)
	   c.SLABreach = false
   }

// slaEscalate 时限已过且已经自动尝试过，才升级。第一次尝试不因历史账龄被跳过。

   func slaEscalate(c *Case) bool {
	   if c == nil || !c.SLABreach || c.State == StateEscalated || c.State == StateRecovered {
		  return false
	   }
	   return c.Attempts >= policyOf(c.Kind).MinBeforeSLA
   }
