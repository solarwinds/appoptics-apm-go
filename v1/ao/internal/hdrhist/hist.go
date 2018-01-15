package hdrhist

import (
	"math"
	"time"
)

// Config contains the options that can be used to
// configure a Hist.
type Config struct {
	_ struct{}

	// LowestDiscernible is the lowest value that
	// can be discerned from 0. Must be positive.
	// This number may be rounded down to the
	// nearest power of 2.
	LowestDiscernible int64

	// HighestTrackable is the largest value that can
	// be tracked by the histogram. This must be at
	// least twice the value of LowestDiscernible.
	HighestTrackable int64

	// SigFigs are the number of significant figures
	// that will be maintained by the histogram. Must
	// be ∈ [0,5].
	SigFigs int32

	// AutoResize will adjust HighestTrackable and
	// resize the histogram if necessary. Note that
	// resizing the histogram requires allocation
	// and will take longer than a typical operation.
	AutoResize bool
}

type Hist struct {
	b          buckets
	cfg        Config
	totalCount int64

	startTime *time.Time
	endTime   *time.Time
}

type buckets struct {
	counts          []int64
	subHalfCount    int32
	subHalfCountMag int32
	subMask         int64 // max value in bucket 0
	subCount        int32
	bucketCount     int32

	// max k where 2^k ≤ LowestDiscernible
	unitMag int32

	// number of leading 0s in max value that fits
	// in bucket 0
	leadZeroCountBase int32
}

// Clone returns a deep copy of the histogram. This
// is useful when combining or taking differences of
// histograms.
func (h *Hist) Clone() *Hist {
	var h2 Hist
	h2 = *h
	c := make([]int64, len(h2.b.counts))
	copy(c, h2.b.counts)
	h2.b.counts = c
	return &h2
}

type HistVal struct {
	Value      int64
	Count      int64
	CumCount   int64
	Percentile float64
}

// New creates a new Hist that auto-resizes and has
// a LowestDiscernible value of 1. Valid values for
// sigfigs are between 0 and 5.
func New(sigfigs int32) *Hist {
	return WithConfig(Config{
		LowestDiscernible: 1,
		HighestTrackable:  2,
		SigFigs:           sigfigs,
		AutoResize:        true,
	})
}

// WithConfig creates a new Hist with the provided
// Config.
func WithConfig(cfg Config) *Hist {
	var h Hist
	h.Init(cfg)
	return &h
}

// Init initializes the Hist with the given Config.
func (h *Hist) Init(cfg Config) {
	if cfg.LowestDiscernible < 1 {
		panic("invalid cfg: LowestDiscernible must be >= 1")
	}
	if cfg.HighestTrackable < 2*cfg.LowestDiscernible {
		if cfg.AutoResize {
			cfg.HighestTrackable = 2 * cfg.LowestDiscernible
		} else {
			panic("invalid cfg: HighestTrackable must be >= 2*LowestDiscernible")
		}
	}
	if cfg.SigFigs < 0 || cfg.SigFigs > 5 {
		panic("invalid cfg: must have SigFigs ∈ [0,5]")
	}
	h.cfg = cfg

	unitMag := int32(math.Floor(math.Log2(float64(cfg.LowestDiscernible))))
	largestSingleUnitResolutionValue := 2 * math.Pow10(int(cfg.SigFigs))
	subCountMag := int32(math.Ceil(math.Log2(largestSingleUnitResolutionValue)))
	subHalfCountMag := int32(0)
	if subCountMag > 1 {
		subHalfCountMag = subCountMag - 1
	}
	subCount := int32(math.Pow(2, float64(subHalfCountMag+1)))
	subHalfCount := subCount / 2
	subMask := (int64(subCount) - 1) << uint64(unitMag)
	bucketCount := numBucketsToCoverVal(cfg.HighestTrackable, subCount, unitMag)
	countsLen := (bucketCount + 1) * subHalfCount

	h.b.counts = make([]int64, countsLen)
	h.b.subHalfCount = subHalfCount
	h.b.subHalfCountMag = subHalfCountMag
	h.b.subMask = subMask
	h.b.subCount = subCount
	h.b.bucketCount = bucketCount
	h.b.unitMag = unitMag
	h.b.leadZeroCountBase = 64 - unitMag - subHalfCountMag - 1
}

