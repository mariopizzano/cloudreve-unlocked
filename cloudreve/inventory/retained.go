package inventory

const (
	// RetainedObjectStatePending marks an object waiting for its retention
	// window to expire before physical deletion.
	RetainedObjectStatePending = "pending"
	// RetainedObjectStateSkipped marks an object that will not be cleaned up
	// because it is still referenced by a live entity. It is kept for audit.
	RetainedObjectStateSkipped = "skipped"
)
