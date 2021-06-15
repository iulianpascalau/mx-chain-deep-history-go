package chunk

import (
	logger "github.com/ElrondNetwork/elrond-go-logger"
)

var log = logger.GetOrCreate("process/interceptors/processor")

type chunk struct {
	maxChunks uint32
	data      map[uint32][]byte
	size      int
}

// NewChunk creates a new chunk instance able to account for the existing and missing chunks of a larger buffer
// Not a concurrent safe component
func NewChunk(maxChunks uint32) *chunk {
	return &chunk{
		data:      make(map[uint32][]byte),
		maxChunks: maxChunks,
	}
}

// Put will add or rewrite an existing chunk
func (c *chunk) Put(chunkIndex uint32, buff []byte) {
	if chunkIndex >= c.maxChunks {
		return
	}

	existing, _ := c.data[chunkIndex]
	c.data[chunkIndex] = buff
	c.size = c.size - len(existing) + len(buff)
}

// TryAssembleAllChunks will try to assemble the original payload by iterating all available chunks
// It returns nil if at least one chunk is missing
func (c *chunk) TryAssembleAllChunks() []byte {
	gotAllParts := c.maxChunks > 0 && len(c.data) == int(c.maxChunks)
	if !gotAllParts {
		//TODO(iulian) remove this log
		log.Warn("not all parts got", "max chunks", c.maxChunks, "len chunk", len(c.data))
		return nil
	}

	buff := make([]byte, 0, c.size)
	for i := uint32(0); i < c.maxChunks; i++ {
		part := c.data[i]
		buff = append(buff, part...)
	}

	return buff
}

// GetAllMissingChunkIndexes returns all missing chunk indexes
func (c *chunk) GetAllMissingChunkIndexes() []uint32 {
	missing := make([]uint32, 0)
	for i := uint32(0); i < c.maxChunks; i++ {
		_, partFound := c.data[i]
		if !partFound {
			missing = append(missing, i)
		}
	}

	return missing
}

// Size returns the size in bytes stored in the values of the inner map
func (c *chunk) Size() int {
	return c.size
}

// IsInterfaceNil returns true if there is no value under the interface
func (c *chunk) IsInterfaceNil() bool {
	return c == nil
}
