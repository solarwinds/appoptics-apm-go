package reporter

import (
	"encoding/base64"
	"encoding/json"
	"io"

	"github.com/pkg/errors"
)

// WriteType denotes the type of the buffer being written, which is either an event
// or a metric.
type WriteType string

const (
	eventWT  WriteType = "e"
	metricWT WriteType = "m"
)

// FlushWriter offers an interface to write a byte slice to a specific destination.
// The caller needs to flush the buffer explicitly in async mode, while for sync
// mode, a flush is implicitly called after a write.
type FlushWriter interface {
	io.Writer
	Flush() error
}

func newLogWriter(host string, service string, wt WriteType, syncWrite bool, dest io.Writer, maxChunkSize int) FlushWriter {
	return &logWriter{
		syncWrite:    syncWrite,
		wt:           wt,
		dest:         dest,
		maxChunkSize: maxChunkSize,
		msg: ServerlessMessage{
			Host:    host,
			Service: service,
		},
	}
}

// ServerlessMessage denotes the message to be written to AWS CloudWatch. The
// forwarder will decode the message and sent the messages to the AO collector.
type ServerlessMessage struct {
	Host    string   `json:"ao-host"`
	Service string   `json:"ao-service"`
	Data    []string `json:"ao-data"`
}

// logWriter writes the byte slices to a bytes buffer and flush the buffer when
// the trace ends. Note that it's for AWS Lambda only so there is no need to keep
// it concurrent-safe.
type logWriter struct {
	syncWrite    bool
	wt           WriteType
	dest         io.Writer
	maxChunkSize int
	chunkSize    int
	msg          ServerlessMessage
}

func (lr *logWriter) encode(bytes []byte) string {
	return string(lr.wt) + ":" + base64.StdEncoding.EncodeToString(bytes)
}

func (lr *logWriter) Write(bytes []byte) (int, error) {
	encoded := lr.encode(bytes)
	if len(encoded) > lr.maxChunkSize {
		return 0, errors.New("message too big")
	}

	if !lr.syncWrite && lr.chunkSize+len(encoded) > lr.maxChunkSize {
		lr.Flush()
		lr.chunkSize = 0
	}

	lr.msg.Data = append(lr.msg.Data, encoded)

	if lr.syncWrite {
		lr.Flush()
	} else {
		lr.chunkSize += len(encoded)
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
