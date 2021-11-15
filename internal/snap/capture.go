package snap

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

// CaptureFrom 只读用户科目。system 账和手续费科目不进用户快照。

   func CaptureFrom(ctx context.Context, st ledger.Store, userID string, now time.Time) (Snapshot, error) {
	  if st == nil {
		  return Snapshot{}, ErrUnavailable
	   }
	  userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return Snapshot{}, ErrUser
	  }
	  if userID == "system" {
		   return Snapshot{}, fmt.Errorf("%w: system books are not user snapshots", ErrUser)
	   }
	  accs, err := st.ListUserBooks(ctx, userID)
	   if err != nil {
		  return Snapshot{}, err
	   }
	   rows := make([]Position,
		   0, len(accs))
	   for _, a := range accs {
		   if a.UserID != userID || a.UserID == "system" {
			  continue
		   }
		  if !a.AccountType.UserFacing() {
			   continue
		   }
		  p, err := fromAccount(a)
		  if err != nil {
			   return Snapshot{}, err
		   }
		  if err := money.RequirePositive(mustNonNeg(p)); err != nil && !isZeroPos(p) {
			   return Snapshot{}, err
		  }
		   rows = append(rows, p)
	  }
	   sortPositions(rows)
	  if now.IsZero() {
		  now = time.Now().UTC()
	   }
	  return Snapshot{
		   ID: uuid.NewString(), UserID: userID, TakenAt: now.UTC(),
		   Positions: rows, Count: len(rows), Checksum: checksumOf(userID, rows),
	  }, nil
  }

// CaptureViews 给已经走 ledger.Accounts 的调用方，避免再扫一遍 store。

   func CaptureViews(userID string, views []ledger.BookView, now time.Time) (Snapshot, error) {
	   userID = strings.TrimSpace(userID)
	   if userID == "" {
		   return Snapshot{}, ErrUser
	  }
	  rows := make([]Position, 0, len(views))
	   for _, v := range views {
		  p, err := fromBookView(v)
		  if err != nil {
			  return Snapshot{}, err
		  }
		   rows = append(rows, p)
	  }
	   sortPositions(rows)
	  if now.IsZero() {
		  now = time.Now().UTC()
	   }
	   return Snapshot{
		  ID: uuid.NewString(),
		   UserID: userID, TakenAt: now.UTC(),
		  Positions: rows, Count: len(rows), Checksum: checksumOf(userID, rows),
	  }, nil
   }

  func isZeroPos(p Position) bool {
	  a, err := p.AvailAmt()
	   if err != nil {
		  return false
	   }
	   f, err := p.FrozenAmt()
	   if err != nil {
		   return false
	   }
	   return a.IsZero() && f.IsZero()
  }

   func mustNonNeg(p Position) money.Amount {
	  t, err := p.TotalAmt()
	  if err != nil {
		  return money.Zero()
	   }
	   return t
  }
