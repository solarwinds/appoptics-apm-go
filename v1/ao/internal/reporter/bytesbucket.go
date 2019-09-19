// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import (
	"time"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
)

// BytesBucket is a struct to simulate a bucket. It has two actions: pour
// some water into it from the water source, and drain it when it's drainable.
// This struct is **not** intended for concurrent-safe.
type BytesBucket struct {
	// the water source, the bucket gets some water from the source when we
	// call PourIn()
	source chan []byte

	// the high watermark of the bucket, we try to keep the current watermark
	// lower than HWM, but just in best-effort.
	HWM int

	// the current watermark of the bucket, it may exceed the HWM temporarily.
	watermark int

	// must drain the bucket no later than this time
	nextDrainTimeout time.Time

	// the function to get the new interval
	getInterval func() time.Duration

	// where the water is stored in
	water [][]byte

	// the events fetched but not poured into the bucket
	waitingList [][]byte

	// the bucket is not accepting new water
	full bool

	// drain it before shutdown
	gracefulShutdown bool

	// the bucket is closing
	closing chan struct{}

	// if the bucket is never drained
	neverDrained bool

	// the count of oversize water that gets dropped
	droppedCount int
}

// NewBytesBucket returns a new BytesBucket object with the options provided
func NewBytesBucket(source chan []byte, opts ...BucketOption) *BytesBucket {
	b := &BytesBucket{
		source:       source,
		neverDrained: true,
		getInterval: func() time.Duration {
			return time.Duration(1<<63 - 1)
		},
	}
	for _, opt := range opts {
		opt(b)
	}
	b.nextDrainTimeout = time.Now().Add(b.getInterval())

	return b
}

// BucketOption defines the function type of option setters
type BucketOption func(b *BytesBucket)

// WithHWM provides a high watermark (in bytes) for the bucket
func WithHWM(HWM int) BucketOption {
	return func(b *BytesBucket) {
		b.HWM = HWM
	}
}

// WithClosingIndicator assigns the closing indicator to the bucket
func WithClosingIndicator(closing chan struct{}) BucketOption {
	return func(b *BytesBucket) {
		b.closing = closing
	}
}

// WithGracefulShutdown sets the flag which determined if the bucket will be closed
// gracefully.
func WithGracefulShutdown(graceful bool) BucketOption {
	return func(b *BytesBucket) {
		b.gracefulShutdown = graceful
	}
}

// WithIntervalGetter provides a ticker to the bucket to drain it periodically.
func WithIntervalGetter(fn func() time.Duration) BucketOption {
	return func(b *BytesBucket) {
		b.getInterval = fn
	}
}

// PourIn pours as much water as possible from the source into the bucket and returns
// the water it pours in.
// This method blocks until it's either full or timeout.
func (b *BytesBucket) PourIn() int {
	if b.full {
		return 0
	}

	oldWM := b.watermark

	for len(b.waitingList) != 0 {
		if len(b.waitingList[0]) <= b.HWM-b.watermark {
			b.watermark += len(b.waitingList[0])
			b.water = append(b.water, b.waitingList[0])
			b.waitingList = b.waitingList[1:]
		} else {
			break
		}
	}

	drainTimeout := time.After(b.nextDrainTimeout.Sub(time.Now()))
	// drain the first drop of water ASAP
	drainASAP := b.neverDrained

outer:
	for {
	inner:
		select {
		case m := <-b.source:
			if len(m) > b.HWM {
				b.droppedCount++
				break inner
			}

			if len(m) <= b.HWM-b.watermark {
				b.watermark += len(m)
				b.water = append(b.water, m)
				if drainASAP {
					b.full = true
					break outer
				}
			} else { // let's stop when the bucket is full
				if len(b.waitingList) <= 100 {
					b.waitingList = append(b.waitingList, m)
				} else {
					log.Debug("Dropping it as waiting list is full.")
				}
				b.full = true
				break outer
			}

		case <-drainTimeout:
			if b.watermark != 0 {
				b.full = true
				break outer
			} else {
				drainASAP = true
			}

		case <-b.closing:
			if b.gracefulShutdown && b.watermark != 0 {
				b.full = true
			}
			break outer
		}
	}

	return b.watermark - oldWM
}

// Drain pour all the water out and make the bucket empty.
func (b *BytesBucket) Drain() [][]byte {
	water := b.water

	b.water = [][]byte{}
	b.watermark = 0
	b.full = false
	b.neverDrained = false
	b.nextDrainTimeout = time.Now().Add(b.getInterval())
	b.droppedCount = 0

	return water
}

// Full checks if it can be drained now. It is true either when the
// bucket is marked as full, or it reaches the maximum drain interval
// (we want to at least drain it periodically)
func (b *BytesBucket) Full() bool {
	return b.full
}

// Watermark returns the current watermark of the bucket.
func (b *BytesBucket) Watermark() int {
	return b.watermark
}

// DroppedCount returns the number of dropped water during a drain cycle
func (b *BytesBucket) DroppedCount() int {
	return b.droppedCount
}

// Count returns the water count during a drain cycle
func (b *BytesBucket) Count() int {
	return len(b.water)
}
