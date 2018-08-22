package reporter

import (
	"os"
	"testing"
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
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

	os.Setenv("APPOPTICS_EVENTS_FLUSH_INTERVAL", "1")
	config.Refresh()

	opts := config.ReporterOpts()

	// a new bucket with high watermark=5 and a ticker of 2 seconds
	b := NewBytesBucket(source,
		WithHWM(5),
		WithIntervalGetter(opts.GetEventFlushInterval))

	// try pour in some water and check the returned value
	poured := b.PourIn()
	assert.Equal(t, 5, poured)

	// try pour in for another 10 times, there should be only 4 drops
	// of water being poured in.
	poured = 0
	for i := 0; i < 3; i++ {
		poured += b.PourIn()
	}
	// no more water can be poured into the bucket
	assert.Equal(t, 0, poured)

	// and it should be drainable now as it's full
	assert.Equal(t, true, b.Drainable())

	// drain the water and check the result
	water := b.Drain()
	for i, w := range water {
		assert.True(t, int(w[0]) == i)
	}

	// pour some water in the bucket
	poured = b.PourIn()

	// 2 drops of water are poured into the bucket
	assert.Equal(t, 2, poured)
	// should not be drainable right now
	assert.Equal(t, false, b.Drainable())
	// sleep for a while to make the ticker timeout
	time.Sleep(time.Millisecond * 1100)
	// it should be drainable now
	assert.Equal(t, true, b.Drainable())
	// and 2 drops of water are drained from the bucket
	assert.Equal(t, 2, len(b.Drain()))

	// get the correct length
	source <- []byte{1}
	source <- []byte{2, 3}
	poured = b.PourIn()
	assert.Equal(t, 3, poured)

	b.getInterval = func() int64 { return 1 }
	// Drain it to trigger the refreshing of flush interval
	b.Drain()

	source <- []byte{1}
	poured = b.PourIn()
	assert.Equal(t, 1, poured)
	assert.Equal(t, false, b.Drainable())
	time.Sleep(time.Millisecond * 1500)
	assert.Equal(t, true, b.Drainable())

}
