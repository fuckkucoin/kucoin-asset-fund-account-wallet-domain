package subacct

import (
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"context"
	"github.com/google/uuid"
	"strings"
)

func (s *Service) BindSQL(db *persist.SQLDB) *Service {
	if s != nil {
		s.sql = db
		if s.vel == nil {
			s.vel = NewVelocity()
		}
		s.vel.BindSQL(db)
		if s.kv != nil {
			s.vel.BindKV(s.kv)
		}
	}
	return s
}

func (s *Service) sqlOn() bool {
	return s != nil && s.sql != nil && s.sql.DB != nil
}

func joinPrivs(in []Privilege) string {
	if len(in) == 0 {
		return ""
	}
	parts := make([]string, 0, len(in))
	for _, p := range in {
		parts = append(parts, string(p))
	}
	return strings.Join(parts, ",")
}

func (s *Service) persistParentSQL(p Parent) error {
	if !s.sqlOn() {
		return nil
	}
	return sqlrepo.NewSubAcct(s.sql).PutParent(context.Background(), sqlrepo.SubParent{
		ID:     p.ID,
		UserID: p.UserID, Label: p.Label, Status: string(p.Status), Reason: p.Reason,
		Fence: p.Fence, OpenedAt: p.OpenedAt, UpdatedAt: p.UpdatedAt,
	})
}

func (s *Service) persistChildSQL(c Child) error {
	if !s.sqlOn() {
		return nil
	}
	return sqlrepo.NewSubAcct(s.sql).PutChild(context.Background(), sqlrepo.SubChild{
		ID: c.ID, ParentID: c.ParentID, UserID: c.UserID, Label: c.Label, Status: string(c.Status),
		Reason: c.Reason, Privileges: joinPrivs(c.Privileges), Fence: c.Fence, OpenedAt: c.OpenedAt, UpdatedAt: c.UpdatedAt,
	})
}

func (s *Service) persistGrantSQL(req Grant) error {
	if !s.sqlOn() {
		return nil
	}
	return sqlrepo.NewSubAcct(s.sql).PutGrant(context.Background(), sqlrepo.SubGrant{
		ID:      uuid.NewString(),
		ChildID: req.ChildID, Actor: req.Actor, Reviewer: req.Reviewer,
		Add: joinPrivs(req.Add), Remove: joinPrivs(req.Remove), Assets: strings.Join(req.Assets, ","),
		DailyCap: req.DailyCap,
	})
}

func (s *Service) persistHopSQL(h HopRec) error {
	if !s.sqlOn() {
		return nil
	}
	return sqlrepo.NewSubAcct(s.sql).PutHop(context.Background(), sqlrepo.SubHop{
		ID: h.ID, Idem: h.Idem, FromOwner: h.FromOwner, ToOwner: h.ToOwner, FromUser: h.FromUser,
		ToUser: h.ToUser, FromBook: h.FromBook, ToBook: h.ToBook, Asset: h.Asset, Amount: h.Amount,
		Kind: h.Kind, TransferID: h.TransferID, CreatedAt: h.CreatedAt,
	})
}
