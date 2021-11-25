package subacct

import (
	"account-assets.kucoin.net/funding/internal/persist"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"time"
)

const kindFreezeAudit = "subacct.freeze_audit"

// FreezeAudit 一次冻解冻闸门记录。Gate 只取 cascade，与用户控制位、科目冻分开。
type FreezeAudit struct {
	ID       string    `json:"id"`
	At       time.Time `json:"at"`
	Actor    string    `json:"actor"`
	Action   string    `json:"action"`
	Gate     string    `json:"gate"`
	ParentID string    `json:"parent_id,omitempty"`
	ChildID  string    `json:"child_id,omitempty"`
	Children []string  `json:"children,omitempty"`
	Detail   string    `json:"detail,omitempty"`
}

func (s *Service) recordFreezeLocked(ev FreezeAudit) {
	if s == nil {
		return
	}
	if ev.ID == "" {
		ev.ID = uuid.NewString()
	}
	if ev.At.IsZero() {
		ev.At = s.now()
	}
	if ev.Gate == "" {
		ev.Gate = "cascade"
	}
	if ev.Actor == "" {
		ev.Actor = "ops"
	}
	s.audits = append(s.audits, ev)
	if s.kv == nil {
		return
	}
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	_ = s.kv.Put(context.Background(), persist.Row{Kind: kindFreezeAudit, Key: ev.ID, Body: body}, persist.Fence{})
}

// FreezeAudits 级联闸门记录，新的在前。parentID 空则全部。

func (s *Service) FreezeAudits(parentID string, limit int) []FreezeAudit {
	if s == nil {
		return nil
	}
	_ = s.ensureHydrated()
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	parentID = strings.TrimSpace(parentID)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]FreezeAudit, 0, limit)
	for i := len(s.audits) - 1; i >= 0 && len(out) < limit; i-- {
		ev := s.audits[i]
		if parentID != "" && ev.ParentID != parentID && ev.ChildID != parentID {
			continue
		}
		out = append(out, ev)
	}
	return out
}

// privIsSubset 新权限没有超出原集合。冻结期间只允许原样或收回。

func privIsSubset(next, cur []Privilege) bool {
	have := map[Privilege]struct{}{}
	for _, p := range cur {
		have[p] = struct{}{}
	}
	for _, p := range next {
		if _, ok := have[p]; !ok {
			return false
		}
	}
	return true
}

// RejectChildCascade 拒绝「解母户同时解子户」。状态不变，只留审计。

func (s *Service) RejectChildCascade(parentID, actor string) error {
	if s == nil {
		return ErrUnavailable
	}
	parentID = strings.TrimSpace(parentID)
	s.mu.Lock()
	defer s.mu.Unlock()
	var children []string
	if p, ok := s.parents[parentID]; ok {
		for _, id := range s.byParent[p.ID] {
			c := s.children[id]
			if c != nil && c.Status == StatusFrozen {
				children = append(children, id)
			}
		}
	}
	s.recordFreezeLocked(FreezeAudit{
		Actor: actor, Action: "unfreeze_rejected", ParentID: parentID, Children: children,
		Detail: "unfreeze parent does not unfreeze children",
	})
	return ErrFrozen
}

func (s *Service) loadFreezeAuditLocked(ctx context.Context) error {
	if s == nil || s.kv == nil {
		return nil
	}
	after := ""
	for {
		page, err := persist.PageKind(ctx, s.kv, kindFreezeAudit, persist.Cursor{After: after, Limit: 100})
		if err != nil {
			return err
		}
		for _, row := range page.Rows {
			var ev FreezeAudit
			if json.Unmarshal(row.Body, &ev) != nil {
				continue
			}
			s.audits = append(s.audits, ev)
		}
		if page.Done || page.Next == "" {
			break
		}
		after = page.Next
	}
	return nil
}
