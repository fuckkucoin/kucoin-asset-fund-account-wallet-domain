package wdrelease

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

const auditGenesis = "GENESIS"

  func (d *Desk) appendAudit(t *Ticket, event, actor, code string, now time.Time) {
	   if d == nil || t == nil {
		   return
	   }
	  prev := auditGenesis
	   rows := d.audits[t.ID]
	   seq := 1
	  if n := len(rows); n > 0 {
		   prev = rows[n-1].Hash
		   seq = rows[n-1].Seq + 1
	   }
	   row := Audit{
		  TicketID: t.ID,
		  Seq:      seq,
		  Event:    event,
		  State: t.State,
		  Actor: actor,
		   Code: code,
		   Prev: prev,
		  At: now.UTC(),
	  }
	   row.Hash = auditHash(row)
	  d.audits[t.ID] = append(rows, row)
   }

  func auditHash(a Audit) string {
	  raw := strings.Join([]string{
		   a.TicketID,
		   strconv.Itoa(a.Seq),
		  a.Prev,
		   a.Event,
		  a.State,
		   a.Actor,
		   a.Code,
		  a.At.UTC().Format(time.RFC3339Nano),
	   }, "|")
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
  }

// VerifyAudit 复核一条票据的哈希链。缺口或哈希不符即失败。

  func VerifyAudit(rows []Audit) error {
	  if len(rows) == 0 {
		  return WithCode(ErrPolicy, CodePolicy)
	   }
	  prev := auditGenesis
	  for i, row := range rows {
		   if row.Seq != i+1 {
			   return WithCode(ErrPolicy, CodePolicy)
		   }
		  if row.Prev != prev {
			  return WithCode(ErrPolicy, CodePolicy)
		  }
		  if auditHash(row) != row.Hash {
			  return WithCode(ErrPolicy, CodePolicy)
		  }
		   prev = row.Hash
	  }
	   return nil
   }
