package reporter

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"os"

	"github.com/pkg/errors"
)

type WriteType string

const (
	eventWT WriteType = "e"
	metricWT WriteType = "m"
)

type FlushWriter interface {
	io.Writer
	Flush() error
}

func newLogWriter(host string, service string, wt WriteType, syncWrite bool) FlushWriter {
	return &logWriter{
		syncWrite: syncWrite,
		wt:      wt,
		dest:    os.Stderr,
		msg: ServerlessMessage{
			Host: host,
			Service: service,
		},
	}
}

type ServerlessMessage struct {
	Host    string   `json:"ao-host"`
	Service string   `json:"ao-service"`
	Data    []string `json:"ao-data"`
}

// logWriter writes the byte slices to a bytes buffer and flush the buffer when
// the trace ends. Note that it's for AWS Lambda only so there is no need to keep
// it concurrent-safe.
type logWriter struct {
	syncWrite bool
	wt        WriteType
	dest      io.Writer
	msg       ServerlessMessage
}

func (lr *logWriter) encode(bytes []byte) {
	lr.msg.Data = append(lr.msg.Data, string(lr.wt) + ":" + base64.StdEncoding.EncodeToString(bytes))
}

func (lr *logWriter) Write(bytes []byte) (int, error) {
	lr.encode(bytes)
	if lr.syncWrite {
		lr.Flush()
	}
	return len(bytes), nil
}

func (lr *logWriter) Flush() error {
	if len(lr.msg.Data) == 0 {
		return errors.New("nothing to flush")
	}

	data, err := json.Marshal(lr.msg)
	if err != nil {
		return errors.Wrap(err, "error marshaling message")
	}
	lr.msg.Data = []string{}

	data = append(data, "\n"...)

	if _, err := lr.dest.Write(data); err != nil {
		return errors.Wrap(err, "write to log reporter failed")
	}
	return nil
}
