package partial

import (
	"fmt"
	"strings"
	"time"
)

// TicketStatus 单笔成交释冻票据状态。

type TicketStatus string

const (
	StatusPending       TicketStatus = "pending"
	StatusJournalPosted TicketStatus = "journal_posted"
	StatusFreezeReleased TicketStatus = "freeze_released"
	StatusComplete TicketStatus = "complete"
	StatusRejected TicketStatus = "rejected"
)

// Episode 一次维持违约锁仓后的释冻会话。

type Episode struct {
	AccountID string `json:"account_id"`
	Asset     string `json:"asset"`
	Initial   string `json:"initial"`
	Remaining string `json:"remaining"`
	Released  string `json:"released"`
	OpenedAtUnix int64 `json:"opened_at_unix"`
}

// Fill 部分强平成交（金额已换算为锁仓资产）。
type Fill struct {
	ID string `json:"id"`
	AccountID string `json:"account_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
}

// Ticket 单笔成交的释冻进度。
type Ticket struct {
	FillID    string `json:"fill_id"`
	AccountID string `json:"account_id"`
	Asset      string `json:"asset"`
	FillAmount string `json:"fill_amount"`
	ReleaseAmt string `json:"release_amt"`
	Status      TicketStatus `json:"status"`
	JournalID   string       `json:"journal_id,omitempty"`
	Reason      string       `json:"reason,omitempty"`
	UpdatedUnix int64        `json:"updated_unix"`
	JournalDone bool         `json:"journal_done"`
	FreezeDone bool `json:"freeze_done"`
}

// ApplyResult 成交处理后的会话快照。
type ApplyResult struct {
	Ticket Ticket `json:"ticket"`
	Episode    Episode `json:"episode"`
	Idempotent bool    `json:"idempotent"`
}

// Config 释冻配置。路径禁止回退已释金额。
type Config struct {
	// AllowRollback 仅夹具可开。必须为 false。
	AllowRollback bool
	// Production 锁死：为 true 时无视 AllowRollback。
	Production bool
}

// ProductionConfig 配置：禁止回退路径。

  func ProductionConfig() Config {
	   return Config{AllowRollback: false, Production: true}
  }

   func (c Config) RollbackForbidden() bool {
	  return c.Production || !c.AllowRollback
   }

   func (c Config) Validate() error {
	  if c.Production && c.AllowRollback {
		   return fmt.Errorf("production config forbids rollback path")
	   }
	  return nil
   }

  func normalizeID(s string) string {
	  return strings.TrimSpace(s)
   }

   func ReleaseIdemKey(fillID string) string {
	  return "liq-partial-release:" + normalizeID(fillID)
  }

  func nowUnix() int64 { return time.Now().UTC().Unix() }
