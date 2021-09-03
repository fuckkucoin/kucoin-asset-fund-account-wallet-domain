package ledshard

import (
	"fmt"
	"strings"
)

// ValidateRoute 拒绝把系统片放进用户环，也拒绝亲和指到系统片。
   func ValidateRoute(r RouteVersion, shards map[string]Shard) error {
	  if r.Version <= 0 {
		  return fmt.Errorf("%w: version", ErrRouteMissing)
	   }
	  if len(r.UserShards) == 0 {
		  return ErrNoRing
	   }
	   if r.Modulus != len(r.UserShards) {
		   return fmt.Errorf("%w: modulus %d ring %d", ErrNoRing, r.Modulus, len(r.UserShards))
	  }
	  if len(r.Pins) == 0 {
		   return ErrUnpinned
	  }
	   for _, sub := range []string{SubBridge, SubFee, SubCold} {
		   id := r.Pins[sub]
		   if id == "" {
			  return fmt.Errorf("%w: %s", ErrUnpinned, sub)
		   }
		   sh, ok := shards[id]
		  if !ok {
			   return fmt.Errorf("%w: shard %s", ErrUnpinned, id)
		  }
		  if sh.Kind != KindSystem || sh.PinSubject != sub {
			  return fmt.Errorf("%w: %s pin %s", ErrUnpinned, sub, id)
		  }
		  if containsShard(r.UserShards, id) {
			  return fmt.Errorf("%w: %s", ErrPinLeak, id)
		  }
	   }
	  seen := map[string]struct{}{}
	   for _, id := range r.UserShards {
		   if _, dup := seen[id]; dup {
			   return fmt.Errorf("%w: duplicate %s", ErrNoRing, id)
		  }
		   seen[id] = struct{}{}
		   sh, ok := shards[id]
		   if !ok {
			  return fmt.Errorf("%w: missing %s", ErrNoRing, id)
		   }
		   if sh.Kind != KindUser {
			   return fmt.Errorf("%w: %s", ErrPinLeak, id)
		  }
		   if sh.PinSubject != "" {
			   return fmt.Errorf("%w: user shard pinned %s", ErrPinLeak, id)
		   }
		  if _, isPin := pinShardIDs(r.Pins)[id]; isPin {
			  return fmt.Errorf("%w: %s", ErrPinLeak, id)
		   }
	  }
	   owned := pinShardIDs(r.Pins)
	  for asset, id := range r.Affinity {
		   asset = strings.ToUpper(strings.TrimSpace(asset))
		   if asset == "" || id == "" {
			  return ErrAffinity
		  }
		  if _, isPin := owned[id]; isPin {
			   return fmt.Errorf("%w: affinity %s", ErrPinLeak, asset)
		  }
		   if !containsShard(r.UserShards, id) {
			  return fmt.Errorf("%w: %s -> %s", ErrAffinity, asset, id)
		   }
	  }
	  if r.Fingerprint != "" && r.Fingerprint != Fingerprint(r) {
		  return fmt.Errorf("%w: fingerprint", ErrRouteImmutable)
	   }
	   return nil
  }

// Resolve 按钉扎、亲和、用户哈希的顺序选片。钉扎先于亲和和哈希。
  func Resolve(route RouteVersion, user, book, asset string) (Assignment, error) {
	  if route.Version <= 0 || route.Fingerprint == "" {
		   return Assignment{}, ErrRouteMissing
	  }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   if asset == "" {
		  return Assignment{}, ErrAsset
	  }
	   subject, isSys := SubjectOf(user, book)
	  if subject == "" {
		  if strings.TrimSpace(user) == "" {
			   return Assignment{}, ErrUser
		  }
		  return Assignment{}, ErrBook
	  }
	  out := Assignment{Version: route.Version, Subject: subject}
	  if isSys {
		   id := route.Pins[subject]
		   if id == "" {
			  return Assignment{}, fmt.Errorf("%w: %s", ErrUnpinned, subject)
		  }
		   if containsShard(route.UserShards, id) {
			  return Assignment{}, fmt.Errorf("%w: %s", ErrPinLeak, subject)
		   }
		  out.ShardID = id
		   out.Pin = subject
		   out.Slot = -1
		   return out, nil
	  }
	  if id, ok := route.Affinity[asset]; ok && id != "" {
		   if _, isPin := pinShardIDs(route.Pins)[id]; isPin || containsShard([]string{}, id) {
			  return Assignment{}, ErrPinLeak
		   }
		   if pinShardIDs(route.Pins)[id] != "" {
			  return Assignment{}, fmt.Errorf("%w: %s", ErrPinLeak, asset)
		  }
		   if !containsShard(route.UserShards, id) {
			  return Assignment{}, fmt.Errorf("%w: %s", ErrAffinity, asset)
		  }
		   out.ShardID = id
		  out.Affinity = asset
		  out.Slot = indexOf(route.UserShards,
			  id)
		  return out, nil
	  }
	   if len(route.UserShards) == 0 {
		  return Assignment{}, ErrNoRing
	   }
	  slot := UserSlot(user,
		   len(route.UserShards))
	   id := route.UserShards[slot]
	   if pinShardIDs(route.Pins)[id] != "" {
		   return Assignment{}, fmt.Errorf("%w: slot %d", ErrPinLeak, slot)
	   }
	  out.ShardID = id
	   out.Slot = slot
	  out.Hashed = true
	  return out, nil
   }

  func indexOf(list []string, id string) int {
	  for i, s := range list {
		  if s == id {
			   return i
		  }
	  }
	  return -1
  }

// SamePlacement 比较两侧解析是否落在同一片。同用户同资产在同一代次下为真。
   func SamePlacement(a, b Assignment) bool {
	  return a.ShardID != "" && a.ShardID == b.ShardID
  }