func (h *Hist) resize(highest int64) {
	bucketCount := numBucketsToCoverVal(highest, h.b.subCount, h.b.unitMag)
	countsLen := int(bucketCount+1) * int(h.b.subCount/2)
	counts := make([]int64, countsLen)
	copy(counts, h.b.counts)

	h.b.counts = counts
	h.b.bucketCount = bucketCount
	h.cfg.HighestTrackable = h.b.highestEquiv(h.b.valueFor(countsLen - 1))
}

func numBucketsToCoverVal(v int64, subCount, unitMag int32) int32 {
	smallestUntrackable := int64(subCount) << uint64(unitMag)

	req := int32(1)
	for smallestUntrackable < v {
		if smallestUntrackable > math.MaxInt64/2 {
			return req + 1
		}
		smallestUntrackable <<= 1
		req++
	}
	return req
}

func (b *buckets) countsIndex(v int64) int {
	bi := bucketIndex(v, b.subMask, b.leadZeroCountBase)
	sbi := subBucketIndex(v, bi, b.unitMag)
	base := (bi + 1) << uint(b.subHalfCountMag)
	offset := sbi - int(b.subHalfCount)
	return base + offset
}

func bucketIndex(v, subMask int64, leadZeroCountBase int32) int {
	return int(leadZeroCountBase) - leadingZeros(uint64(v|subMask))
}

func subBucketIndex(v int64, bi int, unitMag int32) int {
	return int(uint64(v) >> uint(bi+int(unitMag)))
}

func (b *buckets) valueFor(i int) int64 {
	bi := (i >> uint(b.subHalfCountMag)) - 1
	sbi := (i & int(b.subHalfCount-1)) + int(b.subHalfCount)
	if bi < 0 {
		sbi -= int(b.subHalfCount)
		bi = 0
	}
	return int64(sbi) << uint64(bi+int(b.unitMag))
}

func (b *buckets) sizeOfEquivalentValueRange(v int64) int64 {
	bi := bucketIndex(v, b.subMask, b.leadZeroCountBase)
	sbi := subBucketIndex(v, bi, b.unitMag)
	t := bi
	if sbi >= int(b.subCount) {
		bi++
	}
	nextDist := int64(1) << uint64(int64(b.unitMag)+int64(t))
	return nextDist
}

func (b *buckets) lowestEquiv(v int64) int64 {
	bi := bucketIndex(v, b.subMask, b.leadZeroCountBase)
	sbi := subBucketIndex(v, bi, b.unitMag)
	return int64(sbi) << uint64(bi+int(b.unitMag))
}

func (b *buckets) medianEquiv(v int64) int64 {
	return b.lowestEquiv(v) + (b.sizeOfEquivalentValueRange(v) / 2)
}

func (b *buckets) highestEquiv(v int64) int64 {
	return b.lowestEquiv(v) + b.sizeOfEquivalentValueRange(v) - 1
}

func (b *buckets) areEquiv(v1, v2 int64) bool {
	return b.lowestEquiv(v1) == b.lowestEquiv(v2)
}

func leadingZeros(x uint64) int {
	n := 64
	if y := x >> 32; y != 0 {
		n -= 32
		x = y
	}
	if y := x >> 16; y != 0 {
		n -= 16
		x = y
	}
	if y := x >> 8; y != 0 {
		n -= 8
		x = y
	}
	if y := x >> 4; y != 0 {
		n -= 4
		x = y
	}
	if y := x >> 2; y != 0 {
		n -= 2
		x = y
	}
	if y := x >> 1; y != 0 {
		n -= 1
		x = y
	}
	return n - int(x)
}

