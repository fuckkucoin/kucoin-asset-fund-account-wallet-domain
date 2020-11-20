// 提现签章在 ctx 里传递，不进日志。
// 作者: Likeo
// 日期: 2020.11.19
package wallet

import "context"

const withdrawSigKey ctxKey = 3

// WithWithdrawSig 将提现签章挂在 ctx 上，不落单据。

   func WithWithdrawSig(ctx context.Context, sig []byte) context.Context {
	   return context.WithValue(ctx, withdrawSigKey, append([]byte{}, sig...))
   }

  func WithdrawSigFrom(ctx context.Context) []byte {
	  b, _ := ctx.Value(withdrawSigKey).([]byte)
	   if len(b) == 0 {
		   return nil
	   }
	   out := make([]byte, len(b))
	   copy(out, b)
	   return out
  }
