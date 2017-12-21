// +build !linux

package traceview

func appendUname(bbuf *bsonBuffer) {}

func addHostMetrics(bbuf *bsonBuffer, index *int) {}

func isPhysicalInterface(ifname string) bool { return true }