func (h *Hist) Add(o *Hist) {
	highestRecordable := h.b.highestEquiv(h.b.valueFor(len(h.b.counts) - 1))
	if oMax := o.Max(); highestRecordable < o.Max() {
		if !h.cfg.AutoResize {
			panic("other histogram has values that are too large")
		}
		h.resize(oMax)
	}
	if h.b.bucketCount == o.b.bucketCount &&
		h.b.subCount == o.b.subCount &&
		h.b.unitMag == o.b.unitMag {

		// fast path, underlying arrays match, just iterate and copy
		var ototal int64
		for i, count := range o.b.counts {
			h.b.counts[i] += count
			ototal += count
		}
		h.totalCount += ototal
	} else {
		// slow path
		for i, count := range o.b.counts {
			if count > 0 {
				h.RecordN(o.b.valueFor(i), count)
			}
		}
	}

	if h.startTime == nil {
		h.startTime = o.startTime
	} else if o.startTime != nil && o.startTime.Before(*h.startTime) {
		h.startTime = o.startTime
	}
	if h.endTime == nil {
		h.endTime = o.endTime
	} else if o.endTime != nil && h.endTime.Before(*o.endTime) {
		h.endTime = o.endTime
	}
}

func (h *Hist) Sub(o *Hist) {
	highestRecordable := h.b.highestEquiv(h.b.valueFor(len(h.b.counts) - 1))
	if oMax := o.Max(); highestRecordable < oMax {
		if !h.cfg.AutoResize {
			panic("other histogram has values that are too large")
		}
		h.resize(oMax)
	}
	for i, count := range o.b.counts {
		if count > 0 {
			v := o.b.valueFor(i)
			if h.Val(v).Count < count {
				panic("other histogram has higher count than this")
			}
			h.RecordN(v, -count)
		}
	}
}

func (h *Hist) AllVals() []HistVal {
	var vals []HistVal
	var total int64
	for i, count := range h.b.counts {
		if total >= h.totalCount {
			break
		}
		if count == 0 {
			continue
		}
		total += count
		v := h.b.highestEquiv(h.b.valueFor(i))
		vals = append(vals, HistVal{
			Value:      v,
			Count:      count,
			CumCount:   total,
			Percentile: 100 * float64(total) / float64(h.totalCount),
		})
	}
	return vals
}

func (h *Hist) Val(v int64) HistVal {
	i := h.b.countsIndex(v)
	if v < 0 || i < 0 {
		return HistVal{Value: v}
	}
	if i >= len(h.b.counts) {
		return HistVal{
			Value:      v,
			CumCount:   h.TotalCount(),
			Percentile: 100,
		}
	}
	var count int64
	cs := h.b.counts[:i+1]
	for _, c := range cs {
		count += c
	}
	percentile := 100 * float64(count) / float64(h.totalCount)
	if h.totalCount == 0 {
		percentile = 100
	}
	return HistVal{
		Value:      h.b.highestEquiv(v),
		Count:      h.b.counts[i],
		CumCount:   count,
		Percentile: percentile,
	}
}

// EstMemSize estimates the number of bytes being consumed by
// the histogram. It ignores any memory usage caused by the
// start time and end time. The resulting size should not be
// assumed to be exact. The return value is in bytes.
func (h *Hist) EstMemSize() int {
	return histSize + cap(h.b.counts)*8
}

func (h *Hist) Max() int64 { return h.PercentileVal(100).Value }
func (h *Hist) Min() int64 { return h.PercentileVal(0).Value }

func (h *Hist) Mean() float64 {
	var total int64
	for i, count := range h.b.counts {
		v := h.b.medianEquiv(h.b.valueFor(i))
		total += v * count
	}
	return float64(total) / math.Max(float64(h.totalCount), 1)
}

func (h *Hist) Stdev() float64 {
	var sum float64
	μ := h.Mean()
	for i, count := range h.b.counts {
		v := h.b.medianEquiv(h.b.valueFor(i))
		dev := μ - float64(v)
		sum += dev * dev * float64(count)
	}
	return math.Sqrt(sum / math.Max(float64(h.totalCount), 1))
}

