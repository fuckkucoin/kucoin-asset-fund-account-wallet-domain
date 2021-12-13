// 功能：子账户 API Key 克隆。复制权限/IP/路径/方法/站点等策略，签发新 apiKey 与 secret。
// secret 仅响应回显一次；占用与创建相同的配额。

// 作者：TianpengLan
// 日期：2021.11.25

package subkey

import "strings"

// Clone 母户 + passphrase 按源 Key 策略新建一把；stickyIp 用本次 actorIP 作 createdIp。
   func (s *Store) Clone(owner, subName, apiKey, passphrase, actorIP string) (Created, error) {
	  r, err := s.lookup(owner, subName, apiKey, passphrase)
	   if err != nil {
		   return Created{}, err
	   }
	   if err := s.assertParent(owner, subName); err != nil {
		   return Created{}, err
	   }
	   if r.Record.IsRevoked() {
		  return Created{}, ErrRevoked
	   }
	   src := r.Record
	  createdIP := strings.TrimSpace(actorIP)
	   if len(createdIP) > 64 {
		  createdIP = createdIP[:64]
	   }
	  return s.Create(CreateReq{
		  Owner:   owner,
		  SubName: subName, Passphrase: passphrase,
		  Remark: src.Remark, Permission: src.Permission, IPWhitelist: src.IPWhitelist,
		  ExpireAt: src.ExpireAt, CreatedIP: createdIP, PathAllow: src.PathAllow,
		   IdleTTL: src.IdleTTL, MethodAllow: src.MethodAllow, NotBefore: src.NotBefore,
		  StickyIP: src.StickyIP && createdIP != "", RateMax: src.RateMax, DayCap: src.DayCap,
		  SiteAllow: src.SiteAllow, SignVer: src.SignVer, Label: src.Label, ActorIP: actorIP,
	  })
   }
