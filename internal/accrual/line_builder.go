package accrual

import "strings"

// LineSpec 分录对规格。
type LineSpec struct {
	Kind string
	DebitUserID  string
	CreditUserID string
	Asset      string
	Amount     string
	MovementID string
	Note       string
}

// LineBuilder 链式分录规格构建器。

type LineBuilder struct {
	spec LineSpec
	errs []string
}

// NewLineBuilder 初始化分录构建器。
  func NewLineBuilder(kind string) *LineBuilder {
	   return &LineBuilder{spec: LineSpec{Kind: strings.TrimSpace(kind)}}
  }

// DebitUser 设置借记用户 ID。
   func (b *LineBuilder) DebitUser(userID string) *LineBuilder {
	   if b != nil {
		   b.spec.DebitUserID = strings.TrimSpace(userID)
	   }
	  return b
   }

// CreditUser 设置贷记用户 ID。
   func (b *LineBuilder) CreditUser(userID string) *LineBuilder {
	  if b != nil {
		  b.spec.CreditUserID = strings.TrimSpace(userID)
	   }
	   return b
  }

// Asset 设置资产。
  func (b *LineBuilder) Asset(asset string) *LineBuilder {
	   if b != nil {
		  b.spec.Asset = strings.TrimSpace(asset)
	  }
	   return b
  }

// Amount 设置金额（正数字符串）。
  func (b *LineBuilder) Amount(amount string) *LineBuilder {
	  if b != nil {
		   b.spec.Amount = strings.TrimSpace(amount)
	  }
	  return b
   }

// Movement 设置在途 movement 引用。
   func (b *LineBuilder) Movement(movementID string) *LineBuilder {
	  if b != nil {
		  b.spec.MovementID = strings.TrimSpace(movementID)
	   }
	   return b
  }

// Note 设置分录备注。

   func (b *LineBuilder) Note(note string) *LineBuilder {
	   if b != nil {
		  b.spec.Note = strings.TrimSpace(note)
	  }
	  return b
   }

// Validate 校验分录规格完整性。

   func (b *LineBuilder) Validate() []string {
	   if b == nil {
		  return []string{"builder is nil"}
	  }
	   var errs []string
	   if b.spec.Kind == "" {
		   errs = append(errs, "kind is required")
	   }
	   validKinds := map[string]bool{
		   string(LineAccruedInterest): true,
		   string(LineAccruedFee):      true,
		   string(LineNettingDiff): true,
		  string(LineReversal): true,
	   }
	  if b.spec.Kind != "" && !validKinds[b.spec.Kind] {
		  errs = append(errs, "kind '"+b.spec.Kind+"' is invalid")
	   }
	  if b.spec.DebitUserID == "" {
		   errs = append(errs, "debit_user_id is required")
	   }
	   if b.spec.CreditUserID == "" {
		  errs = append(errs, "credit_user_id is required")
	  }
	   if b.spec.Asset == "" {
		   errs = append(errs, "asset is required")
	  }
	   if b.spec.Amount == "" {
		  errs = append(errs, "amount is required")
	   }
	   return errs
  }

// Build 返回构建的 LineSpec，若校验失败则返回 nil 和错误列表。

  func (b *LineBuilder) Build() (*LineSpec, []string) {
	  if errs := b.Validate(); len(errs) > 0 {
		   return nil, errs
	  }
	  cp := b.spec
	   return &cp, nil
   }

// BatchAddSpecs 批量向应计引擎追加分录对（LineSpec 切片）。
// 返回成功追加的分录对数量及第一个错误（若有）。
  func BatchAddSpecs(engine *Engine, batchID string, specs []LineSpec) (int, error) {
	   if engine == nil {
		  return 0, ErrEngineNil
	  }
	   added := 0
	  for _, s := range specs {
		  _, _, err := engine.AddLinePair(
			   batchID,
			   s.Kind,
			   s.DebitUserID,
			   s.CreditUserID,
			  s.Asset,
			   s.Amount,
			   s.MovementID,
			   s.Note,
		  )
		   if err != nil {
			   return added, err
		   }
		   added++
	  }
	   return added, nil
  }
