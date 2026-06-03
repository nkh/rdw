package server

import (
	"github.com/nkh/rdw/internal/session"
)

// ParseTargetID is exported for use in external test packages.
var ParseTargetID = session.ParseTargetID

// PaneStateExport is session.PaneState exposed to external test packages.
type PaneStateExport = session.PaneState
