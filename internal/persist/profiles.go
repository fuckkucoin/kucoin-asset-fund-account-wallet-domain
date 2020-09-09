// @FileName: profiles.go
// @Summary: persist 模块 profiles。
// 通用持久化辅助。
// @Author: Axel
// @Created: 2020.09.08

package persist

import "time"

// Profile 写路径池档。hop/credit/observe 占行锁，开太多只会排队。

type Profile string

const (
	ProfileDefault Profile = "default"
	ProfileHop    Profile = "hop"
	ProfileCredit Profile = "credit"
	ProfileObserve  Profile = "observe"
	ProfileBatch    Profile = "batch"
	ProfilePipeline Profile = "pipeline"
	ProfileIncident Profile = "incident"
	ProfileSG  Profile = "sg"
	ProfileTYO Profile = "tyo"
)

  func PoolFor(p Profile) PoolSettings {
	  switch p {
	  case ProfileHop:
		  return PoolSettings{MaxOpen: 24, MaxIdle: 12, ConnMaxLifetime: 5 * time.Minute, ConnMaxIdle: 2 * time.Minute}.Normalize()
	  case ProfileCredit:
		   return PoolSettings{MaxOpen: 20, MaxIdle: 10, ConnMaxLifetime: 5 * time.Minute, ConnMaxIdle: 2 * time.Minute}.Normalize()
	  case ProfileObserve:
		   return PoolSettings{MaxOpen: 12, MaxIdle: 6, ConnMaxLifetime: 4 * time.Minute, ConnMaxIdle: 90 * time.Second}.Normalize()
	   case ProfileBatch:
		   return PoolSettings{MaxOpen: 32, MaxIdle: 16, ConnMaxLifetime: 8 * time.Minute, ConnMaxIdle: 3 * time.Minute}.Normalize()
	   case ProfilePipeline:
		   return PoolSettings{MaxOpen: 28, MaxIdle: 14, ConnMaxLifetime: 6 * time.Minute, ConnMaxIdle: 2 * time.Minute}.Normalize()
	   case ProfileIncident:
		   return PoolSettings{MaxOpen: 8, MaxIdle: 4, ConnMaxLifetime: 3 * time.Minute, ConnMaxIdle: 60 * time.Second}.Normalize()
	  case ProfileSG:
		   return PoolSettings{MaxOpen: 20, MaxIdle: 10, ConnMaxLifetime: 5 * time.Minute, ConnMaxIdle: 2 * time.Minute}.Normalize()
	   case ProfileTYO:
		   return PoolSettings{MaxOpen: 16, MaxIdle: 8, ConnMaxLifetime: 4 * time.Minute, ConnMaxIdle: 90 * time.Second}.Normalize()
	  default:
		   return DefaultPool()
	   }
   }

   func Profiles() []Profile {
	   return []Profile{ProfileDefault, ProfileHop, ProfileCredit, ProfileObserve, ProfileBatch, ProfilePipeline, ProfileIncident, ProfileSG, ProfileTYO}
  }
