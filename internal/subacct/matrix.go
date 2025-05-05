package subacct

import (
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"strings"
	"sync"
	"time"
)

// Grant 改子户权限。出金授权必须换人复核；母户冻结后不得加权限。
type Grant struct {
	ChildID  string
	Actor    string
	Reviewer string
	Add      []Privilege
	Remove   []Privilege
	Assets   []string
	DailyCap string
}

type PrivilegeRow struct {
	ChildID    string      `json:"child_id"`
	Privileges []Privilege `json:"privileges"`
	Assets     []string    `json:"assets,omitempty"`
	DailyCap   string      `json:"daily_cap,omitempty"`
	UsedToday  string      `json:"used_today,omitempty"`
}

type matrixStore struct {
	mu     sync.Mutex
	scope  map[string][]string
	cap    map[string]string
	used   map[string]string
	capDay map[string]string
}

func (s *Service) matrix() *matrixStore {
	if s == nil {
		return &matrixStore{scope: map[string][]string{}, cap: map[string]string{}, used: map[string]string{}, capDay: map[string]string{}}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mx == nil {
		s.mx = &matrixStore{scope: map[string][]string{}, cap: map[string]string{}, used: map[string]string{}, capDay: map[string]string{}}
	}
	return s.mx
}

func actorOK(actor string) error {
	actor = strings.TrimSpace(actor)
	if actor == "" || strings.EqualFold(actor, "system") {
		return ErrActor
	}
	return nil
}

// ApplyGrant 变更子户权限。withdraw 入列时 reviewer 必须与 actor 不同。
func (s *Service) ApplyGrant(req Grant) (PrivilegeRow, error) {
	if s == nil {
		return PrivilegeRow{}, ErrUnavailable
	}
	if err := actorOK(req.Actor); err != nil {
		return PrivilegeRow{}, err
	}
	adds,
		err := normalizePrivDelta(req.Add)
	if err != nil {
		return PrivilegeRow{}, err
	}
	rms,
		err := normalizePrivDelta(req.Remove)
	if err != nil {
		return PrivilegeRow{}, err
	}
	if hasPriv(adds, PrivWithdraw) || hasPriv(rms, PrivWithdraw) {
		if err := actorOK(req.Reviewer); err != nil {
			return PrivilegeRow{}, ErrFourEye
		}
		if strings.EqualFold(strings.TrimSpace(req.Reviewer), strings.TrimSpace(req.Actor)) {
			return PrivilegeRow{}, ErrFourEye
		}
	}
	assets, err := normalizeAssets(req.Assets)
	if err != nil {
		return PrivilegeRow{}, err
	}
	capAmt := strings.TrimSpace(req.DailyCap)
	if capAmt != "" {
		if _, err := money.Parse(capAmt); err != nil {
			return PrivilegeRow{}, ErrAmount
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c,
		ok := s.children[strings.TrimSpace(req.ChildID)]
	if !ok {
		return PrivilegeRow{}, ErrChild
	}
	p, ok := s.parents[c.ParentID]
	if !ok {
		return PrivilegeRow{}, ErrParent
	}
	if p.Status == StatusClosed || c.Status == StatusClosed {
		return PrivilegeRow{}, ErrClosed
	}
	if p.Status == StatusFrozen || c.Status == StatusFrozen {
		// 冻结期间只允许收回权限。加权限、改资产范围或抬日限额都拒绝。
		if len(adds) > 0 || len(assets) > 0 || capAmt != "" || len(rms) == 0 {
			s.recordFreezeLocked(FreezeAudit{
				Action: "privilege_widen_rejected", ParentID: c.ParentID, ChildID: c.ID,
				Detail: "frozen account cannot gain privileges",
			})
			return PrivilegeRow{}, ErrFrozen
		}
	}
	next := mergePrivs(c.Privileges,
		adds, rms)
	c.Privileges = next
	c.UpdatedAt = s.now()
	if s.mx == nil {
		s.mx = &matrixStore{scope: map[string][]string{}, cap: map[string]string{}, used: map[string]string{}, capDay: map[string]string{}}
	}
	if len(assets) > 0 {
		s.mx.scope[c.ID] = assets
	}
	if capAmt != "" {
		s.mx.cap[c.ID] = capAmt
	}
	if err := s.persistGrantSQL(req); err != nil {
		return PrivilegeRow{}, err
	}
	if err := s.persistChildSQL(c.Copy()); err != nil {
		return PrivilegeRow{}, err
	}
	return s.rowLocked(c), nil
}

// normalizePrivDelta 授权增量。空切片表示不改，不得展开成全权限。
func normalizePrivDelta(in []Privilege) ([]Privilege, error) {
	if len(in) == 0 {
		return nil, nil
	}
	return normalizePrivs(in)
}

func mergePrivs(cur, add, rm []Privilege) []Privilege {
	seen := map[Privilege]struct{}{}
	out := make([]Privilege, 0, len(cur)+len(add))
	for _, p := range cur {
		seen[p] = struct{}{}
		out = append(out, p)
	}
	for _, p := range add {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if len(rm) == 0 {
		return out
	}
	drop := map[Privilege]struct{}{}
	for _, p := range rm {
		drop[p] = struct{}{}
	}
	keep := make([]Privilege, 0, len(out))
	for _, p := range out {
		if _, ok := drop[p]; ok {
			continue
		}
		keep = append(keep, p)
	}
	return keep
}

func normalizeAssets(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		as, err := validate.Asset(raw)
		if err != nil {
			return nil, ErrAsset
		}
		if _, ok := seen[as]; ok {
			continue
		}
		seen[as] = struct{}{}
		out = append(out, as)
	}
	return out, nil
}

func (s *Service) rowLocked(c *Child) PrivilegeRow {
	row := PrivilegeRow{ChildID: c.ID, Privileges: append([]Privilege(nil), c.Privileges...)}
	if s.mx != nil {
		if a := s.mx.scope[c.ID]; len(a) > 0 {
			row.Assets = append([]string(nil), a...)
		}
		row.DailyCap = s.mx.cap[c.ID]
		row.UsedToday = s.mx.used[c.ID]
	}
	return row
}

func (s *Service) PrivilegeOf(childID string) (PrivilegeRow, error) {
	if s == nil {
		return PrivilegeRow{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.children[strings.TrimSpace(childID)]
	if !ok {
		return PrivilegeRow{}, ErrChild
	}
	return s.rowLocked(c), nil
}

// AssertAction 子户出金/充值/划转前过权限、资产范围和日限额。
func (s *Service) AssertAction(userID string, priv Privilege, asset, amount string) error {
	if s == nil {
		return ErrUnavailable
	}
	kind, id, err := s.OwnerOf(userID)
	if err != nil {
		return err
	}
	if kind == "parent" {
		return nil
	}
	as := ""
	if strings.TrimSpace(asset) != "" {
		as, err = validate.Asset(asset)
		if err != nil {
			return ErrAsset
		}
	}
	amt := strings.TrimSpace(amount)
	if amt != "" {
		if _, err := money.Parse(amt); err != nil {
			return ErrAmount
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rollCapLocked(id, s.now())
	if err := s.requireActiveLocked(id, priv == PrivWithdraw || priv == PrivHop || priv == PrivTransfer); err != nil {
		return err
	}
	if err := s.requirePrivLocked(id, priv); err != nil {
		return err
	}
	if as != "" && s.mx != nil {
		if scope := s.mx.scope[id]; len(scope) > 0 && !inScope(scope, as) {
			return ErrScope
		}
	}
	if amt != "" && s.mx != nil {
		if capAmt := s.mx.cap[id]; capAmt != "" {
			used := s.mx.used[id]
			if used == "" {
				used = "0"
			}
			u, err := money.ParseBook(used)
			if err != nil {
				return ErrAmount
			}
			add, err := money.Parse(amt)
			if err != nil {
				return ErrAmount
			}
			next, err := u.Add(add)
			if err != nil {
				return err
			}
			lim, err := money.Parse(capAmt)
			if err != nil {
				return ErrAmount
			}
			if next.Cmp(lim) > 0 {
				return ErrCap
			}
		}
	}
	return nil
}

// ConsumeCap 出金成功后占用日限额。失败路径不得调用。

func (s *Service) ConsumeCap(userID, amount string) error {
	if s == nil {
		return ErrUnavailable
	}
	kind, id, err := s.OwnerOf(userID)
	if err != nil {
		return err
	}
	if kind != "child" {
		return nil
	}
	add, err := money.Parse(amount)
	if err != nil {
		return ErrAmount
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mx == nil {
		return nil
	}
	s.rollCapLocked(id, s.now())
	used := s.mx.used[id]
	if used == "" {
		used = "0"
	}
	u, err := money.ParseBook(used)
	if err != nil {
		return ErrAmount
	}
	next, err := u.Add(add)
	if err != nil {
		return err
	}
	if capAmt := s.mx.cap[id]; capAmt != "" {
		lim, err := money.Parse(capAmt)
		if err != nil {
			return ErrAmount
		}
		if next.Cmp(lim) > 0 {
			return ErrCap
		}
	}
	s.mx.used[id] = next.String()
	return nil
}

func (s *Service) rollCapLocked(id string, now time.Time) {
	if s == nil || s.mx == nil {
		return
	}
	if s.mx.capDay == nil {
		s.mx.capDay = map[string]string{}
	}
	day := now.UTC().Format("2006-01-02")
	if s.mx.capDay[id] != day {
		s.mx.used[id] = "0"
		s.mx.capDay[id] = day
	}
}

// RevokeWithdraw 收回出金权。必须换人复核，避免单人改权限矩阵。

func (s *Service) RevokeWithdraw(childID, actor, reviewer string) (PrivilegeRow, error) {
	return s.ApplyGrant(Grant{ChildID: childID, Actor: actor, Reviewer: reviewer, Remove: []Privilege{PrivWithdraw}})
}

func inScope(scope []string, asset string) bool {
	for _, a := range scope {
		if a == asset {
			return true
		}
	}
	return false
}
