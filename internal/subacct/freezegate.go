package subacct

import (
	"account-assets.kucoin.net/funding/pkg/money"
	"strings"
)

const (
	gateDebit      = "debit"
	gateCredit     = "credit"
	gateHopOut     = "hop_out"
	gateHopIn      = "hop_in"
	gatePrivAdd    = "priv_add"
	gatePrivRemove = "priv_remove"
	gateQuotaRaise = "quota_raise"
	gateQuotaLower = "quota_lower"
)

// GateDecision 一次冻结闸预览或实际拒绝。
type GateDecision struct {
	OwnerID  string `json:"owner_id"`
	ParentID string `json:"parent_id,omitempty"`
	ChildID  string `json:"child_id,omitempty"`
	Action   string `json:"action"`
	Allow    bool   `json:"allow"`
	Frozen   bool   `json:"frozen"`
	Reason   string `json:"reason,omitempty"`
	Status   string `json:"status,omitempty"`
}

// FreezeActions 闸门动作表，供读面。判定与 PreviewFreeze 使用同一套动作。
func FreezeActions() []GateDecision {
	return []GateDecision{
		{Action: gateDebit, Allow: false, Reason: "frozen account cannot debit"},
		{Action: gateCredit, Allow: true, Reason: "frozen account may receive"},
		{Action: gateHopOut, Allow: false, Reason: "frozen account cannot hop out"},
		{Action: gateHopIn, Allow: true, Reason: "frozen account may receive hop"},
		{Action: gatePrivAdd, Allow: false, Reason: "frozen account cannot gain privileges"},
		{Action: gatePrivRemove, Allow: true, Reason: "frozen account may lose privileges"},
		{Action: gateQuotaRaise, Allow: false, Reason: "frozen parent cannot raise transfer quota"},
		{Action: gateQuotaLower, Allow: true, Reason: "frozen parent may lower transfer quota"},
	}
}

// PreviewFreeze 不改状态，只回答该主体此刻能否做这个动作。

func (s *Service) PreviewFreeze(owner, action string) (GateDecision, error) {
	if s == nil {
		return GateDecision{}, ErrUnavailable
	}
	if err := s.ensureHydrated(); err != nil {
		return GateDecision{}, err
	}
	owner = strings.TrimSpace(owner)
	action = strings.ToLower(strings.TrimSpace(action))
	if owner == "" || action == "" {
		return GateDecision{}, ErrUser
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.decideLocked(owner, action), nil
}

func (s *Service) decideLocked(owner, action string) GateDecision {
	d := GateDecision{OwnerID: owner, Action: action, Allow: true, Reason: "active"}
	p, c, err := s.parentOfLocked(owner)
	if err != nil {
		d.Allow = false
		d.Reason = err.Error()
		return d
	}
	if p != nil {
		d.ParentID = p.ID
		d.Status = string(p.Status)
	}
	frozen := false
	if c != nil {
		d.ChildID = c.ID
		d.Status = string(c.Status)
		if c.Status == StatusFrozen || c.Status == StatusClosed {
			frozen = c.Status == StatusFrozen
			if c.Status == StatusClosed {
				d.Allow = false
				d.Frozen = false
				d.Reason = "closed"
				return d
			}
		}
	}
	if p != nil && (p.Status == StatusFrozen || p.Status == StatusClosed) {
		if p.Status == StatusClosed {
			d.Allow = false
			d.Reason = "closed"
			return d
		}
		frozen = true
	}
	d.Frozen = frozen
	if !frozen {
		return d
	}
	switch action {
	case gateCredit, gateHopIn, gatePrivRemove, gateQuotaLower:
		d.Allow = true
		d.Reason = "frozen account may receive or narrow"
	case gateDebit:
		d.Allow = false
		d.Reason = "frozen account cannot debit"
	case gateHopOut:
		d.Allow = false
		d.Reason = "frozen account cannot hop out"
	case gatePrivAdd, gateQuotaRaise:
		d.Allow = false
		d.Reason = "frozen account cannot gain privileges"
	default:
		d.Allow = false
		d.Reason = "unknown freeze action"
	}
	return d
}

func (s *Service) rejectFrozenLocked(owner, action, reason string) {
	if s == nil {
		return
	}
	d := s.decideLocked(owner, action)
	if reason == "" {
		reason = d.Reason
	}
	s.recordFreezeLocked(FreezeAudit{
		Action: "freeze_gate_rejected", ParentID: d.ParentID, ChildID: d.ChildID,
		Detail: action + ": " + reason,
	})
}

// noteCreditFrozen 冻结期间入账留痕，不改变余额结果。
func (s *Service) noteCreditFrozen(owner, detail string) {
	if s == nil {
		return
	}
	d := s.decideLocked(owner, gateCredit)
	if !d.Frozen {
		return
	}
	if strings.TrimSpace(detail) == "" {
		detail = "credit allowed while frozen"
	}
	s.recordFreezeLocked(FreezeAudit{
		Action: "credit_while_frozen", ParentID: d.ParentID, ChildID: d.ChildID,
		Detail: detail,
	})
}

// refuseQuotaRaise 母户冻结时拒绝提高该资产的日限额或单笔限额。降低允许。
func (s *Service) refuseQuotaRaise(parentID, asset, daily, perTx string) error {
	if s == nil {
		return ErrUnavailable
	}
	parentID = strings.TrimSpace(parentID)
	asset = strings.ToUpper(strings.TrimSpace(asset))
	if err := s.ensureHydrated(); err != nil {
		return err
	}
	nextDaily, err := money.Parse(daily)
	if err != nil {
		return ErrAmount
	}
	nextPer, err := money.Parse(perTx)
	if err != nil {
		return ErrAmount
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.parents[parentID]
	if p == nil || p.Status != StatusFrozen {
		return nil
	}
	oldDaily, oldPer := s.quotaPairLocked(parentID, asset)
	if nextDaily.Cmp(oldDaily) > 0 || nextPer.Cmp(oldPer) > 0 {
		s.recordFreezeLocked(FreezeAudit{
			Action: "freeze_gate_rejected", ParentID: p.ID,
			Detail: "quota_raise: frozen parent cannot raise transfer quota",
		})
		return ErrFrozen
	}
	return nil
}

func (s *Service) quotaPairLocked(parentID, asset string) (money.Amount, money.Amount) {
	q := s.quotas()
	q.mu.Lock()
	defer q.mu.Unlock()
	row := q.rows[quotaKey(parentID, asset)]
	if row == nil {
		return money.Zero(), money.Zero()
	}
	return parseOrZero(row.DailyLimit),
		parseOrZero(row.PerTxLimit)
}

func parseOrZero(raw string) money.Amount {
	a, err := money.Parse(raw)
	if err != nil {
		return money.Zero()
	}
	return a
}
