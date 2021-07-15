package holdcat

import "errors"

var (
	// 已在 hold.go 定义：ErrUser / ErrReason / ErrAmount / ErrStore / ErrGone / ErrOwner
	// 分层冻结新增
	ErrLayerInvalid = errors.New("holdcat: layer invalid or unknown")
	ErrReasonMismatch    = errors.New("holdcat: reason code does not belong to the specified layer")
	ErrIdemKeyRequired   = errors.New("holdcat: idem_key is required for hold token")
	ErrLayerDenied       = errors.New("holdcat: caller layer has insufficient priority to release target layer")
	ErrTokenNotActive    = errors.New("holdcat: token is not in active state")
	ErrTokenNotReleasing = errors.New("holdcat: token is not in releasing state")
	ErrHigherLayerBlock = errors.New("holdcat: higher-priority layer freeze exists; release denied")
)
