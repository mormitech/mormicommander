// Package dirsize is the public face of mormiCommander's folder size engine (internal/dirsize), for other apps such
// as mormiDiskFree (board #24: the folder size treemap shares its engine). Go does not let another module import an
// internal package, so this package re-exports the engine's API under the same names: the types are aliases, the
// functions are the same functions. The engine itself stays in internal/dirsize, documented in its README.md; the
// matching pure treemap layout for the page is frontend/src/shared/treemap-lib.js.
//
// Another module uses it with a require of module "mormicommander" and a replace directive pointing at a checkout.
package dirsize

import (
	"mormicommander/internal/dirsize"
)

// Counting folder sizes.
type (
	Options    = dirsize.Options
	Result     = dirsize.Result
	Summary    = dirsize.Summary
	Progress   = dirsize.Progress
	PathError  = dirsize.PathError
	Job        = dirsize.Job
	Cache      = dirsize.Cache
	Store      = dirsize.Store
	Remembered = dirsize.Remembered
	VolumeInfo = dirsize.VolumeInfo
	Unit       = dirsize.Unit
)

// Size units for FormatSize.
const (
	UnitBytes   = dirsize.UnitBytes
	UnitKB      = dirsize.UnitKB
	UnitMB      = dirsize.UnitMB
	UnitGB      = dirsize.UnitGB
	UnitDynamic = dirsize.UnitDynamic

	DefaultWorkers          = dirsize.DefaultWorkers
	DefaultProgressInterval = dirsize.DefaultProgressInterval
	DefaultStoreMax         = dirsize.DefaultStoreMax
)

// ErrCanceled is wrapped by the error of a cancelled calculation.
var ErrCanceled = dirsize.ErrCanceled

// The engine's functions (see internal/dirsize/README.md).
var (
	Calculate    = dirsize.Calculate
	CalculateAll = dirsize.CalculateAll
	Start        = dirsize.Start
	OpenStore    = dirsize.OpenStore
	VolumeOf     = dirsize.VolumeOf
	FormatSize   = dirsize.FormatSize
	Group        = dirsize.Group
	DriveLine    = dirsize.DriveLine
	SpaceLine    = dirsize.SpaceLine
)
