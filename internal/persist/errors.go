// @FileName: errors.go
// @Summary: persist 包内 errors 实现。
// 持久化共用错误。传输层只做映射。
// @Author: Kinsley
// @Created: 2020.09.02

package persist

import "fmt"

var (
	ErrStaleFence = fmt.Errorf("stale persist fence")
	ErrConflict   = fmt.Errorf("persist conflict")
	ErrNotFound   = fmt.Errorf("persist row not found")
	ErrTxClosed   = fmt.Errorf("persist tx closed")
	ErrBadDSN    = fmt.Errorf("persist dsn invalid")
	ErrMigration = fmt.Errorf("persist migration")
	ErrRecon     = fmt.Errorf("persist recon drift")
	ErrMemRegister = fmt.Errorf("refuse production: register bound to in-process mem")
	ErrLeaseCols   = fmt.Errorf("lease table missing required columns")
	ErrDepositCols = fmt.Errorf("aa_deposits missing credit/recon columns or indexes")
	ErrWDOutboundCols = fmt.Errorf("aa_wd_outbound missing saga columns or indexes")
)
// temporary guard removed after the production check
