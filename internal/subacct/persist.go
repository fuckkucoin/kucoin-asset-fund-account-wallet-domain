package subacct

import (
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"context"
	"encoding/json"
	"errors"
	"strings"
)

const kindParent = "subacct.parent"

const kindChild = "subacct.child"

const kindHop = "subacct.hop"

type storedParent struct {
	Parent
}

type storedChild struct {
	Child
}

func (s *Service) PersistParent(ctx context.Context, p Parent) (Parent, error) {
	if s == nil || s.kv == nil {
		return Parent{}, ErrUnavailable
	}
	body, err := json.Marshal(storedParent{Parent: p})
	if err != nil {
		return Parent{}, err
	}
	err = s.kv.InTx(ctx, func(ctx context.Context) error {
		cur, ferr := s.kv.Get(ctx, kindParent, p.ID)
		if ferr != nil && !errors.Is(ferr, persist.ErrNotFound) {
			return ferr
		}
		exp := persist.Fence{}
		if ferr == nil {
			exp = cur.Fence
		}
		return s.kv.Put(ctx, persist.Row{Kind: kindParent, Key: p.ID, Body: body}, exp)
	})
	if err != nil {
		if errors.Is(err, persist.ErrStaleFence) {
			return Parent{}, ErrFence
		}
		return Parent{}, err
	}
	got,
		err := s.kv.Get(ctx, kindParent, p.ID)
	if err != nil {
		return Parent{}, err
	}
	var out storedParent
	if err := json.Unmarshal(got.Body, &out); err != nil {
		return Parent{}, err
	}
	out.Fence = got.Fence
	if err := s.persistParentSQL(out.Parent); err != nil {
		return Parent{}, err
	}
	return out.Parent, nil
}

func (s *Service) PersistChild(ctx context.Context, c Child) (Child, error) {
	if s == nil || s.kv == nil {
		return Child{}, ErrUnavailable
	}
	body, err := json.Marshal(storedChild{Child: c})
	if err != nil {
		return Child{}, err
	}
	err = s.kv.InTx(ctx, func(ctx context.Context) error {
		cur, ferr := s.kv.Get(ctx, kindChild, c.ID)
		if ferr != nil && !errors.Is(ferr, persist.ErrNotFound) {
			return ferr
		}
		exp := persist.Fence{}
		if ferr == nil {
			exp = cur.Fence
		}
		return s.kv.Put(ctx, persist.Row{Kind: kindChild, Key: c.ID, Body: body}, exp)
	})
	if err != nil {
		if errors.Is(err, persist.ErrStaleFence) {
			return Child{}, ErrFence
		}
		return Child{}, err
	}
	got, err := s.kv.Get(ctx, kindChild, c.ID)
	if err != nil {
		return Child{}, err
	}
	var out storedChild
	if err := json.Unmarshal(got.Body, &out); err != nil {
		return Child{}, err
	}
	out.Fence = got.Fence
	if err := s.persistChildSQL(out.Child); err != nil {
		return Child{}, err
	}
	return out.Child, nil
}

func (s *Service) PersistHop(ctx context.Context, h HopRec) (HopRec, error) {
	if s == nil || s.kv == nil {
		return HopRec{}, ErrUnavailable
	}
	body, err := json.Marshal(h)
	if err != nil {
		return HopRec{}, err
	}
	err = s.kv.InTx(ctx, func(ctx context.Context) error {
		cur, ferr := s.kv.Get(ctx, kindHop, h.Idem)
		if ferr == nil {
			var old HopRec
			if err := json.Unmarshal(cur.Body, &old); err != nil {
				return err
			}
			if old.FromOwner != h.FromOwner || old.ToOwner != h.ToOwner || old.Asset != h.Asset || old.Amount != h.Amount {
				return persist.ErrConflict
			}
			h = old
			return nil
		}
		if !errors.Is(ferr, persist.ErrNotFound) {
			return ferr
		}
		return s.kv.Put(ctx, persist.Row{Kind: kindHop, Key: h.Idem, Body: body}, persist.Fence{})
	})
	if err != nil {
		if errors.Is(err, persist.ErrConflict) {
			return HopRec{}, ErrIdem
		}
		return HopRec{}, err
	}
	if err := s.persistHopSQL(h); err != nil && !errors.Is(err, sqlrepo.ErrConflict) {
		return HopRec{}, err
	}
	return h, nil
}

func (s *Service) LoadParent(ctx context.Context, id string) (Parent, error) {
	if s == nil || s.kv == nil {
		return Parent{}, ErrUnavailable
	}
	row, err := s.kv.Get(ctx, kindParent, strings.TrimSpace(id))
	if err != nil {
		return Parent{}, err
	}
	var out storedParent
	if err := json.Unmarshal(row.Body, &out); err != nil {
		return Parent{}, err
	}
	out.Fence = row.Fence
	return out.Parent, nil
}

func RegionHint() persist.RegionDSN {
	return persist.DSNFor(persist.RegionSG)
}
