package media

import "github.com/rsbohn/cordwright/internal/tu56"

func OpenTU56(source string) (Drive, error) { return tu56.Open(source) }
