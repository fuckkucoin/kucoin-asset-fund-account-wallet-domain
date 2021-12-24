package subacct

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type bookRow struct {
	OwnerID   string
	UserID    string
	Book      string
	Asset     string
	Available money.Amount
	Frozen    money.Amount
	Fence     persist.Fence
}

func bookKey(owner, book, asset string) string {
	return owner + "|" + book + "|" + asset
}

func (s *Service) listBooksLocked(parentID string) []BookBal {
	owners := []string{parentID}
	owners = append(owners, s.byParent[parentID]...)
	out := make([]BookBal, 0)
	for _, o := range owners {
		uid := ""
		if p, ok := s.parents[o]; ok {
			uid = p.UserID
		}
		if c, ok := s.children[o]; ok {
			uid = c.UserID
		}
		for _, row := range s.books {
			if row.OwnerID != o {
				continue
			}
			out = append(out, BookBal{
				OwnerID: row.OwnerID, UserID: uid, Book: row.Book, Asset: row.Asset,
				Available: row.Available.String(), Frozen: row.Frozen.String(),
			})
		}
	}
	return out
}

func (s *Service) bookLocked(owner, book, asset string) *bookRow {
	k := bookKey(owner, book, asset)
	row := s.books[k]
	if row == nil {
		uid := owner
		if p, ok := s.parents[owner]; ok {
			uid = p.UserID
		}
		if c, ok := s.children[owner]; ok {
			uid = c.UserID
		}
		row = &bookRow{OwnerID: owner,
			UserID: uid, Book: book, Asset: asset, Available: money.Zero(), Frozen: money.Zero()}
		s.books[k] = row
	}
	return row
}

// BookReader 关子账户前核对真实资金账，避免只看内部 overlay。
type BookReader interface {
	Accounts(ctx context.Context, userID string) ([]ledger.BookView, error)
}

func (s *Service) BindBooks(r BookReader) *Service {
	if s != nil {
		s.ledgers = r
	}
	return s
}

func (s *Service) requireLedgerZero(userID string) error {
	if s == nil || s.ledgers == nil || strings.TrimSpace(userID) == "" {
		return nil
	}
	rows, err := s.ledgers.Accounts(context.Background(), userID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		avail, err := money.Parse(row.Available)
		if err != nil {
			return err
		}
		fr, err := money.Parse(row.Frozen)
		if err != nil {
			return err
		}
		if !avail.IsZero() || !fr.IsZero() {
			return fmt.Errorf("%w: ledger %s still holds %s", ErrClosed, row.Asset, row.Available)
		}
	}
	return nil
}

func (s *Service) requireZeroLocked(ownerID string) error {
	for _, row := range s.books {
		if row.OwnerID != ownerID {
			continue
		}
		if !row.Available.IsZero() || !row.Frozen.IsZero() {
			return fmt.Errorf("%w: %s still holds %s", ErrClosed, row.Asset, row.Available)
		}
	}
	return nil
}

