// 资金事务的隔离级别与可重开错误。
// 作者: Knox
// 日期: 2020.09.11
package persist

   import (
   	"context"
   	"database/sql"
   	"errors"
   	"strings"
   	"time"

   	"github.com/go-sql-driver/mysql"
   	"github.com/lib/pq"
   )

   // MoneyTxOptions 资金变更使用的事务选项，隔离级别为可重复读。
   // 同一事务内再次读取同一行，看到的是本事务开始时的代次。
   // 余额是否写成功由条件更新决定，不靠把科目行锁住排队。
   // 不使用可串行化。发生锁冲突时由调用方整笔回滚后重开。
   func MoneyTxOptions() *sql.TxOptions {
   	return &sql.TxOptions{Isolation: sql.LevelRepeatableRead}
   }

   // RetryableTxConflict 判断这笔资金事务能否整笔重开。
   // MySQL 1213、Postgres 40001 与 40P01 属于死锁或快照冲突，可以重开。
   // 锁等待超时不在此列，调用方应直接失败。
   // 严禁把余额不足、风控拒绝、参数错误当成可重试错误。
   func RetryableTxConflict(err error) bool {
   	if err == nil {
   		return false
   	}
   	var me *mysql.MySQLError
   	if errors.As(err, &me) && me.Number == 1213 {
   		return true
   	}
   	var pe *pq.Error
   	if errors.As(err, &pe) {
   		switch string(pe.Code) {
   			case "40001", "40P01":
   			return true
   		}
   	}
   	msg := strings.ToLower(err.Error())
   	return strings.Contains(msg, "deadlock") || strings.Contains(msg, "could not serialize")
   }

   // SleepTxRetry 在重开前按次数短暂停顿，上下文取消时立即返回。
   // 停顿只为错开同时冲突的事务，不得在这里做业务判断。
   func SleepTxRetry(ctx context.Context, attempt int) error {
   	if attempt < 1 {
   		attempt = 1
   	}
   	d := time.Duration(attempt) * 2 * time.Millisecond
   	t := time.NewTimer(d)
   	defer t.Stop()
   	select {
   		case <-ctx.Done():
   		return ctx.Err()
   		case <-t.C:
   		return nil
   	}
   }
