package media

import "github.com/rsbohn/cordwright/internal/hawk"

func OpenHawk(source string, stride int) (Drive, error) { return hawk.Open(source, stride) }

// Optional capabilities keep image operations out of host-folder drivers.
type SectorReader interface{ Sector(n int64) ([]byte, error) }
type AllocationReader interface {
	Sectors(name string) ([]int64, error)
}
type TextReader interface {
	Text(name string) (string, error)
}