func (s *Service) Credit(req PostReq) (BookBal, error) {
	if s == nil {
		return BookBal{}, ErrUnavailable
	}
	if err := s.ensureHydrated(); err != nil {
		return BookBal{}, err
	}
	as, err := validate.Asset(req.Asset)
	if err != nil {
		return BookBal{}, ErrAsset
	}
	amt, err := money.Parse(req.Amount)
	if err != nil {
		return BookBal{}, ErrAmount
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if prev, ok := s.replayPostLocked("credit", req); ok {
		return prev, nil
	}
	owner := strings.TrimSpace(req.OwnerID)
	if err := s.requireActiveLocked(owner, false); err != nil {
		return BookBal{}, err
	}
	s.noteCreditFrozen(owner, "credit allowed while frozen")
	if err := s.requirePrivLocked(owner, PrivDeposit); err != nil {
		return BookBal{}, err
	}
	row := s.bookLocked(owner, "funding", as)
	next, err := row.Available.Add(amt)
	if err != nil {
		return BookBal{}, err
	}
	prevAvail := row.Available
	row.Available = next
	bal := BookBal{
		OwnerID: row.OwnerID, UserID: row.UserID, Book: row.Book, Asset: row.Asset,
		Available: row.Available.String(), Frozen: row.Frozen.String(),
	}
	s.rememberPostLocked("credit", req, bal)
	if err := s.persistBookKV(row); err != nil {
		row.Available = prevAvail
		s.forgetPostLocked("credit", req)
		return BookBal{}, err
	}
	if err := s.persistPostKV(postIdemKey("credit", req), bal); err != nil {
		row.Available = prevAvail
		_ = s.persistBookKV(row)
		s.forgetPostLocked("credit", req)
		return BookBal{}, err
	}
	return bal, nil
}

func (s *Service) Debit(req PostReq) (BookBal, error) {
	if s == nil {
		return BookBal{}, ErrUnavailable
	}
	if err := s.ensureHydrated(); err != nil {
		return BookBal{}, err
	}
	as, err := validate.Asset(req.Asset)
	if err != nil {
		return BookBal{}, ErrAsset
	}
	amt,
		err := money.Parse(req.Amount)
	if err != nil {
		return BookBal{}, ErrAmount
	}
	owner := strings.TrimSpace(req.OwnerID)
	// CAS 闸与 journal 必须在账本锁外：Guard/Note 会回读 GetParent，持锁再入会死锁。
	if err := s.guardDebit(owner, req.FreezeCASID, as, amt.String()); err != nil {
		s.noteDebit(owner, req.FreezeCASID, as, amt.String(), req.IdempotencyKey, false, err.Error())
		return BookBal{}, err
	}
	s.mu.Lock()
	if prev, ok := s.replayPostLocked("debit", req); ok {
		s.mu.Unlock()
		return prev, nil
	}
	if err := s.requireActiveLocked(owner, true); err != nil {
		if errors.Is(err, ErrFrozen) {
			s.rejectFrozenLocked(owner, gateDebit, "")
		}
		s.mu.Unlock()
		s.noteDebit(owner, req.FreezeCASID, as, amt.String(), req.IdempotencyKey, false, err.Error())
		return BookBal{}, err
	}
	if err := s.requirePrivLocked(owner, PrivWithdraw); err != nil {
		s.mu.Unlock()
		return BookBal{}, err
	}
	if err := s.vel.Reserve(owner, amt.String()); err != nil {
		s.mu.Unlock()
		return BookBal{}, err
	}
	row := s.bookLocked(owner, "funding", as)
	next, err := row.Available.Sub(amt)
	if err != nil {
		s.vel.ReleaseLast(owner)
		s.mu.Unlock()
		return BookBal{}, err
	}
	prevAvail := row.Available
	row.Available = next
	bal := BookBal{
		OwnerID: row.OwnerID, UserID: row.UserID, Book: row.Book, Asset: row.Asset,
		Available: row.Available.String(), Frozen: row.Frozen.String(),
	}
	s.rememberPostLocked("debit", req, bal)
	if err := s.persistBookKV(row); err != nil {
		row.Available = prevAvail
		s.vel.ReleaseLast(owner)
		s.forgetPostLocked("debit", req)
		s.mu.Unlock()
		return BookBal{}, err
	}
	if err := s.persistPostKV(postIdemKey("debit", req), bal); err != nil {
		row.Available = prevAvail
		_ = s.persistBookKV(row)
		s.vel.ReleaseLast(owner)
		s.forgetPostLocked("debit", req)
		s.mu.Unlock()
		return BookBal{}, err
	}
	s.mu.Unlock()
	s.noteDebit(owner, req.FreezeCASID, as, amt.String(), req.IdempotencyKey, true, "")
	return bal, nil
}

func postIdemKey(kind string, req PostReq) string {
	key := strings.TrimSpace(req.IdempotencyKey)
	if key == "" {
		return ""
	}
	return kind + "|" + strings.TrimSpace(req.OwnerID) + "|" + key
}

func (s *Service) replayPostLocked(kind string, req PostReq) (BookBal, bool) {
	k := postIdemKey(kind, req)
	if k == "" || s == nil || s.postIdem == nil {
		return BookBal{}, false
	}
	got, ok := s.postIdem[k]
	return got, ok
}

func (s *Service) rememberPostLocked(kind string, req PostReq, bal BookBal) {
	k := postIdemKey(kind, req)
	if k == "" || s == nil {
		return
	}
	if s.postIdem == nil {
		s.postIdem = map[string]BookBal{}
	}
	s.postIdem[k] = bal
}

func (s *Service) forgetPostLocked(kind string, req PostReq) {
	k := postIdemKey(kind, req)
	if k == "" || s == nil || s.postIdem == nil {
		return
	}
	delete(s.postIdem, k)
	s.deletePostKV(k)
}

// ForgetPost 补偿冲正后丢掉原过账幂等快照，重试必须重新借贷，禁止回放已冲正的余额。

func (s *Service) ForgetPost(kind string, req PostReq) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.forgetPostLocked(kind, req)
}

