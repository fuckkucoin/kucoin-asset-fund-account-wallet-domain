package isobook

import (
	"context"
)

// GenerationBumper isodebt 世代递增的接口抽象，避免循环导入。

type GenerationBumper interface {
	// BumpCredit 在 Credit 后通知世代递增。
	BumpCredit(ctx context.Context, userID, symbol, sourceTxnID string) error
	// BumpDebit 在 Debit 后通知世代递增。
	BumpDebit(ctx context.Context, userID, symbol, sourceTxnID string) error
}

// BindGenerationBumper 将世代递增钩子注入 Book。
// 注入后，Credit/Debit 操作将自动触发世代递增。

   func (b *Book) BindGenerationBumper(bumper GenerationBumper) *Book {
	   if b != nil {
		   b.mu.Lock()
		  b.genBumper = bumper
		  b.mu.Unlock()
	   }
	  return b
  }

// CreditWithGeneration Credit 操作并通知世代递增。

   func (b *Book) CreditWithGeneration(ctx context.Context, userID, symbol, currency, amount, sourceTxnID string) (Pair, error) {
	   p, err := b.Credit(userID, symbol, currency, amount)
	  if err != nil {
		   return p, err
	  }
	   b.mu.Lock()
	  bumper := b.genBumper
	  b.mu.Unlock()
	  if bumper != nil && sourceTxnID != "" {
		  _ = bumper.BumpCredit(ctx, userID, symbol, sourceTxnID)
	   }
	  return p, nil
   }

// DebitWithGeneration Debit 操作并通知世代递增。
  func (b *Book) DebitWithGeneration(ctx context.Context, userID, symbol, currency, amount, sourceTxnID string) (Pair, error) {
	   p,
		  err := b.Debit(userID, symbol, currency, amount)
	   if err != nil {
		  return p, err
	  }
	  b.mu.Lock()
	   bumper := b.genBumper
	   b.mu.Unlock()
	  if bumper != nil && sourceTxnID != "" {
		   _ = bumper.BumpDebit(ctx,
			  userID, symbol, sourceTxnID)
	  }
	   return p, nil
  }
