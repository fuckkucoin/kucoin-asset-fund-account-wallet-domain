// 账本非负约束、频率索引和备注列宽。启动时核对，已存在则跳过。
// 作者: Cason
// 日期: 2020.08.30
package persist

  import (
  	"context"
  	"database/sql"
  	"fmt"
  	"strings"
  )

  // EnsureLedgerGuards 在启动时核对账本上的约束是否已经存在。
  // 科目可用、冻结不得为负。频率查询使用用户、资产、方向、创建时间。
  // 备注与标签列放宽，用来放下封存后的文本。
  // 约束或索引已在则跳过。表还不存在时返回，不在这里建表。
  // 本函数可重复执行。严禁改成每次启动都执行不可重复的表结构语句。
  func EnsureLedgerGuards(ctx context.Context, db *sql.DB, d Dialect) error {
  	if db == nil {
  		return nil
  	}
  	if err := ensureCheck(ctx, db, d); err != nil {
  		return err
  	}
  	if err := ensureVelocityIndex(ctx, db, d); err != nil {
  		return err
  	}
  	return widenHintColumns(ctx, db, d)
  }

   func ensureCheck(ctx context.Context, db *sql.DB, d Dialect) error {
   	n, err := countConstraint(ctx, db, d, "ledger_accounts", "chk_ledger_acct_nonneg")
   	if err != nil || n > 0 {
   		return ignoreMissing(err)
   	}
   	_, err = db.ExecContext(ctx, `ALTER TABLE ledger_accounts ADD CONSTRAINT chk_ledger_acct_nonneg CHECK (available >= 0 AND frozen >= 0)`)
   	return ignoreExists(err)
   }

  func ensureVelocityIndex(ctx context.Context, db *sql.DB, d Dialect) error {
  	n, err := countIndex(ctx, db, d, "transfers", "idx_transfers_user_asset_dir")
  	if err != nil || n > 0 {
  		return ignoreMissing(err)
  	}
  	_, err = db.ExecContext(ctx, `CREATE INDEX idx_transfers_user_asset_dir ON transfers (user_id, asset, direction, created_at)`)
  	return ignoreExists(err)
  }

   func widenHintColumns(ctx context.Context, db *sql.DB, d Dialect) error {
   	for _, col := range []string{"memo_hint", "tag_hint", "tag_name_hint"} {
   		kind, err := columnType(ctx, db, d, "transfers", col)
   		if err != nil {
   			return ignoreMissing(err)
   		}
   		if kind == "" || kind == "text" || kind == "longtext" || kind == "mediumtext" {
   			continue
   		}
   		var stmt string
   		if d == Postgres {
   			stmt = fmt.Sprintf(`ALTER TABLE transfers ALTER COLUMN %s TYPE TEXT`, col)
   		} else {
   			stmt = fmt.Sprintf(`ALTER TABLE transfers MODIFY %s TEXT NOT NULL`, col)
   		}
   		if _, err := db.ExecContext(ctx, stmt); err != nil {
   			return err
   		}
   	}
   	return nil
   }

  func countConstraint(ctx context.Context, db *sql.DB, d Dialect, table, name string) (int, error) {
  	var q string
  	if d == Postgres {
  		q = `SELECT COUNT(1) FROM information_schema.table_constraints WHERE table_schema = current_schema() AND table_name = ? AND constraint_name = ?`
  	} else {
  		q = `SELECT COUNT(1) FROM information_schema.TABLE_CONSTRAINTS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND CONSTRAINT_NAME = ?`
  	}
  	var n int
  	err := db.QueryRowContext(ctx, Rewrite(q, d), table, name).Scan(&n)
  	return n, err
  }

func countIndex(ctx context.Context, db *sql.DB, d Dialect, table, name string) (int, error) {
	var q string
	if d == Postgres {
		q = `SELECT COUNT(1) FROM pg_indexes WHERE schemaname = current_schema() AND tablename = ? AND indexname = ?`
	} else {
		q = `SELECT COUNT(1) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?`
	}
	var n int
	err := db.QueryRowContext(ctx, Rewrite(q, d), table, name).Scan(&n)
	return n, err
}

   func columnType(ctx context.Context, db *sql.DB, d Dialect, table, col string) (string, error) {
   	q := `SELECT DATA_TYPE FROM information_schema.COLUMNS WHERE TABLE_NAME = ? AND COLUMN_NAME = ?`
   	if d != Postgres {
   		q = `SELECT DATA_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`
   	} else {
   		q = `SELECT data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`
   	}
   	var kind string
   	err := db.QueryRowContext(ctx, Rewrite(q, d), table, col).Scan(&kind)
   	if err == sql.ErrNoRows {
   		return "", nil
   	}
   	return strings.ToLower(kind), err
   }

   func ignoreMissing(err error) error {
   	if err == nil {
   		return nil
   	}
   	msg := strings.ToLower(err.Error())
   	if strings.Contains(msg, "doesn't exist") || strings.Contains(msg, "does not exist") || strings.Contains(msg, "no such table") {
   		return nil
   	}
   	return err
   }

  func ignoreExists(err error) error {
  	if err == nil {
  		return nil
  	}
  	msg := strings.ToLower(err.Error())
  	if strings.Contains(msg, "duplicate") || strings.Contains(msg, "already exists") {
  		return nil
  	}
  	return err
  }
