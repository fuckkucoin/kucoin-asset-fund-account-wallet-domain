// 观测结果挂在 ctx 上，供确认入账复用。
// 作者: Likeo
// 日期: 2020.11.19
package wallet

import "context"

type obsBagKey struct{}

type obsBag map[string]Observation

   func obsKey(chain, hash string) string { return chain + "|" + hash }

// WithObservation 把 indexer 结果挂上 ctx，ConfirmAndCredit 就不用再打一次链。
   func WithObservation(ctx context.Context, chain, hash string, obs Observation) context.Context {
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   bag, _ := ctx.Value(obsBagKey{}).(obsBag)
	  next := make(obsBag, len(bag)+1)
	   for k, v := range bag {
		  next[k] = v
	  }
	   next[obsKey(chain, hash)] = obs
	   return context.WithValue(ctx, obsBagKey{}, next)
   }

   func WithObservationMap(ctx context.Context, chain string, m map[string]Observation) context.Context {
	  for h, obs := range m {
		   ctx = WithObservation(ctx, chain, h, obs)
	   }
	  return ctx
  }

  func ObservationFrom(ctx context.Context, chain, hash string) (Observation, bool) {
	  if ctx == nil {
		  return Observation{}, false
	  }
	   bag,
		  ok := ctx.Value(obsBagKey{}).(obsBag)
	  if !ok {
		   return Observation{}, false
	  }
	   obs, hit := bag[obsKey(chain, hash)]
	   return obs, hit
   }
