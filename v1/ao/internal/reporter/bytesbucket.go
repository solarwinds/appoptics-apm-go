// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import "time"

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

	// the maximum duration between two drains
	maxDrainInterval time.Duration

	// the function to get the new interval
	getInterval func() int64

	// where the water is stored in
	water [][]byte

	// the last drain time
	lastDrainTime time.Time
}

// NewBytesBucket returns a new BytesBucket object with the options provided
func NewBytesBucket(source chan []byte, opts ...BucketOption) *BytesBucket {
	b := &BytesBucket{source: source}
	for _, opt := range opts {
		opt(b)
	}
	// fetch the interval and create a ticker
	b.maxDrainInterval = time.Second * time.Duration(b.getInterval())

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

// WithIntervalGetter provides a ticker to the bucket to drain it periodically.
func WithIntervalGetter(fn func() int64) BucketOption {
	return func(b *BytesBucket) {
		b.getInterval = fn
	}
}

// PourIn pours as much water as possible from the source into the bucket
// It stops either when it's full or no more water from the source.
func (b *BytesBucket) PourIn() (poured int) {
	if b.watermark >= b.HWM && b.HWM != 0 {
		return
	}

	if b.HWM == 0 && b.watermark != 0 {
		return
	}

outer:
	for {
		select {
		case m := <-b.source:
			b.watermark += len(m)
			b.water = append(b.water, m)
			poured += len(m)
			// check the water after pour some water in, as we want it
			// accept some water even with HWM=0
			if b.watermark >= b.HWM {
				break outer
			}
			// stop it when it's timeout
			if !b.lastDrainTime.IsZero() &&
				time.Now().Sub(b.lastDrainTime) >= b.maxDrainInterval {
				break outer
			}
		default:
			break outer
		}
	}
	return
}

// Drain pour all the water out and make the bucket empty.
func (b *BytesBucket) Drain() [][]byte {
	water := b.water

	b.water = [][]byte{}
	b.watermark = 0
	b.lastDrainTime = time.Now()
	// refresh the interval here instead of drainable() to avoid polling
	// the global config (mutex needed) too often.
	b.maxDrainInterval = time.Second * time.Duration(b.getInterval())

	return water
}

// Drainable checks if it can be drained now. It is true either when the
// watermark is higher than HWM, or it reaches the maximum drain interval
// (we want to at least drain it periodically)
func (b *BytesBucket) Drainable() bool {
	if b.watermark == 0 {
		return false
	}

	// It's drainable when the current watermark exceeds the HWM
	// Do the first drain as soon as possible, even for one drop of water.
	if b.watermark >= b.HWM || b.lastDrainTime.IsZero() {
		return true
	}

	// It's drainable when the duration is beyond the maximum interval
	if time.Now().Sub(b.lastDrainTime) >= b.maxDrainInterval {
		return true
	}

	return false
}

// Watermark returns the current watermark of the bucket.
func (b *BytesBucket) Watermark() int {
	return b.watermark
}
