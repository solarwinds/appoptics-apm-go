package reporter

import (
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	"github.com/coocood/freecache"
	"github.com/stretchr/testify/assert"
)

func TestCache(t *testing.T) {
	cache := &urlCache{freecache.NewCache(1024 * 1024)}

	cache.setURLTrace("traced_1", TRACE_ENABLED)
	cache.setURLTrace("not_traced_1", TRACE_DISABLED)
	assert.Equal(t, int64(2), cache.EntryCount())

	trace, err := cache.getURLTrace("traced_1")
	assert.Nil(t, err)
	assert.Equal(t, TRACE_ENABLED, trace)
	assert.Equal(t, int64(1), cache.HitCount())

	trace, err = cache.getURLTrace("not_traced_1")
	assert.Nil(t, err)
	assert.Equal(t, TRACE_DISABLED, trace)
	assert.Equal(t, int64(2), cache.HitCount())

	trace, err = cache.getURLTrace("non_exist_1")
	assert.NotNil(t, err)
	assert.Equal(t, TRACE_UNKNOWN, trace)
	assert.Equal(t, int64(2), cache.HitCount())
	assert.Equal(t, int64(1), cache.MissCount())
}

func TestUrlFilter(t *testing.T) {
	filter := newURLFilters()
	filter.loadConfig([]config.TransactionFilter{
		{Type: "url", RegEx: `user\d{3}`, Tracing: config.DisabledTracingMode},
		{Type: "url", Extensions: []string{".png", ".jpg"}, Tracing: config.DisabledTracingMode},
	})

	assert.Equal(t, TRACE_DISABLED, filter.getTracingMode("user123"))
	assert.Equal(t, int64(1), filter.cache.EntryCount())
	assert.Equal(t, int64(0), filter.cache.HitCount())

	assert.Equal(t, TRACE_UNKNOWN, filter.getTracingMode("test123"))
	assert.Equal(t, int64(2), filter.cache.EntryCount())
	assert.Equal(t, int64(2), filter.cache.MissCount())

	assert.Equal(t, TRACE_DISABLED, filter.getTracingMode("user200"))
	assert.Equal(t, int64(3), filter.cache.EntryCount())
	assert.Equal(t, int64(0), filter.cache.HitCount())

	assert.Equal(t, TRACE_DISABLED, filter.getTracingMode("user123"))
	assert.Equal(t, int64(3), filter.cache.EntryCount())
	assert.Equal(t, int64(1), filter.cache.HitCount())

	assert.Equal(t, TRACE_DISABLED, filter.getTracingMode("http://user.com/eric/avatar.png"))
	assert.Equal(t, int64(4), filter.cache.EntryCount())
}