func (h *Hist) TotalCount() int64 { return h.totalCount }

func (h *Hist) PercentileVal(p float64) HistVal {
	p = math.Min(p, 100)
	desiredCount := int64((p/100)*float64(h.totalCount) + 0.5)
	if desiredCount < 1 {
		desiredCount = 1
	}
	var total int64
	for i, count := range h.b.counts {
		total += count
		if total >= desiredCount {
			v := h.b.valueFor(i)
			if p == 0 {
				v = h.b.lowestEquiv(v)
			} else {
				v = h.b.highestEquiv(v)
			}
			percentile := (100 * float64(total)) / float64(h.totalCount)
			if h.totalCount == 0 {
				percentile = 100
			}
			return HistVal{
				Value:      v,
				Count:      count,
				CumCount:   total,
				Percentile: percentile,
			}
		}
	}
	return HistVal{
		Value:      0,
		Count:      0,
		CumCount:   0,
		Percentile: 0,
	}
}

func (h *Hist) StartTime() (time.Time, bool) {
	if h.startTime != nil {
		return *h.startTime, true
	}
	return time.Time{}, false
}

func (h *Hist) EndTime() (time.Time, bool) {
	if h.endTime != nil {
		return *h.endTime, true
	}
	return time.Time{}, false
}

func (h *Hist) SetStartTime(t time.Time) { h.startTime = &t }
func (h *Hist) SetEndTime(t time.Time)   { h.endTime = &t }
func (h *Hist) SetAutoResize(b bool)     { h.cfg.AutoResize = b }
func (h *Hist) Config() Config           { return h.cfg }

func (h *Hist) Record(v int64) { h.RecordN(v, 1) }

func (h *Hist) RecordN(v, count int64) {
	i := h.b.countsIndex(v)
	if i > len(h.b.counts) && h.cfg.AutoResize {
		h.resize(v)
	}
	if 0 > i || i > len(h.b.counts) {
		panic("value to large")
	}
	h.b.counts[i] += count
	h.totalCount += count
}

func (h *Hist) RecordCorrected(v int64, expectedInterval int64) {
	h.RecordN(v, 1)
	missing := v - expectedInterval
	for missing >= expectedInterval {
		h.RecordN(missing, 1)
		missing -= expectedInterval
	}
}

// Clear deletes all recorded values as well as the start and end times.
func (h *Hist) Clear() {
	for i := range h.b.counts {
		h.b.counts[i] = 0
	}
	h.totalCount = 0
	h.startTime = nil
	h.endTime = nil
}

func (h *Hist) GetConfig() Config {
	return h.cfg
}

type Recorder struct {
	h Hist
}

func NewRecorder(sigfigs int32) *Recorder {
	return NewRecorderWithConfig(Config{
		LowestDiscernible: 1,
		HighestTrackable:  2,
		SigFigs:           sigfigs,
		AutoResize:        true,
	})
}

func NewRecorderWithConfig(cfg Config) *Recorder {
	var r Recorder
	r.Init(cfg)
	return &r
}

func (r *Recorder) Init(cfg Config) {
	r.h.Init(cfg)
	r.h.SetStartTime(time.Now())
}

func (r *Recorder) Clear()                 { r.h.Clear() }
func (r *Recorder) Record(v int64)         { r.h.Record(v) }
func (r *Recorder) RecordN(v, count int64) { r.h.RecordN(v, count) }
func (r *Recorder) RecordCorrected(v int64, expectedInterval int64) {
	r.h.RecordCorrected(v, expectedInterval)
}

func (r *Recorder) IntervalHist(h *Hist) *Hist {
	if h == nil {
		h = &Hist{}
	}
	newCounts := h.b.counts
	*h = r.h
	now := time.Now()
	h.SetEndTime(now)
	if cap(newCounts) < len(r.h.b.counts) {
		newCounts = make([]int64, len(r.h.b.counts))
	}
	r.h.b.counts = newCounts[0:len(r.h.b.counts)]
	r.h.Clear()
	r.h.SetStartTime(now)
	return h
}
