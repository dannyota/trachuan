package serve

const partitionMemoryLimit int64 = 256 * 1024 * 1024

func (e *Engine) reservePartitionMemory(bytes int64) error {
	limit := e.partitionLimit
	if limit == 0 {
		limit = partitionMemoryLimit
	}
	if bytes < 0 || bytes > limit-e.partitionBytes {
		return ErrUnavailable
	}
	e.partitionBytes += bytes
	return nil
}

func (e *Engine) reserveControlMemory(c *control) error {
	bytes := int64(512 + len(c.citation) + len(c.norm) + len(c.kind) + len(c.title) + len(c.status))
	return e.reservePartitionMemory(bytes)
}
