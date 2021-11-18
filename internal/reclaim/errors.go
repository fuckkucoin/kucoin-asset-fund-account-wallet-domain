package reclaim

import "fmt"

var (
	// ErrUnavailable 服务未初始化或内部状态不可用。
	ErrUnavailable = fmt.Errorf("reclaim: service unavailable")
	// ErrNotFound 案件不存在。
	ErrNotFound = fmt.Errorf("reclaim: case not found")
	// ErrIdem 幂等键冲突（已有案件，但关键参数不一致）。
	ErrIdem = fmt.Errorf("reclaim: idempotency conflict")
	// ErrDuplicate 同一 (TxHash, Vout) 已存在活跃 ReclaimCase，禁止重复开案。
	ErrDuplicate = fmt.Errorf("reclaim: duplicate active case for output")
	// ErrIllegalTransition 非法状态转移（FSM 拒绝）。
	ErrIllegalTransition = fmt.Errorf("reclaim: illegal fsm transition")
	// ErrTerminal 案件已在终态，不可再操作。
	ErrTerminal = fmt.Errorf("reclaim: case is in terminal state")
	// ErrFence 乐观锁 epoch/token 过期，拒绝写入。
	ErrFence = fmt.Errorf("reclaim: stale fence")
	// ErrActor 操作人 ID 非法。
	ErrActor = fmt.Errorf("reclaim: invalid actor")
	// ErrUser 用户 ID 非法。
	ErrUser = fmt.Errorf("reclaim: invalid user_id")
	// ErrAsset 资产代码非法。
	ErrAsset = fmt.Errorf("reclaim: invalid asset")
	// ErrAmount 金额形态或范围非法。
	ErrAmount = fmt.Errorf("reclaim: invalid amount")
	// ErrOutput TxHash 或 Vout 非法。
	ErrOutput = fmt.Errorf("reclaim: invalid tx output (hash or vout)")
	// ErrChain 链标识符非法。
	ErrChain = fmt.Errorf("reclaim: invalid chain")
	// ErrOccupancyLocked 输出已被另一个 ReclaimCase 占用，无法双重认领。
	ErrOccupancyLocked = fmt.Errorf("reclaim: output already occupied by another case")
	// ErrLateCreditBlocked 迟贷记路径冲突：输出正被 ReclaimCase 占用，贷记被阻塞。
	ErrLateCreditBlocked = fmt.Errorf("reclaim: late credit blocked by active reclaim occupancy")
	// ErrSweepNotTerminal 案件未到终态，无法创建 Sweep leg 目录。
	ErrSweepNotTerminal = fmt.Errorf("reclaim: case must reach terminal state before sweep legs")
	// ErrSweepExists 该案件已存在 Sweep leg 目录，禁止重复创建。
	ErrSweepExists = fmt.Errorf("reclaim: sweep legs already exist for this case")
	// ErrJournal 账本分录验证失败。
	ErrJournal = fmt.Errorf("reclaim: journal validation failed")
	// ErrHalt 服务处于维护暂停状态。
	ErrHalt = fmt.Errorf("reclaim: service halted for maintenance")
	// ErrSLA SLA 超时校验失败（不应在正常路径出现；仅调试用）。
	ErrSLA = fmt.Errorf("reclaim: sla deadline exceeded")
)

// wrapErr 将底层错误包装后附在域错误后，便于 errors.Is 同时保留原始信息。
   func wrapErr(sentinel, cause error) error {
	  if cause == nil {
		  return sentinel
	  }
	  return fmt.Errorf("%w: %v", sentinel, cause)
  }
