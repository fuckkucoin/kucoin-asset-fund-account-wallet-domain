package cerbind

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"account-assets.kucoin.net/funding/internal/airgap"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/keycer"
)

// AirgapExec 是气隙会话执行器。

type AirgapExec interface {
	Open(actor,
		label string, shares, threshold int, p airgap.Payload) (airgap.Session, error)
	Tag(id, ref string) (airgap.Session, error)
}

// KeycerExec 是解锁工单执行器。只碰槽位标签和指纹。
type KeycerExec interface {
	EnsureSlot(id, label string) (keycer.Slot, error)
	Get(id string) (keycer.Slot, error)
	Load(id, label, fp, actor string) (keycer.Slot, error)
	Lock(id, actor string) (keycer.Slot, error)
	OpenUnlock(slotID, actor, note string) (keycer.Ticket, error)
	TagTicket(id, ref string) (keycer.Ticket, error)
}

// CustodyExec 是托管相位簿。只记录阶段，不保存密钥。
type CustodyExec interface {
	Start(actor,
		label string, shares, threshold int) (*custody.Ceremony, error)
	Advance(id, actor string, next custody.CeremonyPhase) (*custody.Ceremony, error)
	Annotate(id, note string) error
}

type execIDs struct {
	Air string
	Key string
	Cus string
}

  func (e *Engine) bindDefaultExec() {
	  if e == nil {
		  return
	   }
	  if e.exec.Air == nil {
		  e.exec.Air = airgap.Process()
	  }
	   if e.exec.Key == nil {
		   e.exec.Key = keycer.Process()
	  }
	   if e.exec.Cus == nil {
		  e.exec.Cus = custody.ProcessCeremony()
	  }
  }

// UseExec 替换执行器。可传入独立实例，避免碰进程单例。
  func (e *Engine) UseExec(ex Exec) {
	  if e == nil {
		   return
	  }
	  e.mu.Lock()
	  e.exec = ex
	   e.mu.Unlock()
  }

   func runExec(ex Exec, ses Session, actor string) (execIDs, string, error) {
	   var ids execIDs
	   if ex.Air == nil && ex.Key == nil && ex.Cus == nil {
		  return ids, "no executor", nil
	  }
	   if ex.Air != nil {
		  row, err := ex.Air.Open(actor, "cerbind:"+ses.ID, ses.Shares, ses.Threshold, airgap.Payload{
			  Kind: "withdraw", Asset: ses.Asset, Amount: ses.Amount, Dest: ses.Dest,
		  })
		   if err != nil {
			  return ids, "", err
		   }
		   tagged, err := ex.Air.Tag(row.ID, ses.BatchID)
		   if err != nil {
			   return ids, "", err
		   }
		   ids.Air = tagged.ID
	  }
	  if ex.Key != nil {
		   ticket, err := openKeyTicket(ex.Key, ses, actor)
		  if err != nil {
			   return ids, "", err
		   }
		   ids.Key = ticket
	  }
	   if ex.Cus != nil {
		   row, err := ex.Cus.Start(actor, ses.ID, ses.Shares, ses.Threshold)
		  if err != nil {
			   return ids, "", err
		  }
		  if _, err := ex.Cus.Advance(row.ID, actor, custody.PhaseGenerate); err != nil {
			   return ids, "", err
		  }
		   if err := ex.Cus.Annotate(row.ID, "batch:"+ses.BatchID); err != nil {
			  return ids, "", err
		  }
		   ids.Cus = row.ID
	   }
	   return ids, "executors linked", nil
  }

   func openKeyTicket(key KeycerExec, ses Session, actor string) (string, error) {
	   slotID := "cb" + strings.ReplaceAll(ses.ID, "-", "")
	   if len(slotID) > 32 {
		   slotID = slotID[:32]
	  }
	  if _, err := key.EnsureSlot(slotID, "cerbind"); err != nil {
		   return "", err
	  }
	  sl, err := key.Get(slotID)
	  if err != nil {
		  return "", err
	  }
	  fp := fingerprint(ses.ID)
	   switch sl.State {
	  case keycer.SlotEmpty, keycer.SlotRetired:
		   if _, err := key.Load(slotID, "cerbind", fp, actor); err != nil {
			  return "", err
		   }
		   if _, err := key.Lock(slotID, actor); err != nil {
			  return "", err
		   }
	  case keycer.SlotLoaded:
		  if _, err := key.Lock(slotID, actor); err != nil {
			  return "", err
		  }
	  case keycer.SlotLocked:
	  default:
		  return "", ErrState
	   }
	  t, err := key.OpenUnlock(slotID, actor, "cerbind")
	  if err != nil {
		   return "", err
	  }
	   tagged, err := key.TagTicket(t.ID, ses.ID)
	   if err != nil {
		   return "", err
	   }
	   return tagged.ID, nil
   }

   func fingerprint(sessionID string) string {
	  sum := sha256.Sum256([]byte("fp|" + sessionID))
	  return hex.EncodeToString(sum[:])
   }
