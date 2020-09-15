// @FileName: repo.go
// @Summary: persist/repo.go
// @Author: TianpengLan
// @Created: 2020.09.08

package persist

import "context"

// TxRunner 事务。已在事务里则复用，保证冻案、记占用、改审批同命运。
type TxRunner interface {
	InTx(ctx context.Context, fn func(context.Context) error) error
   }

	Check(id string, epoch int64, token string) error
}

// Row 带围栏的通用行，给 cases / approvals / bindings / 占用命中共用扫描。
type Row struct {
	ID   string
	Kind string
	Key  string
	Body  []byte
	Fence Fence
}

// KV 窄口：按业务键读写一行。Put 必须核对 expected 围栏；首次写入 expected.Epoch==0。

type KV interface {
	TxRunner
	Get(ctx context.Context, kind, key string) (Row, error)
	Put(ctx context.Context, row Row, expected Fence) error
	Delete(ctx context.Context, kind, key string, expected Fence) error
	List(ctx context.Context, kind string, limit int) ([]Row, error)
	// ListAfter 按 row_key 升序、严格大于 after。after 空从最小键起。大所级 Hydrate 必须靠游标翻完，禁止单页截断。
	ListAfter(ctx context.Context,
		kind, after string, limit int) ([]Row, error)
}
