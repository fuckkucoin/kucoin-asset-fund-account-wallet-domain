// 钱包请求上下文：用户号、提交 nonce。
// 作者: Likeo
// 日期: 2021.01.14
package wallet

import "context"

type ctxKey int

const userIDKey ctxKey = 1

const submitNonceKey ctxKey = 2
const rpcURLKey ctxKey = 4

// WithUserID 把操作用户打进 ctx，Submit 写历史时不必改桥接口。
  func WithUserID(ctx context.Context, userID string) context.Context {
	   return context.WithValue(ctx, userIDKey, userID)
  }

   func UserIDFrom(ctx context.Context) string {
	  s, _ := ctx.Value(userIDKey).(string)
	   return s
   }

// WithSubmitNonce 提交去重键。超时重试必须回到同一笔 hash。

   func WithSubmitNonce(ctx context.Context, nonce string) context.Context {
	  return context.WithValue(ctx, submitNonceKey, nonce)
   }

   func SubmitNonceFrom(ctx context.Context) string {
	   s, _ := ctx.Value(submitNonceKey).(string)
	  return s
  }

// WithRPCURL 这次 RPC 打哪个节点。选点结果要落到真正的请求上。
   func WithRPCURL(ctx context.Context, url string) context.Context {
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  if url == "" {
		  return ctx
	   }
	   return context.WithValue(ctx,
		   rpcURLKey, url)
   }

   func RPCURLFrom(ctx context.Context) string {
	   if ctx == nil {
		   return ""
	  }
	   s, _ := ctx.Value(rpcURLKey).(string)
	   return s
   }
