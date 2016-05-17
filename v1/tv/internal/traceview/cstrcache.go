// Copyright (C) 2016 AppNeta, Inc. All rights reserved.

package traceview

import (
	"C"
	"sync"
)

// Caches CStrings:
// currently used for entry layer names to avoid repetitive malloc/free of the same string.
// We intentionally do not free here.
type CStringCache struct {
	m map[string]*C.char
	sync.RWMutex
}

func NewCStringCache() *CStringCache {
	return &CStringCache{
		m: make(map[string]*C.char),
	}
}

// Has looks for the existence of a string
func (c *CStringCache) Has(str string) *C.char {
	c.RLock()
	defer c.RUnlock()
	cstr := c.m[str]
	return cstr
}

// Gets *C.char associated with a Go string
func (c *CStringCache) Get(str string) *C.char {
	cstr := c.Has(str)
	if cstr == nil {
		// Not found, need to allocate:
		c.Lock()
		defer c.Unlock()
		cstr = C.CString(str)
		c.m[str] = cstr
	}
	return cstr
}
