package reporter

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"os"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/metrics"
	"github.com/pkg/errors"
)

type WriteType string

const (
	eventWT WriteType = "e"
	metricWT WriteType = "m"
)

func newLogWriter(host string, service string, wt WriteType) io.Writer {
	return &logWriter{
		wt:      wt,
		host:    host,
		service: service,
		dest:    os.Stderr,
	}
}

type ServerlessMessage struct {
	Host    string   `json:"ao-host"`
	Service string   `json:"ao-service"`
	Data    []string `json:"ao-data"`
}

type logWriter struct {
	wt WriteType
	host          string
	service       string
	dest          io.Writer
	customMetrics *metrics.Measurements
}

func (lr *logWriter) encode(bytes []byte) ([]byte, error) {
	msg := ServerlessMessage{
		Host:    lr.host,
		Service: lr.service,
		Data:    []string{string(lr.wt) + ":" + base64.StdEncoding.EncodeToString(bytes)},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, errors.Wrap(err, "error encoding message")
	}
	return data, nil
}

func (lr *logWriter) Write(bytes []byte) (int, error) {
	data, err := lr.encode(bytes)
	if err != nil {
		return 0, errors.Wrap(err, "write to log reporter failed")
	}
	if n, err := lr.dest.Write(data); err != nil {
		return 0, errors.Wrap(err, "write to log reporter failed")
	} else {
		return n, nil
	}
}
