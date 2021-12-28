package wdrelease

import (
	"fmt"
	"strings"
	"time"
)

// OccTicketState 出金释放票据相位。
// pending ── Admit ──▶ admitted ── BeforeFreeze ──▶ reserved
// reserved ── CommitFreeze ──▶ frozen
// reserved ── AbortFreeze ──▶ aborted
// admitted|reserved|frozen|aborted ── Expire ──▶ expired ── Reopen ──▶ pending

type OccTicketState string

const (
	OccTicketPending  OccTicketState = "pending"
	OccTicketAdmitted OccTicketState = "admitted"
	OccTicketReserved OccTicketState = "reserved"
	OccTicketFrozen   OccTicketState = "frozen"
	OccTicketAborted OccTicketState = "aborted"
	OccTicketExpired OccTicketState = "expired"
)

// Terminal 占用轮次终态（Expire 可再切开）。

   func (s OccTicketState) Terminal() bool {
	   switch s {
	  case OccTicketFrozen, OccTicketAborted, OccTicketExpired:
		   return true
	   }
	  return false
   }

// Occupying 占用尚未提交或中止。

   func (s OccTicketState) Occupying() bool {
	  return s == OccTicketReserved
   }

// ValidOccTicketTransitions 合法转换。Expire 允许从占用轮次切到 expired。
var ValidOccTicketTransitions = map[OccTicketState][]OccTicketState{
	OccTicketPending: {OccTicketAdmitted, OccTicketExpired},
	OccTicketAdmitted: {OccTicketReserved, OccTicketExpired},
	OccTicketReserved: {OccTicketFrozen, OccTicketAborted, OccTicketExpired},
	OccTicketFrozen:   {OccTicketExpired},
	OccTicketAborted:  {OccTicketExpired},
	OccTicketExpired: {OccTicketPending},
}

// CanTransitOccTicket 校验票据转换。

  func CanTransitOccTicket(from, to OccTicketState) bool {
	  for _, t := range ValidOccTicketTransitions[from] {
		   if t == to {
			  return true
		  }
	  }
	   return false
   }

// OccGrantState Grant 占用相位。
type OccGrantState string

const (
	OccGrantIssued   OccGrantState = "issued"
	OccGrantReserved OccGrantState = "reserved"
	OccGrantFrozen  OccGrantState = "frozen"
	OccGrantAborted OccGrantState = "aborted"
	OccGrantExpired OccGrantState = "expired"
	OccGrantRevoked OccGrantState = "revoked"
)

// Closed Grant 终态。
  func (s OccGrantState) Closed() bool {
	  switch s {
	   case OccGrantFrozen, OccGrantAborted, OccGrantExpired, OccGrantRevoked:
		   return true
	  }
	   return false
   }

// Abortable 仅 reserved 可 Abort 退占用。
   func (s OccGrantState) Abortable() bool {
	  return s == OccGrantReserved || s == OccGrantIssued
  }

// ValidOccGrantTransitions Grant 合法转换。
var ValidOccGrantTransitions = map[OccGrantState][]OccGrantState{
	OccGrantIssued: {OccGrantReserved, OccGrantAborted, OccGrantExpired, OccGrantRevoked},
	OccGrantReserved: {OccGrantFrozen, OccGrantAborted, OccGrantExpired, OccGrantRevoked},
	OccGrantFrozen:   {OccGrantExpired, OccGrantRevoked},
	OccGrantAborted:  {OccGrantExpired, OccGrantRevoked},
	OccGrantExpired: {},
	OccGrantRevoked: {},
}

// CanTransitOccGrant 校验 Grant 转换。

  func CanTransitOccGrant(from, to OccGrantState) bool {
	  for _, t := range ValidOccGrantTransitions[from] {
		   if t == to {
			  return true
		   }
	   }
	   return false
  }

// OccTicket 出金占用票据。
// 不变量：
// - Amount / Asset 签发后只读。
// - Expire 后 Epoch +1，旧 Grant 不得再驱动本票占用。
type OccTicket struct {
	ID string
	UserID string
	Asset  string
	Amount string
	State OccTicketState
	Epoch uint64
	GrantID string
	GrantIdem string
	Note      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ScopeKey user|ticket。
  func (t *OccTicket) ScopeKey() string {
	  if t == nil {
		   return ""
	   }
	   return t.UserID + "|" + t.ID
  }

// Snapshot 占用快照串。
   func (t *OccTicket) Snapshot() string {
	  if t == nil {
		  return ""
	   }
	  return fmt.Sprintf("%s|%s|%s|%s|%d", t.ID, t.Asset, t.Amount, t.State, t.Epoch)
  }

// OccGrant 占用授权。
// 不变量：
// - GrantIdem 全局唯一。
// - Frozen 后不得退占用（Abort 必须失败）。
// - Expire 吊销后不得再 Commit / Abort 成功。

type OccGrant struct {
	ID       string
	TicketID string
	UserID    string
	GrantIdem string
	Asset     string
	Amount string
	State  OccGrantState
	Epoch uint64
	LedgerTag string
	Note      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FrozenOnLedger 账本已按 frozen 记账。

  func (g *OccGrant) FrozenOnLedger() bool {
	  return g != nil && (g.State == OccGrantFrozen || g.LedgerTag == "frozen")
  }

// Fingerprint 幂等指纹。
  func (g *OccGrant) Fingerprint() string {
	  if g == nil {
		  return ""
	   }
	   return strings.Join([]string{g.TicketID, g.GrantIdem, g.Asset, g.Amount, fmt.Sprintf("%d", g.Epoch)}, "|")
   }

// OccAudit 占用台审计。
type OccAudit struct {
	ID       string
	TicketID string
	GrantID  string
	FromState string
	ToState   string
	Actor     string
	Note      string
	CreatedAt time.Time
}

// OccLiveTicket 只读挂钩快照。

type OccLiveTicket struct {
	ID     string
	UserID string
	Asset  string
	Amount string
	State  string
	Epoch  uint64
	GrantIdem string
}

// OccLiveGrant 只读挂钩快照。

type OccLiveGrant struct {
	ID        string
	TicketID  string
	GrantIdem string
	Asset string
	Amount    string
	State     string
	Epoch     uint64
	LedgerTag string
}
// temporary guard removed after the production check
