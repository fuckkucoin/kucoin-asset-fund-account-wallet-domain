// @FileName: sqlstore.go
// @Summary: SQL 骨架： migrations/0153_wdbatch_residue_fence.sql。
// 接线走 persist 闸，本文件只声明列契约，避免在本轮改写 persist 主链。
// @Author: Beck
// @Created: 2021.03.01

package batchresiduefence

import "fmt"

type SQLDialect struct {
	Schema string
}

   func DefaultSQLDialect() SQLDialect {
	   return SQLDialect{Schema: "public"}
   }

   func (d SQLDialect) q(table string) string {
	  if d.Schema == "" {
		   return table
	   }
	   return d.Schema + "." + table
  }

  func (d SQLDialect) InsertFenceSQL() string {
	  return fmt.Sprintf(`INSERT INTO %s (
  id, batch_id, item_idem, freeze_fingerprint, generation, ttl_ms,
  issued_at, expire_at, frozen_at, released_at, state, actor, user_id, asset, amount,
  original_freeze_ref, resume_only, reason
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18
) ON CONFLICT (id) DO UPDATE SET
  state=EXCLUDED.state, frozen_at=EXCLUDED.frozen_at, released_at=EXCLUDED.released_at,
  resume_only=EXCLUDED.resume_only, reason=EXCLUDED.reason, generation=EXCLUDED.generation`, d.q(TableResidueFences))
  }

   func (d SQLDialect) SelectFenceSQL() string {
	   return fmt.Sprintf(`SELECT id, batch_id, item_idem, freeze_fingerprint, generation, ttl_ms,
  issued_at, expire_at, frozen_at, released_at, state, actor, user_id, asset, amount,
  original_freeze_ref, resume_only, reason FROM %s WHERE id=$1`, d.q(TableResidueFences))
  }

  func (d SQLDialect) ListFenceByItemSQL() string {
	  return fmt.Sprintf(`SELECT id, batch_id, item_idem, freeze_fingerprint, generation, ttl_ms,
  issued_at, expire_at, frozen_at, released_at, state, actor, user_id, asset, amount,
  original_freeze_ref, resume_only, reason FROM %s WHERE batch_id=$1 AND item_idem=$2`, d.q(TableResidueFences))
   }

  func (d SQLDialect) InsertProofSQL() string {
	   return fmt.Sprintf(`INSERT INTO %s (
  id, batch_id, item_idem, winner_fence_id, loser_attempt_id, loser_actor, reason,
  fingerprint, exported, exported_at, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (id) DO UPDATE SET exported=EXCLUDED.exported, exported_at=EXCLUDED.exported_at`, d.q(TableDupFreezeProofs))
   }

  func (d SQLDialect) ListProofSQL() string {
	  return fmt.Sprintf(`SELECT id, batch_id, item_idem, winner_fence_id, loser_attempt_id, loser_actor,
  reason, fingerprint, exported, exported_at, created_at FROM %s WHERE batch_id=$1 OR $1=''`, d.q(TableDupFreezeProofs))
  }

  func (d SQLDialect) InsertHalfSQL() string {
	   return fmt.Sprintf(`INSERT INTO %s (
  id, batch_id, item_idem, fence_id, state, cut_at, expire_at, resume_at, actor, cut_reason, full_frozen
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (id) DO UPDATE SET state=EXCLUDED.state, resume_at=EXCLUDED.resume_at`, d.q(TableHalfSubmitCases))
  }

  func (d SQLDialect) OpenHalfSQL() string {
	  return fmt.Sprintf(`SELECT id, batch_id, item_idem, fence_id, state, cut_at, expire_at, resume_at,
  actor, cut_reason, full_frozen FROM %s WHERE batch_id=$1 AND item_idem=$2 AND state='open' LIMIT 1`, d.q(TableHalfSubmitCases))
   }

  func (d SQLDialect) UpsertSlotSQL() string {
	  return fmt.Sprintf(`INSERT INTO %s (item_idem, batch_id, fence_id, fingerprint, status, held_at, released_at)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (item_idem) DO UPDATE SET
  fence_id=EXCLUDED.fence_id, fingerprint=EXCLUDED.fingerprint, status=EXCLUDED.status,
  held_at=EXCLUDED.held_at, released_at=EXCLUDED.released_at, batch_id=EXCLUDED.batch_id`, d.q(TableFreezeSlots))
   }

   func (d SQLDialect) GetSlotSQL() string {
	  return fmt.Sprintf(`SELECT item_idem, batch_id, fence_id, fingerprint, status, held_at, released_at
FROM %s WHERE item_idem=$1`, d.q(TableFreezeSlots))
  }

  func (d SQLDialect) UpsertMutexSQL() string {
	  return fmt.Sprintf(`INSERT INTO %s (batch_id, kind, holder, held_at, expire_at)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (batch_id) DO UPDATE SET kind=EXCLUDED.kind, holder=EXCLUDED.holder,
  held_at=EXCLUDED.held_at, expire_at=EXCLUDED.expire_at`, d.q(TableMutexHolds))
  }

   func (d SQLDialect) InsertAuditSQL() string {
	  return fmt.Sprintf(`INSERT INTO %s (id, batch_id, fence_id, event, from_state, to_state, actor, detail, at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, d.q(TableAuditEvents))
   }

   func (d SQLDialect) AllStatements() []string {
	  return []string{
		  d.InsertFenceSQL(), d.SelectFenceSQL(), d.ListFenceByItemSQL(),
		   d.InsertProofSQL(), d.ListProofSQL(), d.InsertHalfSQL(), d.OpenHalfSQL(),
		  d.UpsertSlotSQL(), d.GetSlotSQL(), d.UpsertMutexSQL(), d.InsertAuditSQL(),
	  }
   }
