// 账本按用户号选择库。未挂额外库时仍用原来的那一个。
// 作者: jakson
// 日期: 2021.01.06
package ledger

  import (
  	"context"
  	"database/sql"
  	"errors"
  	"hash/fnv"
  	"strings"

  	"account-assets.kucoin.net/funding/internal/persist"
  	"github.com/shopspring/decimal"
  )

  type shardUserKey struct{}
  type shardIndexKey struct{}

  var errFound = errors.New("shard row found")

   // WithShardUser 把随后的账本读写固定到该用户所在的库。
   // 未挂额外库时始终使用原来的那一个库。用户号为空或为 system 时落在 0 号库。
   // 一笔事务只打开一个库。严禁把同一笔的两边余额写进两个库，再指望它们一起提交。
   // 系统科目跟随当前用户所在库。多个库时，系统余额在各库分别记账。
   func WithShardUser(ctx context.Context, userID string) context.Context {
   	if ctx == nil {
   		ctx = context.Background()
   	}
   	return context.WithValue(ctx, shardUserKey{}, strings.TrimSpace(userID))
   }

   // WithShardIndex 指定片号。扫系统科目各片余额时使用。
   func WithShardIndex(ctx context.Context, index int) context.Context {
   	if ctx == nil {
   		ctx = context.Background()
   	}
   	return context.WithValue(ctx, shardIndexKey{}, index)
   }

// AddShards 挂上额外的账本库。未挂时全部读写仍在原来的库上，行为与单库一致。
func (s *SQLStore) AddShards(extra []*sql.DB) {
	if s == nil {
		return
	}
	for _, db := range extra {
		if db != nil && db != s.db {
			s.extra = append(s.extra, db)
		}
	}
}

  func (s *SQLStore) ShardCount() int {
  	return len(s.dbs())
  }

  func (s *SQLStore) dbs() []*sql.DB {
  	if s == nil || s.db == nil {
  		return nil
  	}
  	if len(s.extra) == 0 {
  		return []*sql.DB{s.db}
  	}
  	out := make([]*sql.DB, 0, 1+len(s.extra))
  	out = append(out, s.db)
  	for _, db := range s.extra {
  		if db != nil {
  			out = append(out, db)
  		}
  	}
  	return out
  }

  func (s *SQLStore) pickDB(ctx context.Context) *sql.DB {
  	dbs := s.dbs()
  	if len(dbs) == 0 {
  		return nil
  	}
  	if len(dbs) == 1 {
  		return dbs[0]
  	}
  	if ctx != nil {
  		if n, ok := ctx.Value(shardIndexKey{}).(int); ok && n >= 0 && n < len(dbs) {
  			return dbs[n]
  		}
  		if user, ok := ctx.Value(shardUserKey{}).(string); ok {
  			return dbs[shardIndex(user, len(dbs))]
  		}
  	}
  	return dbs[0]
  }

   // retryMoney 决定资金事务是否整笔重开。
   // 死锁、快照冲突、余额条件更新失败可以重开。
   // 余额不足、风控拒绝、参数错误不得重开。
   func retryMoney(err error) bool {
   	return persist.RetryableTxConflict(err) || errors.Is(err, ErrStaleBook)
   }

  func dupKey(err error) bool {
  	if err == nil {
  		return false
  	}
  	msg := strings.ToLower(err.Error())
  	return strings.Contains(msg, "duplicate")
  }

// ShardSlot 返回 user 在 n 个分片中的稳定下标（system 固定 0）。
func ShardSlot(user string, n int) int {
	return shardIndex(user, n)
}

  func shardIndex(user string, n int) int {
  	if n <= 1 {
  		return 0
  	}
  	user = strings.ToLower(strings.TrimSpace(user))
  	if user == "" || user == "system" {
  		return 0
  	}
  	h := fnv.New64a()
  	_, _ = h.Write([]byte(user))
  	return int(h.Sum64() % uint64(n))
  }

func (s *SQLStore) visit(ctx context.Context, fn func(queryer) error) error {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok && tx != nil {
		return fn(s.conn(ctx))
	}
	var last error
	for i := range s.dbs() {
		err := fn(s.conn(WithShardIndex(ctx, i)))
		if errors.Is(err, errFound) {
			return err
		}
		if err != nil {
			last = err
		}
	}
	return last
}

  // SumSystem 把各片上的系统科目可用额加总。热钱包核对用总和。
  func (s *SQLStore) SumSystem(ctx context.Context, typ AccountType, asset string) (decimal.Decimal, error) {
  	sum := decimal.Zero
  	if s == nil {
  		return sum, nil
  	}
  	for i := range s.dbs() {
  		row := s.conn(WithShardIndex(ctx, i)).QueryRowContext(ctx, `SELECT available FROM ledger_accounts WHERE user_id=? AND account_type=? AND asset=?`, "system", typ, asset)
  		var raw string
  		err := row.Scan(&raw)
  		if errors.Is(err, sql.ErrNoRows) {
  			continue
  		}
  		if err != nil {
  			return decimal.Zero, err
  		}
  		part, err := ParseBook(raw)
  		if err != nil {
  			return decimal.Zero, err
  		}
  		sum = sum.Add(part)
  	}
  	return sum, nil
  }
