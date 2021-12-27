package subacct

import (
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"context"
	"errors"
	"github.com/google/uuid"
	"strings"
)

// Hop 母户资金账与子户资金账互转。子户对子户直接拒，必须经母户。
func (s *Service) Hop(ctx context.Context, req HopReq) (HopRec, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil {
		return HopRec{}, ErrUnavailable
	}
	if err := s.ensureHydrated(); err != nil {
		return HopRec{}, err
	}
	as,
		err := validate.Asset(req.Asset)
	if err != nil {
		return HopRec{}, ErrAsset
	}
	amt, err := money.Parse(req.Amount)
	if err != nil {
		return HopRec{}, ErrAmount
	}
	idem, err := validate.Idem(req.IdempotencyKey)
	if err != nil {
		return HopRec{}, ErrIdem
	}
	from := strings.TrimSpace(req.FromOwner)
	to := strings.TrimSpace(req.ToOwner)
	if from == "" || to == "" || from == to {
		return HopRec{}, ErrSelf
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.byIdem[idem]; ok {
		cur := s.hops[id]
		if cur.FromOwner != from || cur.ToOwner != to || cur.Asset != as || cur.Amount != amt.String() {
			return HopRec{}, ErrIdem
		}
		return cur.Copy(), nil
	}
	if err := s.sameTreeLocked(from, to); err != nil {
		return HopRec{}, err
	}
	if err := s.requireActiveLocked(from, true); err != nil {
		if errors.Is(err, ErrFrozen) {
			s.rejectFrozenLocked(from, gateHopOut, "")
		}
		return HopRec{}, err
	}
	if err := s.requireActiveLocked(to, false); err != nil {
		if errors.Is(err, ErrFrozen) {
			s.rejectFrozenLocked(to, gateHopIn, "")
		}
		return HopRec{}, err
	}
	s.noteCreditFrozen(to, "hop in allowed while frozen")
	if err := s.requirePrivLocked(from, PrivHop); err != nil {
		return HopRec{}, err
	}
	if err := s.requirePrivLocked(to, PrivTransfer); err != nil {
		return HopRec{}, err
	}
	if err := s.vel.Reserve(from, amt.String()); err != nil {
		return HopRec{}, err
	}
	src := s.bookLocked(from, "funding", as)
	dst := s.bookLocked(to, "funding", as)
	nextSrc, err := src.Available.Sub(amt)
	if err != nil {
		s.vel.ReleaseLast(from)
		return HopRec{}, err
	}
	nextDst, err := dst.Available.Add(amt)
	if err != nil {
		s.vel.ReleaseLast(from)
		return HopRec{}, err
	}
	left, err := src.Available.Add(dst.Available)
	if err != nil {
		s.vel.ReleaseLast(from)
		return HopRec{}, err
	}
	src.Available = nextSrc
	dst.Available = nextDst
	right, err := src.Available.Add(dst.Available)
	if err != nil {
		s.vel.ReleaseLast(from)
		return HopRec{}, err
	}
	if err := money.Conserve(left, right); err != nil {
		src.Available, _ = src.Available.Add(amt)
		dst.Available,
			_ = dst.Available.Sub(amt)
		s.vel.ReleaseLast(from)
		return HopRec{}, err
	}
	lines := money.TransferPair(as, amt, "sub_debit", "sub_credit")
	if err := money.Balanced(lines); err != nil {
		src.Available,
			_ = src.Available.Add(amt)
		dst.Available, _ = dst.Available.Sub(amt)
		s.vel.ReleaseLast(from)
		return HopRec{}, err
	}
	fromUser, toUser := src.UserID, dst.UserID
	row := &HopRec{
		ID: uuid.NewString(), Idem: idem, FromOwner: from, ToOwner: to,
		FromUser: fromUser, ToUser: toUser, FromBook: "funding", ToBook: "funding",
		Asset: as, Amount: amt.String(), Kind: "parent_child", CreatedAt: s.now(),
	}
	s.hops[row.ID] = row
	s.byIdem[idem] = row.ID
	if err := s.persistBookKV(src); err != nil {
		src.Available, _ = src.Available.Add(amt)
		dst.Available, _ = dst.Available.Sub(amt)
		delete(s.hops, row.ID)
		delete(s.byIdem, idem)
		s.vel.ReleaseLast(from)
		return HopRec{}, err
	}
	if err := s.persistBookKV(dst); err != nil {
		src.Available, _ = src.Available.Add(amt)
		dst.Available,
			_ = dst.Available.Sub(amt)
		_ = s.persistBookKV(src)
		delete(s.hops, row.ID)
		delete(s.byIdem, idem)
		s.vel.ReleaseLast(from)
		return HopRec{}, err
	}
	if err := s.persistHopKV(row.Copy()); err != nil {
		src.Available, _ = src.Available.Add(amt)
		dst.Available, _ = dst.Available.Sub(amt)
		_ = s.persistBookKV(src)
		_ = s.persistBookKV(dst)
		delete(s.hops, row.ID)
		delete(s.byIdem, idem)
		s.vel.ReleaseLast(from)
		return HopRec{}, err
	}
	return row.Copy(), nil
}

func (s *Service) sameTreeLocked(a, b string) error {
	pa, ca, err := s.parentOfLocked(a)
	if err != nil {
		return ErrHop
	}
	pb, cb, err := s.parentOfLocked(b)
	if err != nil {
		return ErrHop
	}
	if pa.ID != pb.ID {
		return ErrHop
	}
	if ca != nil && cb != nil {
		return ErrHop
	}
	return nil
}

func (s *Service) GetHopByIdem(idem string) (HopRec, error) {
	if s == nil {
		return HopRec{}, ErrUnavailable
	}
	if err := s.ensureHydrated(); err != nil {
		return HopRec{}, err
	}
	idem = strings.TrimSpace(idem)
	if idem == "" {
		return HopRec{}, ErrHop
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byIdem[idem]
	if !ok {
		return HopRec{}, ErrHop
	}
	row, ok := s.hops[id]
	if !ok {
		return HopRec{}, ErrHop
	}
	return row.Copy(), nil
}

// ReverseHop 把已成功 hop 的金额划回并丢掉幂等键，同键可再跳。已冲正则空操作。

func (s *Service) ReverseHop(ctx context.Context, idem string) error {
	if s == nil {
		return ErrUnavailable
	}
	if err := s.ensureHydrated(); err != nil {
		return err
	}
	key, err := validate.Idem(idem)
	if err != nil {
		return ErrIdem
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byIdem[key]
	if !ok {
		return nil
	}
	row := s.hops[id]
	if row == nil {
		delete(s.byIdem, key)
		return nil
	}
	if row.Reversed {
		delete(s.byIdem, key)
		return nil
	}
	amt, err := money.Parse(row.Amount)
	if err != nil {
		return err
	}
	src := s.bookLocked(row.FromOwner, "funding", row.Asset)
	dst := s.bookLocked(row.ToOwner, "funding", row.Asset)
	left, err := src.Available.Add(dst.Available)
	if err != nil {
		return err
	}
	nextSrc, err := src.Available.Add(amt)
	if err != nil {
		return err
	}
	nextDst, err := dst.Available.Sub(amt)
	if err != nil {
		return err
	}
	prevSrc, prevDst := src.Available, dst.Available
	src.Available = nextSrc
	dst.Available = nextDst
	right, err := src.Available.Add(dst.Available)
	if err != nil {
		src.Available, dst.Available = prevSrc, prevDst
		return err
	}
	if err := money.Conserve(left, right); err != nil {
		src.Available, dst.Available = prevSrc, prevDst
		return err
	}
	row.Reversed = true
	row.Kind = "reversed"
	delete(s.byIdem, key)
	if err := s.persistBookKV(src); err != nil {
		src.Available, dst.Available = prevSrc, prevDst
		row.Reversed = false
		row.Kind = "parent_child"
		s.byIdem[key] = id
		return err
	}
	if err := s.persistBookKV(dst); err != nil {
		src.Available, dst.Available = prevSrc, prevDst
		row.Reversed = false
		row.Kind = "parent_child"
		s.byIdem[key] = id
		_ = s.persistBookKV(src)
		return err
	}
	if err := s.persistHopKV(row.Copy()); err != nil {
		src.Available, dst.Available = prevSrc, prevDst
		row.Reversed = false
		row.Kind = "parent_child"
		s.byIdem[key] = id
		_ = s.persistBookKV(src)
		_ = s.persistBookKV(dst)
		return err
	}
	return nil
}

func (s *Service) GetHop(id string) (HopRec, error) {
	if s == nil {
		return HopRec{}, ErrUnavailable
	}
	if err := s.ensureHydrated(); err != nil {
		return HopRec{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row,
		ok := s.hops[strings.TrimSpace(id)]
	if !ok {
		return HopRec{}, ErrHop
	}
	return row.Copy(), nil
}

func (s *Service) ListHops(owner string, limit int) []HopRec {
	if s == nil {
		return nil
	}
	_ = s.ensureHydrated()
	if limit <= 0 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]HopRec, 0, limit)
	for _, h := range s.hops {
		if owner != "" && h.FromOwner != owner && h.ToOwner != owner {
			continue
		}
		out = append(out, h.Copy())
		if len(out) >= limit {
			break
		}
	}
	return out
}
