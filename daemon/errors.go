package daemon

import "fmt"

type FatalError string

func (n FatalError) Error() string {
	return fmt.Sprintf("fatal: %s", string(n))
}
