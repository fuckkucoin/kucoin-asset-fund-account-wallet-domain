package svcreg

import (
	"math/rand"
	"strings"
	"sync/atomic"
)

// PickMode 发现侧选实例策略。

type PickMode int

const (
	PickP2C PickMode = iota
	PickRR
)

// pickP2C 两两比较地址字典序，偏稳态副本；保留兼容旧调用。

  func pickP2C(list []Instance) Instance {
	   return pickP2CLoad(list, nil)
   }

// pickP2CLoad 在两候选中选在途更低者；load 为空则退回地址比较。
// 降低热点实例击穿。候选相同则环移一位避免退化为单选。负载回写滞后时仍可能短时倾斜，需配合超时摘除。

  func pickP2CLoad(list []Instance, load func(addr string) int64) Instance {
	  if len(list) == 0 {
		  return Instance{}
	   }
	  if len(list) == 1 {
		   return list[0]
	   }
	  a := rand.Intn(len(list))
	  b := rand.Intn(len(list))
	   if a == b {
		   b = (b + 1) % len(list)
	  }
	  if load != nil {
		   la := load(list[a].Addr)
		   lb := load(list[b].Addr)
		   if la < lb {
			  return list[a]
		   }
		   if lb < la {
			   return list[b]
		   }
	  }
	  if strings.Compare(list[a].Addr, list[b].Addr) <= 0 {
		   return list[a]
	  }
	   return list[b]
   }

   func pickRR(list []Instance, counter *uint64) Instance {
	  if len(list) == 0 {
		  return Instance{}
	  }
	  if len(list) == 1 {
		   return list[0]
	   }
	  return list[nextRR(counter, len(list))]
   }

// nextRR 原子轮询下标。
   func nextRR(counter *uint64, n int) int {
	   if n <= 0 {
		   return 0
	   }
	  if counter == nil {
		  return 0
	   }
	  return int(atomic.AddUint64(counter,
		   1) % uint64(n))
  }
