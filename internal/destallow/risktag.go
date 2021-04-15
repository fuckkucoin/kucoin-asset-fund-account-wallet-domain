package destallow

import (
	"context"
	"strings"
	"time"
)

// SetRiskTag 给精确命中条打风险标签。
   func (b *Book) SetRiskTag(userID, chain, addr, tag string) (EntryMeta, error) {
	  if b == nil {
		  return EntryMeta{}, ErrMissing
	  }
	   uid, ch, a, err := normUserChainAddr(userID, chain, addr)
	   if err != nil {
		   return EntryMeta{}, err
	   }
	  if _, ok := b.GetExact(uid, ch, a); !ok {
		   return EntryMeta{}, ErrMissing
	   }
	   clean, err := cleanRiskTag(tag)
	  if err != nil {
		  return EntryMeta{}, err
	  }
	  m := b.loadMeta(context.Background(), uid, ch, a)
	   if m.expired(b.now()) {
		   return EntryMeta{}, ErrExpired
	   }
	   m.UserID, m.Chain, m.Address = uid, ch, a
	  m.RiskTag = clean
	   if err := b.putMeta(context.Background(), m); err != nil {
		   return EntryMeta{}, err
	   }
	   b.noteAudit(uid, "risk_tag", uid, ch, a, "", clean)
	  return m, nil
  }

// MetaOf 读覆盖元数据。
   func (b *Book) MetaOf(userID, chain, addr string) EntryMeta {
	  if b == nil {
		  return EntryMeta{}
	  }
	   return b.loadMeta(context.Background(), userID, chain, addr)
  }

   func (v *EntryView) applyMeta(m EntryMeta, now func() time.Time) {
	  if v == nil {
		  return
	  }
	  v.RiskTag = strings.TrimSpace(m.RiskTag)
	  v.ExpiresAt = m.ExpiresAt
	   v.Temp = m.Temp
	   if now != nil && m.expired(now()) {
		  v.Expired = true
	  }
  }