func (s *Service) FreezeAmt(owner, asset, amount string) (BookBal, error) {
	if s == nil {
		return BookBal{}, ErrUnavailable
	}
	if err := s.ensureHydrated(); err != nil {
		return BookBal{}, err
	}
	as, err := validate.Asset(asset)
	if err != nil {
		return BookBal{}, ErrAsset
	}
	amt, err := money.Parse(amount)
	if err != nil {
		return BookBal{}, ErrAmount
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireActiveLocked(strings.TrimSpace(owner), true); err != nil {
		return BookBal{}, err
	}
	row := s.bookLocked(strings.TrimSpace(owner), "funding", as)
	next, err := row.Available.Sub(amt)
	if err != nil {
		return BookBal{}, err
	}
	fr, err := row.Frozen.Add(amt)
	if err != nil {
		return BookBal{}, err
	}
	if err := money.Conserve(amt, amt); err != nil {
		return BookBal{}, err
	}
	prevAvail, prevFr := row.Available, row.Frozen
	row.Available = next
	row.Frozen = fr
	if err := s.persistBookKV(row); err != nil {
		row.Available, row.Frozen = prevAvail, prevFr
		return BookBal{}, err
	}
	return BookBal{
		OwnerID: row.OwnerID, UserID: row.UserID, Book: row.Book, Asset: row.Asset,
		Available: row.Available.String(),
		Frozen:    row.Frozen.String(),
	}, nil
}

func (s *Service) UnfreezeAmt(owner, asset, amount string) (BookBal, error) {
	if s == nil {
		return BookBal{},
			ErrUnavailable
	}
	if err := s.ensureHydrated(); err != nil {
		return BookBal{}, err
	}
	as, err := validate.Asset(asset)
	if err != nil {
		return BookBal{}, ErrAsset
	}
	amt, err := money.Parse(amount)
	if err != nil {
		return BookBal{}, ErrAmount
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.bookLocked(strings.TrimSpace(owner), "funding", as)
	next, err := row.Frozen.Sub(amt)
	if err != nil {
		return BookBal{}, err
	}
	av, err := row.Available.Add(amt)
	if err != nil {
		return BookBal{}, err
	}
	prevAvail, prevFr := row.Available, row.Frozen
	row.Frozen = next
	row.Available = av
	if err := s.persistBookKV(row); err != nil {
		row.Available, row.Frozen = prevAvail, prevFr
		return BookBal{}, err
	}
	return BookBal{
		OwnerID: row.OwnerID, UserID: row.UserID, Book: row.Book, Asset: row.Asset,
		Available: row.Available.String(), Frozen: row.Frozen.String(),
	}, nil
}

func (s *Service) ListOwnerBooks(ownerID string) []BookBal {
	if s == nil {
		return nil
	}
	_ = s.ensureHydrated()
	s.mu.Lock()
	defer s.mu.Unlock()
	ownerID = strings.TrimSpace(ownerID)
	out := make([]BookBal, 0)
	uid := ownerID
	if p, ok := s.parents[ownerID]; ok {
		uid = p.UserID
	}
	if c, ok := s.children[ownerID]; ok {
		uid = c.UserID
	}
	for _, row := range s.books {
		if row.OwnerID != ownerID {
			continue
		}
		out = append(out, BookBal{
			OwnerID: row.OwnerID,
			UserID:  uid, Book: row.Book, Asset: row.Asset,
			Available: row.Available.String(), Frozen: row.Frozen.String(),
		})
	}
	return out
}

func (s *Service) BookOf(owner, asset string) (BookBal, error) {
	if s == nil {
		return BookBal{}, ErrUnavailable
	}
	if err := s.ensureHydrated(); err != nil {
		return BookBal{}, err
	}
	as, err := validate.Asset(asset)
	if err != nil {
		return BookBal{}, ErrAsset
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.bookLocked(strings.TrimSpace(owner), "funding", as)
	return BookBal{
		OwnerID: row.OwnerID, UserID: row.UserID, Book: row.Book, Asset: row.Asset,
		Available: row.Available.String(), Frozen: row.Frozen.String(),
	}, nil
}
