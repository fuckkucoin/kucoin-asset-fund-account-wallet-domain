package collide

import "errors"

var (
	errBook = errors.New("collision book unavailable")
	errMiss = errors.New("collision case not found")
)

// ErrBook 案簿不可用。

   func ErrBook() error { return errBook }

// ErrMiss 没有这宗案件。

  func ErrMiss() error { return errMiss }
