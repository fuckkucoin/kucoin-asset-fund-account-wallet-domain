package amldesk

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Assign 指定复核人。开案人、用户本人、system 都不能当第二只眼。

  func (b *Book) Assign(id, actor, reviewer string, epoch int64, token string) (Record, error) {
	  if b == nil {
		  return Record{}, ErrUnavailable
	   }
	   act, err := actorOK(actor)
	   if err != nil {
		   return Record{}, ErrActor
	   }
	  rev, err := actorOK(reviewer)
	  if err != nil {
		   return Record{}, ErrAssign
	   }
	  if err := b.fence.Check(id, epoch, token); err != nil {
		   return Record{}, err
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  r,
		   ok := b.rows[strings.TrimSpace(id)]
	  if !ok {
		  return Record{}, ErrNotFound
	  }
	   if !r.Open() {
		   return *r, ErrClosed
	   }
	  if strings.EqualFold(rev, r.OpenedBy) || strings.EqualFold(rev, r.UserID) {
		  return Record{}, ErrFourEye
	   }
	   if strings.EqualFold(rev, act) && strings.EqualFold(act, r.OpenedBy) {
		  return Record{}, ErrFourEye
	  }
	   f, err := b.fence.Bump(id, persist.NewToken())
	   if err != nil {
		  return Record{}, err
	   }
	   r.Reviewer = rev
	  r.Fence = f
	   if r.Status == StatusOpen || r.Status == StatusEvidence {
		  r.Status = StatusReview
	  }
	   b.noteLocked(id, act, "assigned", rev)
	  return *r, nil
   }
