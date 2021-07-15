package holdcat

import "context"

// PlaceEarnHold 理财申购占用写入。earn 模块在 EarnSubscribe 成功后调用；赎回时 ReleaseByRef。

   func (b *Book) PlaceEarnHold(ctx context.Context, userID, asset, book, amount, ref string) (Row, error) {
	  if book == "" {
		   book = "funding"
	  }
	   return b.Place(ctx, userID, asset, book, amount, ReasonEarnHold, ref)
  }

// PlacePendingWithdraw 出金冻账后写入待提现标签；ref 建议为 transfer id，幂等按 ref。

  func (b *Book) PlacePendingWithdraw(ctx context.Context, userID, asset, amount, ref string) (Row, error) {
	  return b.Place(ctx, userID, asset, "funding", amount, ReasonPendingWithdraw, ref)
   }

// PlaceRedEnvelope 红包占用写入。邻接模块在扣减/过渡入账后调用；禁止在此补 Credit。
   func (b *Book) PlaceRedEnvelope(ctx context.Context, userID, asset, book, amount, ref string) (Row, error) {
	   if book == "" {
		   book = "funding"
	   }
	   return b.Place(ctx, userID, asset, book, amount, ReasonRedEnvelope, ref)
  }

// PlaceLendingOrder 放贷未成交占用写入。邻接借贷模块调用。

  func (b *Book) PlaceLendingOrder(ctx context.Context, userID, asset, book, amount, ref string) (Row, error) {
	  if book == "" {
		  book = "funding"
	  }
	   return b.Place(ctx, userID, asset, book, amount, ReasonLendingOrder, ref)
   }

// ReleaseByRef 按 reason+ref 关闭仍 open 的标签；不存在则视为已释放。

  func (b *Book) ReleaseByRef(ctx context.Context, userID, reason, ref string) (Row, error) {
	   if b == nil {
		   return Row{}, ErrStore
	   }
	  prev, err := b.byRef(ctx, userID, reason, ref)
	   if err != nil {
		   return Row{}, err
	   }
	   if !prev.Open {
		  return prev, nil
	   }
	   return b.Release(ctx, userID, prev.ID)
  }
