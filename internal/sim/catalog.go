package sim

import "context"

type Catalog interface {
	Packages(context.Context, string) ([]Package, error)
	Package(context.Context, string) (Package, error)
}
