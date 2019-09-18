package reporter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBytesBucket(t *testing.T) {
	// the water source with 7 drops of water
	source := make(chan []byte, 7)
	for i := 0; i < 7; i++ {
		select {
		case source <- []byte{byte(i)}:
		default:
		}
	}

	// a new bucket with high watermark=5 and a ticker of 20 milliseconds
	b := NewBytesBucket(source,
		WithHWM(5),
		WithIntervalGetter(func() time.Duration { return time.Millisecond * 20 }))

	// try pour in some water and check the returned value. It should be full with
	// just one drop of water for the first draining.
	poured := b.PourIn()
	assert.Equal(t, 1, poured)
	assert.True(t, b.Full())

	// it cannot be poured in as it's full
	poured = b.PourIn()
	assert.Equal(t, 0, poured)

	// drain the bucket
	b.Drain()

	// 5 drops of water can be poured in
	poured = b.PourIn()
	assert.Equal(t, 5, poured)
	assert.True(t, b.Full())
	b.Drain()

	// there should be only 1 drop of water being poured in.
	poured = b.PourIn()
	assert.Equal(t, 1, poured)
	assert.Equal(t, true, b.Full())

	// no more water can be poured into the bucket
	poured = b.PourIn()
	assert.Equal(t, 0, poured)

	b.Drain()

	// get the correct length
	source <- []byte{1}
	source <- []byte{2, 3}
	poured = b.PourIn()
	assert.Equal(t, 3, poured)
}

func TestBytesBucket_OversizeCount(t *testing.T) {
	source := make(chan []byte, 7)
	source <- []byte{1}
	source <- []byte{2, 3}
	source <- []byte{2, 3, 4, 5}
	source <- []byte{6, 7}

	closer := make(chan struct{})

	b := NewBytesBucket(source,
		WithHWM(3),
		WithIntervalGetter(func() time.Duration { return time.Millisecond * 20 }),
		WithClosingIndicator(closer))

	poured := b.PourIn()
	assert.Equal(t, 1, poured)
	assert.True(t, b.Full())
	b.Drain()

	poured = b.PourIn()
	assert.Equal(t, 2, poured)
	assert.True(t, b.Full())
	assert.Equal(t, 1, b.OversizeCount())
	b.Drain()

	poured = b.PourIn()
	assert.Equal(t, 2, poured)
	assert.Equal(t, 0, b.OversizeCount())
	assert.True(t, b.Full())

	close(closer)
	poured = b.PourIn()
	assert.Equal(t, 0, poured)
	assert.True(t, b.Full())
}
