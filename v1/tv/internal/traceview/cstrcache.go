// +build traceview

// Copyright (C) 2016 Librato, Inc. All rights reserved.

package traceview

import (
	"C"
	"sync"
)

// Currently used for entry layer names to avoid repetitive malloc/free of the same string,
// and to count per-layer metrics. We intentionally do not free here.
type cStringCache struct {
	m map[string]*cachedLayer
	sync.RWMutex
}
type cachedLayer struct {
	name    *C.char
	counter *rateCounter
}

func newCStringCache() *cStringCache {
	return &cStringCache{
		m: make(map[string]*cachedLayer),
	}
}

// Has looks for the existence of a string
func (c *cStringCache) Has(str string) *cachedLayer {
	c.RLock()
	defer c.RUnlock()
	return c.m[str]
}

// Keys returns a list of this cache's keys
func (c *cStringCache) Keys() (keys []string) {
	c.RLock()
	defer c.RUnlock()
	for k := range c.m {
		keys = append(keys, k)
	}
	return
}

// Gets *C.char associated with a Go string
func (c *cStringCache) Get(str string) *cachedLayer {
	cl := c.Has(str)
	if cl == nil {
		// Not found, need to allocate:
		c.Lock()
		defer c.Unlock()
		c.m[str] = &cachedLayer{
			name:    C.CString(str),
			counter: newRateCounter(rateCounterDefaultRate, rateCounterDefaultSize),
		}
		cl = c.m[str]
	}
	return cl
}
