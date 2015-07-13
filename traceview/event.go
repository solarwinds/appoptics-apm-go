package traceview

type Event struct {
	metadata oboe_metadata_t
	bbuf     bson_buffer
}

// Every event needs a label:
type Label string

const (
	LabelEntry = "entry"
	LabelExit  = "exit"
	LabelInfo  = "info"
)

const (
	EventHeader = "1"
)

func oboe_event_init(evt *Event, md *oboe_metadata_t) int {
	var result int

	// Metadata initialization
	result = oboe_metadata_init(&evt.metadata)
	if result < 0 {
		return result
	}

	evt.metadata.task_len = md.task_len
	evt.metadata.op_len = md.op_len

	copy(evt.metadata.ids.task_id, md.ids.task_id)
	oboe_random_op_id(&evt.metadata)

	// Buffer initialization

	bson_buffer_init(&evt.bbuf)

	// Copy header to buffer
	// TODO errors?
	bson_append_string(&evt.bbuf, "_V", EventHeader)

	// Pack metadata
	md_str, err := oboe_metadata_tostr(&evt.metadata)
	if err == nil {
		bson_append_string(&evt.bbuf, "X-Trace", md_str)
	}

	return 0
}

func NewEvent(md *oboe_metadata_t, label Label, layer string) *Event {
	e := &Event{}
	oboe_event_init(e, md)
	e.addLabelLayer(label, layer)
	return e
}

func (e *Event) addLabelLayer(label Label, layer string) {
	e.AddString("Label", string(label))
	if layer != "" {
		e.AddString("Layer", layer)
	}
}

// Adds string key/value to event
func (e *Event) AddString(key, value string) {
	bson_append_string(&e.bbuf, key, value)
}

// Adds int key/value to event
func (e *Event) AddInt(key string, value int) {
	bson_append_int(&e.bbuf, key, value)
}

// Adds int64 key/value to event
func (e *Event) AddInt64(key string, value int64) {
	bson_append_int64(&e.bbuf, key, value)
}

// Adds int32 key/value to event
func (e *Event) AddInt32(key string, value int32) {
	bson_append_int32(&e.bbuf, key, value)
}

// Adds edge (reference to previous event) to event
func (e *Event) AddEdge(ctx *Context) {
	bson_append_string(&e.bbuf, "Edge", metadataString(&ctx.metadata))
}

func (e *Event) AddEdgeFromMetaDataString(mdstr string) {
	var md oboe_metadata_t
	oboe_metadata_init(&md)
	oboe_metadata_fromstr(&md, mdstr)
	bson_append_string(&e.bbuf, "Edge", metadataString(&md))
}

// Reports event using specified Reporter
func (e *Event) ReportUsing(c *Context, r *Reporter) error {
	return r.ReportEvent(c, e)
}

// Reports event using default (UDP) Reporter
func (e *Event) Report(c *Context) error {
	return e.ReportUsing(c, udp_reporter)
}

// Returns Metadata string (X-Trace header)
func (e *Event) MetaDataString() string {
	return metadataString(&e.metadata)
}
