package addrintel

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("address intel unavailable")
	ErrAddress     = fmt.Errorf("address intel address invalid")
	ErrChain = fmt.Errorf("address intel chain invalid")
	ErrUser  = fmt.Errorf("address intel user invalid")
	ErrSanction = fmt.Errorf("address on local sanctions list")
	ErrTag      = fmt.Errorf("address cluster tag invalid")
	ErrNotFound = fmt.Errorf("address intel row not found")
	ErrList = fmt.Errorf("sanctions list invalid")
)
