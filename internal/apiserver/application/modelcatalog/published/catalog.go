package published

import port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"

// Catalog is the application-facing read side for published assessment snapshots.
type Catalog interface {
	port.PublishedModelReader
	port.PublishedModelLister
}

type Snapshot = port.AssessmentSnapshot
