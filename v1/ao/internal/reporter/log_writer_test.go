package reporter

import (
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
	"github.com/stretchr/testify/assert"
)

func TestLogWriter(t *testing.T) {
	sb := &utils.SafeBuffer{}
	eventWriter := newLogWriter("myHost", "myService", eventWT, false, sb, 1e6)
	eventWriter.SetRequestID("my-request-id")
	eventWriter.Write([]byte("hello event"))
	assert.Equal(t, 0, sb.Len())
	eventWriter.Flush()
	assert.Equal(t, "{\"ao-host\":\"myHost\",\"ao-service\":\"myService\",\"ao-data\":[\"e:aGVsbG8gZXZlbnQ=\"],\"request-id\":\"my-request-id\"}\n", sb.String())

	sb.Reset()
	metricWriter := newLogWriter("myHost", "myService", metricWT, true, sb, 1e6)
	metricWriter.SetRequestID("my-request-id")
	metricWriter.Write([]byte("hello metric"))
	assert.Equal(t, "{\"ao-host\":\"myHost\",\"ao-service\":\"myService\",\"ao-data\":[\"m:aGVsbG8gbWV0cmlj\"],\"request-id\":\"my-request-id\"}\n", sb.String())
	assert.NotNil(t, metricWriter.Flush())

	sb.Reset()
	writer := newLogWriter("myHost", "myService", eventWT, false, sb, 15)
	writer.SetRequestID("my-request-id")
	n, err := writer.Write([]byte("hello event"))
	assert.Zero(t, n)
	assert.Error(t, err)

	writer.Write([]byte("hello"))
	assert.Zero(t, sb.Len())
	writer.Write([]byte(" event"))
	assert.Equal(t, 100, sb.Len())
	assert.Equal(t, "{\"ao-host\":\"myHost\",\"ao-service\":\"myService\",\"ao-data\":[\"e:aGVsbG8=\"],\"request-id\":\"my-request-id\"}\n", sb.String())
	writer.Flush()
	assert.Equal(t, "{\"ao-host\":\"myHost\",\"ao-service\":\"myService\",\"ao-data\":[\"e:aGVsbG8=\"],\"request-id\":\"my-request-id\"}\n{\"ao-host\":\"myHost\",\"ao-service\":\"myService\",\"ao-data\":[\"e:IGV2ZW50\"],\"request-id\":\"my-request-id\"}\n",
		sb.String())

}
