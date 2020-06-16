// Copyright (C) 2017 Librato, Inc. All rights reserved.

package opentracing

import "github.com/opentracing/opentracing-go/ext"

// Map selected OpenTracing tag constants to AppOptics analogs
var otAOMap = map[string]string{
	string(ext.Component): "OTComponent",

	string(ext.PeerService):  "RemoteController",
	string(ext.PeerAddress):  "RemoteURL",
	string(ext.PeerHostname): "RemoteHost",

	string(ext.HTTPUrl):        "URL",
	string(ext.HTTPMethod):     "Method",
	string(ext.HTTPStatusCode): "Status",

	string(ext.DBInstance):  "Database",
	string(ext.DBStatement): "Query",
	string(ext.DBType):      "Flavor",

	string("resource.name"): "TransactionName",
}

func translateTagName(key string) string {
	if k := otAOMap[key]; k != "" {
		return k
	}
	return key
}

func translateTags(tags map[string]interface{}) map[string]interface{} {
	ret := make(map[string]interface{})
	for k, v := range tags {
		ret[translateTagName(k)] = v
	}
	return ret
}
