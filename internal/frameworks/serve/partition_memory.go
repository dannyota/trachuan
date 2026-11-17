package serve

const partitionMemoryLimit int64 = 256 * 1024 * 1024

func (e *Engine) reservePartitionMemory(bytes int64) error {
	if bytes < 0 || bytes > partitionMemoryLimit-e.partitionBytes {
		return ErrUnavailable
	}
	e.partitionBytes += bytes
	return nil
}

func (e *Engine) reserveControlMemory(c *control) error {
	bytes := int64(512 + len(c.citation) + len(c.norm) + len(c.kind) + len(c.title) + len(c.status))
	return e.reservePartitionMemory(bytes)
}
