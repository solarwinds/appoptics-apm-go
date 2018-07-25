// Copyright (C) 2017 Librato, Inc. All rights reserved.

package reporter

import "time"

// BytesBucket is a struct to simulate a bucket. It has two actions: pour
// some water into it from the water source, and drain it when it's drainable.
// This struct is not intended for concurrent-safe.
type BytesBucket struct {
	// the water source, the bucket gets some water from the source when we
	// call PourIn()
	source chan []byte

	// the high watermark of the bucket, we try to keep the current watermark
	// lower than HWM, but just in best-effort.
	HWM int

	// the current watermark of the bucket, it may exceed the HWM temporarily.
	watermark int

	// the ticker to drain the bucket periodically
	ticker *time.Ticker

	// where the water is stored in
	water [][]byte

	// the last drain time
	drainTime time.Time
}

func NewBytesBucket(source chan []byte, opts ...BucketOption) *BytesBucket {
	b := &BytesBucket{source: source}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// BucketOption defines the function type of option setters
type BucketOption func(b *BytesBucket)

// WithHWM provides a high watermark for the bucket
func WithHWM(HWM int) BucketOption {
	return func(b *BytesBucket) {
		b.HWM = HWM
	}
}

// WithTicker provides a ticker to the bucket to drain it periodically.
func WithTicker(ticker *time.Ticker) BucketOption {
	return func(b *BytesBucket) {
		b.ticker = ticker
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
	// Seems we'd better to `reset` the ticker here but there is no such
	// API for a Ticker. A minor problem is that the ticker may become
	// timeout shortly after the previous drain triggered by watermark >= HWM.
	// The last drain time is stored to avoid this problem.
	b.drainTime = time.Now()
	return water
}

// Drainable checks if it can be drained now. It is true either when the
// watermark is higher than HWM, or the ticker is timeout (we want to at
// least drain it periodically)
func (b *BytesBucket) Drainable() bool {
	if b.watermark == 0 {
		return false
	}

	if b.watermark >= b.HWM {
		return true
	}

	tickerout := false
	if b.ticker != nil {
		select {
		case <-b.ticker.C:
			// Skip this chance if we've drained the bucket recently.
			if time.Now().After(b.drainTime.Add(time.Millisecond * 500)) {
				tickerout = true
			}
		default:
		}
	}

	return tickerout
}
