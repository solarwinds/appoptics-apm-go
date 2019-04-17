package reporter

import (
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	"github.com/coocood/freecache"
	"github.com/stretchr/testify/assert"
)

func TestCache(t *testing.T) {
	cache := &Cache{freecache.NewCache(1024 * 1024)}

	cache.SetURLTrace("traced_1", true)
	cache.SetURLTrace("not_traced_1", false)
	assert.Equal(t, int64(2), cache.EntryCount())

	trace, err := cache.GetURLTrace("traced_1")
	assert.Nil(t, err)
	assert.Equal(t, true, trace)
	assert.Equal(t, int64(1), cache.HitCount())

	trace, err = cache.GetURLTrace("not_traced_1")
	assert.Nil(t, err)
	assert.Equal(t, false, trace)
	assert.Equal(t, int64(2), cache.HitCount())

	trace, err = cache.GetURLTrace("non_exist_1")
	assert.NotNil(t, err)
	assert.Equal(t, false, trace)
	assert.Equal(t, int64(2), cache.HitCount())
	assert.Equal(t, int64(1), cache.MissCount())
}

func TestUrlFilter(t *testing.T) {
	filter := newURLFilter()
	filter.loadConfig([]config.TransactionFilter{
		{Type: "url", RegEx: `user\d{3}`, Tracing: config.DisabledTracingMode},
		{Type: "url", Extensions: []string{".png", ".jpg"}, Tracing: config.DisabledTracingMode},
	})

	assert.False(t, filter.ShouldTrace("user123"))
	assert.Equal(t, int64(1), filter.cache.EntryCount())
	assert.Equal(t, int64(0), filter.cache.HitCount())

	assert.True(t, filter.ShouldTrace("test123"))
	assert.Equal(t, int64(2), filter.cache.EntryCount())
	assert.Equal(t, int64(2), filter.cache.MissCount())

	assert.False(t, filter.ShouldTrace("user200"))
	assert.Equal(t, int64(3), filter.cache.EntryCount())
	assert.Equal(t, int64(0), filter.cache.HitCount())

	assert.False(t, filter.ShouldTrace("user123"))
	assert.Equal(t, int64(3), filter.cache.EntryCount())
	assert.Equal(t, int64(1), filter.cache.HitCount())

	assert.False(t, filter.ShouldTrace("http://user.com/eric/avatar.png"))
	assert.Equal(t, int64(4), filter.cache.EntryCount())
}
