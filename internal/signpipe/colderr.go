package signpipe

import "fmt"

   func errText(s string) error { return fmt.Errorf("%s", s) }
