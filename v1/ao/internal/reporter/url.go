// Copyright (C) 2019 Librato, Inc. All rights reserved.

package reporter

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/coocood/freecache"
	"github.com/pkg/errors"
)

var globalURLFilter *urlFilter

func init() {
	globalURLFilter = newURLFilter()
	globalURLFilter.loadConfig(config.GetTransactionFiltering())
}

// Cache is a cache to store the disabled url patterns
type Cache struct{ *freecache.Cache }

// SetURLTrace sets a url and its trace decision into the cache
func (c *Cache) SetURLTrace(url string, trace bool) {
	val := []byte("t")
	if !trace {
		val = []byte("f")
	}
	_ = c.Set([]byte(url), val, 0)
}

// GetURLTrace gets the trace decision of a URL
func (c *Cache) GetURLTrace(url string) (bool, error) {
	traceStr, err := c.Get([]byte(url))
	if err != nil {
		return false, err
	}
	if string(traceStr) == "t" {
		return true, nil
	} else {
		return false, fmt.Errorf("invalid value fetched from cache: %s", traceStr)
	}

}

// Filter defines a URL filter
type Filter interface {
	Match(url string) bool
}

// RegexFilter is a regular expression based URL filter
type RegexFilter struct {
	Regex *regexp.Regexp
}

// NewRegexFilter creates a new RegexFilter instance
func NewRegexFilter(regex string) (*RegexFilter, error) {
	re, err := regexp.Compile(regex)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse regexp")
	}
	return &RegexFilter{Regex: re}, nil
}

// Match checks if the url matches the filter
func (f *RegexFilter) Match(url string) bool {
	return f.Regex.MatchString(url)
}

// ExtensionFilter is a extension-based filter
type ExtensionFilter struct {
	Exts []string
}

// NewExtensionFilter create a new instance of ExtensionFilter
func NewExtensionFilter(extensions []string) *ExtensionFilter {
	exts := append(extensions[:0:0], extensions...)
	sort.Strings(exts)
	return &ExtensionFilter{Exts: exts}
}

// Match checks if the url matches the filter
func (f *ExtensionFilter) Match(url string) bool {
	ext := filepath.Ext(url)
	return sort.SearchStrings(f.Exts, ext) != len(f.Exts)
}

type urlFilter struct {
	cache   *Cache
	filters []Filter
}

func newURLFilter() *urlFilter {
	return &urlFilter{
		cache: &Cache{freecache.NewCache(1024 * 1024)},
	}
}

func (f *urlFilter) loadConfig(filters []config.TransactionFilter) {
	for _, filter := range filters {
		if filter.Tracing == config.Enabled {
			continue
		}

		if filter.RegEx != "" {
			re, err := NewRegexFilter(filter.RegEx)
			if err != nil {
				log.Warningf("Ignoring bad regex: %s, error=", filter.RegEx, err.Error())
			}
			f.filters = append(f.filters, re)
		} else {
			f.filters = append(f.filters, NewExtensionFilter(filter.Extensions))
		}
	}
}

// ShouldTrace checks if the URL should be traced or not.
func (f *urlFilter) ShouldTrace(url string) bool {
	if len(f.filters) == 0 {
		return true
	}

	trace, err := f.cache.GetURLTrace(url)
	if err == nil {
		return trace
	}

	trace = f.shouldTrace(url)
	f.cache.SetURLTrace(url, trace)

	return trace
}

func (f *urlFilter) shouldTrace(url string) bool {
	for _, filter := range f.filters {
		if filter.Match(url) {
			return false
		}
	}
	return true
}
