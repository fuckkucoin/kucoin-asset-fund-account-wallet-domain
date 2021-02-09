package approvestore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	// KindWDRelease 用户出金放行，不是托管归集。
	KindWDRelease = "wd_release"
	kvRelease     = "wd_release_ref"
)

// ReleaseRef 放行票据在审批库旁的索引。

type ReleaseRef struct {
	ID        string `json:"id"`
	TicketID  string `json:"ticket_id"`
	Applicant string `json:"applicant"`
	PolicyVer string `json:"policy_version"`
	State string `json:"state"`
	Amount string `json:"amount"`
	Asset  string `json:"asset"`
	Kind string `json:"kind"`
}

var relFence sync.Map

   func relKey(s *Store, id string) string {
	   return fmt.Sprintf("%p|%s", s, id)
  }

// NoteRelease 记下出金放行引用。归集、签章、仪式种类一律拒绝。
  func (s *Store) NoteRelease(ctx context.Context, ref ReleaseRef) error {
	  if s == nil || s.kv == nil {
		   return persist.ErrTxClosed
	   }
	   switch ref.Kind {
	  case string(custody.KindSweep), string(custody.KindSign), string(custody.KindCeremony):
		  return custody.ErrIllegalApproval
	   }
	   if ref.ID == "" {
		   ref.ID = ref.TicketID
	  }
	   if ref.ID == "" {
		   return custody.ErrIllegalApproval
	   }
	  ref.Kind = KindWDRelease
	   body, err := json.Marshal(ref)
	  if err != nil {
		   return err
	  }
	   exp := persist.Fence{}
	   if v, ok := relFence.Load(relKey(s, ref.ID)); ok {
		   exp, _ = v.(persist.Fence)
	  }
	   if err := s.kv.Put(ctx, persist.Row{Kind: kvRelease, Key: ref.ID, Body: body}, exp); err != nil {
		   return err
	   }
	  got, err := s.kv.Get(ctx, kvRelease, ref.ID)
	   if err != nil {
		   return err
	   }
	   relFence.Store(relKey(s, ref.ID), got.Fence)
	   return nil
   }

// ReleaseOf 读取出金放行引用。不存在时不是一笔归集审批。

  func (s *Store) ReleaseOf(ctx context.Context, id string) (ReleaseRef, error) {
	  if s == nil || s.kv == nil {
		  return ReleaseRef{}, persist.ErrTxClosed
	   }
	   row, err := s.kv.Get(ctx, kvRelease, id)
	  if err != nil {
		  return ReleaseRef{}, err
	  }
	   var ref ReleaseRef
	  if err := json.Unmarshal(row.Body, &ref); err != nil {
		  return ReleaseRef{}, err
	   }
	  if ref.Kind == string(custody.KindSweep) {
		  return ReleaseRef{}, custody.ErrIllegalApproval
	   }
	   return ref, nil
   }
