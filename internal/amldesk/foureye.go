package amldesk

import (
	"strings"
)

// CloseFourEye 结案必须换人。开案人、system、用户本人都不能关。
// expire/reject 同样要第二只眼，避免开案人自己销案。
   func (b *Book) CloseFourEye(id, actor, decision string, epoch int64, token, note string) (Record, error) {
	   if b == nil {
		   return Record{}, ErrUnavailable
	  }
	  act, err := actorOK(actor)
	  if err != nil {
		   return Record{}, ErrActor
	  }
	  cur, err := b.Get(id)
	  if err != nil {
		   return Record{}, err
	   }
	  if strings.EqualFold(act, cur.OpenedBy) {
		   return Record{}, ErrFourEye
	  }
	  if strings.EqualFold(act, cur.UserID) {
		   return Record{}, ErrFourEye
	  }
	   to := StatusClosed
	  switch strings.ToLower(strings.TrimSpace(decision)) {
	  case "close", "file", "sar_file", "":
		   to = StatusClosed
	  case "reject", "deny":
		   to = StatusRejected
	   case "escalate":
		   to = StatusEscalated
	   default:
		   return Record{}, ErrState
	   }
	   if to == StatusEscalated {
		  return b.mark(id, act, to, epoch, token, note)
	   }
	   return b.mark(id, act, to, epoch, token, note)
  }

  func (b *Book) MarkReview(id, actor string, epoch int64, token string) (Record, error) {
	   return b.mark(id, actor, StatusReview, epoch, token, "")
   }
