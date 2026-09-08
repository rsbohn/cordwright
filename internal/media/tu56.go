package media

import (
	"github.com/rsbohn/cordwright/internal/os8"
	"github.com/rsbohn/cordwright/internal/tu56"
)

func OpenTU56(source string) (Drive, error)    { return os8.Open(source) }
func OpenTU56Raw(source string) (Drive, error) { return tu56.Open(source) }
