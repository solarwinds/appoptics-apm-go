package reporter

import (
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/utils"
	"github.com/stretchr/testify/assert"
)

func TestLogWriter(t *testing.T) {
	sb := &utils.SafeBuffer{}
	eventWriter := newLogWriter(false, sb, 1e6)
	eventWriter.Write(EventWT, []byte("hello event"))
	assert.Equal(t, 0, sb.Len())
	eventWriter.Flush()
	assert.Equal(t, "{\"ao-data\":{\"events\":[\"aGVsbG8gZXZlbnQ=\"]}}\n", sb.String())

	sb.Reset()
	metricWriter := newLogWriter(true, sb, 1e6)
	metricWriter.Write(MetricWT, []byte("hello metric"))
	assert.Equal(t, "{\"ao-data\":{\"metrics\":[\"aGVsbG8gbWV0cmlj\"]}}\n", sb.String())
	assert.NotNil(t, metricWriter.Flush())

	sb.Reset()
	writer := newLogWriter(false, sb, 15)
	n, err := writer.Write(EventWT, []byte("hello event"))
	assert.Zero(t, n)
	assert.Error(t, err)

	writer.Write(EventWT, []byte("hello"))
	assert.Zero(t, sb.Len())
	writer.Write(EventWT, []byte(" event"))
	assert.Equal(t, 36, sb.Len())
	assert.Equal(t, "{\"ao-data\":{\"events\":[\"aGVsbG8=\"]}}\n", sb.String())
	writer.Flush()
	assert.Equal(t, "{\"ao-data\":{\"events\":[\"aGVsbG8=\"]}}\n{\"ao-data\":{\"events\":[\"IGV2ZW50\"]}}\n",
		sb.String())

}
