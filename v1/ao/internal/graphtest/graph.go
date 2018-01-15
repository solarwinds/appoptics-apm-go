// Copyright (C) 2016 Librato, Inc. All rights reserved.

// Package graphtest provides test utilities for asserting properties of event graphs.
package graphtest

import (
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/mgo.v2/bson"
)

// assert that each condition in a switch statement only occurs once.
func assertOnce(t *testing.T) {
	s := string(debug.Stack())
	_, contains := seenStacks[s]
	assert.False(t, contains, fmt.Sprintf("seeing multiple %s", s))
	seenStacks[s] = true
}

var seenStacks = make(map[string]bool)

// Node is a decoded event report used for testing assertions.
type Node struct {
	Layer, Label string
	OpID         string
	Edges        []string
	Map          map[string]interface{}
	Flag         uint8
}
type eventGraph map[string]Node

func buildGraph(t *testing.T, bufs [][]byte) eventGraph {
	t.Logf("got %v events\n", len(bufs))
	g := make(eventGraph)
	for i, buf := range bufs {
		d := bson.D{}
		err := bson.Unmarshal(buf, &d)
		assert.NoError(t, err)
		n := Node{Map: make(map[string]interface{})}
		if os.Getenv("LOG_EVENTS") != "" {
			t.Logf("# event %v\n", i)
		}
		for _, v := range d {
			switch v.Name {
			case "Edge":
				n.Edges = append(n.Edges, v.Value.(string))
			case "Layer":
				n.Layer = v.Value.(string)
			case "Label":
				n.Label = v.Value.(string)
			case "X-Trace":
				n.OpID = v.Value.(string)[42:58]

				buf := v.Value.(string)[58:60]
				flag := make([]byte, 1)
				if _, err := hex.Decode(flag, []byte(buf)); err != nil {
					n.Flag = buf[0]
				}
				fallthrough
			default:
				n.Map[v.Name] = v.Value
			}
			if os.Getenv("LOG_EVENTS") != "" {
				t.Logf("got kv %v\n", v)
			}
		}
		g[n.OpID] = n
	}
	return g
}

// MatchNode describes a node by its Layer and Label, used to match for assertions about a node and
// when listing its outedges.
type MatchNode struct{ Layer, Label string }

// MatchNodeKV allows a test to assert properties of a node by specifying its Layer, Label and a KV
// pair.
type MatchNodeKV struct{ Layer, Label, K, V string }

// Edges is a list of outedges to assert on.
type Edges []MatchNode

// NodeAsserter checks a list of outedges and calls cb to run more asserts for this node.
type NodeAsserter struct { // run to assert each Node
	Edges    Edges
	Callback func(n Node)
	Count    int
	Seen     bool
}

// an AsserterMap looks up NodeAsserters for Nodes in a graph, keeping track of which
// have been seen.
type AsserterMap interface {
	Match(n Node) (NodeAsserter, bool)
	Size() int
	AssertSeen(t *testing.T, n Node)
	AssertMissing(t *testing.T)
}

type matcher interface {
	Match(n Node) (mn matcher, ok bool)
}

// An AssertNodeMap describes a list of nodes by {Layer, Label} and assertions about them.
type AssertNodeMap map[MatchNode]NodeAsserter

// An AssertNodeKVMap describes a list of nodes by {Layer, Label, K, V} and assertions about them.
type AssertNodeKVMap map[MatchNodeKV]NodeAsserter

// Match a node, returning an asserter.
func (m AssertNodeMap) Match(n Node) (NodeAsserter, bool) {
	ret, ok := m[MatchNode{n.Layer, n.Label}]
	return ret, ok
}

// Size returns the number of nodes in the asserted graph.
func (m AssertNodeMap) Size() (ret int) {
	for _, a := range m {
		switch {
		case a.Count == 0:
			ret++
		case a.Count > 0:
			ret += a.Count
		}
	}
	return
}

// AssertSeen ensures each node is seen at most once.
func (m AssertNodeMap) AssertSeen(t *testing.T, n Node) {
	mn := MatchNode{n.Layer, n.Label}
	if m[mn].Count > 0 {
		asserter := m[mn]
		asserter.Count--
		m[mn] = asserter
	}
	if m[mn].Count == 0 {
		assert.False(t, m[mn].Seen, "Already saw node %v match %v", n, mn)
		asserter := m[mn]
		asserter.Seen = true
		m[mn] = asserter
	}
}

// AssertMissing ensures each node is seen.
func (m AssertNodeMap) AssertMissing(t *testing.T) {
	for mn, a := range m {
		assert.True(t, m[mn].Seen, "Didn't see node %v edges %v", mn, a)
	}
}

// Match a node by KV pair, returning an asserter.
func (m AssertNodeKVMap) Match(n Node) (ret NodeAsserter, ok bool) {
	var mn MatchNodeKV
	if mn, ok = m.match(n); ok {
		ret, ok = m[mn]
	}
	return
}
func (m AssertNodeKVMap) match(n Node) (mn MatchNodeKV, ok bool) {
	// look for node with same KV pair as specified in assert structure
	for k, v := range n.Map {
		var vs string
		if vs, ok = v.(string); !ok {
			continue
		}
		mn = MatchNodeKV{n.Layer, n.Label, k, vs}
		if _, ok = m[mn]; ok {
			return
		}
	}
	// or look for node matching just on label/layer, no KV pair
	if _, ok = m[MatchNodeKV{n.Layer, n.Label, "", ""}]; ok {
		mn = MatchNodeKV{n.Layer, n.Label, "", ""}
		return
	}
	return
}

// Size returns the number of nodes in the asserted graph.
func (m AssertNodeKVMap) Size() (ret int) {
	for _, a := range m {
		switch {
		case a.Count == 0:
			ret++
		case a.Count > 0:
			ret += a.Count
		}
	}
	return
}

// AssertSeen ensures each node is seen at most once.
func (m AssertNodeKVMap) AssertSeen(t *testing.T, n Node) {
	mn, ok := m.match(n)
	assert.True(t, ok)
	if m[mn].Count > 0 {
		asserter := m[mn]
		asserter.Count--
		m[mn] = asserter
	}
	if m[mn].Count == 0 {
		assert.False(t, m[mn].Seen, "Already saw node %v match %v", n, mn)
		asserter := m[mn]
		asserter.Seen = true
		m[mn] = asserter
	}
}

// AssertMissing ensures each node is seen.
func (m AssertNodeKVMap) AssertMissing(t *testing.T) {
	for mn, a := range m {
		assert.True(t, m[mn].Seen, "Didn't see node %v edges %v", mn, a)
	}
}

var checkedEdges = 0
var checkedNodes = 0

// AssertGraph builds a graph from encoded events and asserts properties and edges about each node
// in asserterMap.
func AssertGraph(t *testing.T, bufs [][]byte, numNodes int, asserterMap AsserterMap) {
	assert.Equal(t, len(bufs), numNodes, "bufs len expected %d, actual %d", numNodes, len(bufs))
	g := buildGraph(t, bufs)
	assert.Equal(t, len(g), numNodes, "graph len expected %d, actual %d", numNodes, len(g))
	assert.Equal(t, asserterMap.Size(), numNodes)
	for op, n := range g {
		assert.Equal(t, op, n.OpID)
		// assert edges for this node
		asserter, ok := asserterMap.Match(n)
		assert.True(t, ok, "Unrecognized event: "+fmt.Sprintf("%v", n))
		assertOutEdges(t, g, n, asserter.Edges...)
		// call assert cb if provided
		if asserter.Callback != nil {
			asserter.Callback(n)
		}
		asserterMap.AssertSeen(t, n)
		checkedNodes++
	}
	asserterMap.AssertMissing(t)

	t.Logf("Total %d nodes, %d edges checked", checkedNodes, checkedEdges)

	if os.Getenv("DOT_GRAPHS") != "" { // save graph to file named for caller
		var pc uintptr
		var line int
		funcDepth := func(d int) string {
			pc, _, line, _ = runtime.Caller(d)
			f := runtime.FuncForPC(pc).Name()
			return f[strings.LastIndex(f, "/")+1:]
		}
		caller := funcDepth(1)
		for i := 2; strings.HasPrefix(strings.ToLower(caller), "ao_test.assert") ||
			strings.HasPrefix(caller, "graphtest.") ||
			strings.HasPrefix(caller, "ao_test.test"); i++ {
			caller = funcDepth(i)
		}
		fname := fmt.Sprintf("graph_%s-%d_%d.dot", caller, line, os.Getpid())
		if dir := os.Getenv("DOT_GRAPHDIR"); dir != "" {
			fname = filepath.Join(dir, fname)
		}
		output, _ := os.Create(fname)
		defer output.Close()
		t.Logf("Saving DOT graph %s", fname)
		dotGraph(g, output)
	}
}

func assertOutEdges(t *testing.T, g eventGraph, n Node, edges ...MatchNode) {
	assert.Equal(t, len(n.Edges), len(edges),
		"[layer %s label %s] len(n.Edges) %d expected %d", n.Layer, n.Label, len(n.Edges), len(edges))
	foundEdges := 0
	if len(edges) <= len(n.Edges) {
		for i, edge := range edges {
			checkedEdges++
			// assert edge to op ID of unreported event
			if edge.Layer == "Edge" {
				assert.Equal(t, n.Edges[i], edge.Label)
				if n.Edges[i] == edge.Label {
					foundEdges++
					continue
				}
			}
			// check if node for this edge exists and assert Label, Layer
			_, ok := g[n.Edges[i]]
			assert.True(t, ok, "Edge from {%s, %s} missing to {%s, %s} no node %d", n.Layer, n.Label, edge.Layer, edge.Label, i)
			assert.Equal(t, edge.Layer, g[n.Edges[i]].Layer,
				"[layer %s label %s] missing edge to {%s, %s} actual %d {%s, %s}", n.Layer, n.Label, edge.Layer, edge.Label, i, g[n.Edges[i]].Layer, g[n.Edges[i]].Label)
			assert.Equal(t, edge.Label, g[n.Edges[i]].Label,
				"[layer %s label %s] missing edge to {%s, %s} actual %d {%s, %s}", n.Layer, n.Label, edge.Layer, edge.Label, i, g[n.Edges[i]].Layer, g[n.Edges[i]].Label)
			if edge.Layer == g[n.Edges[i]].Layer && edge.Label == g[n.Edges[i]].Label {
				foundEdges++
			}
		}
		assert.Equal(t, foundEdges, len(edges))
	}
}

// dotGraph writes a graphviz dot file to output Writer
func dotGraph(g eventGraph, output io.Writer) {
	fmt.Fprintln(output, "digraph main{")
	fmt.Fprintln(output, "\tedge[arrowhead=vee]")
	fmt.Fprintln(output, "\tgraph [rankdir=RL,compound=true,ranksep=1.0];")

	minT := int64(math.MaxInt64) // find min timestamp
	for _, n := range g {
		if ts, ok := n.Map["Timestamp_u"]; ok {
			if t, ok := ts.(int64); ok && t < minT {
				minT = t
			}
		}
	}

	for opID, n := range g {
		var ts int64
		if tval, ok := n.Map["Timestamp_u"]; ok {
			if t, ok := tval.(int64); ok {
				ts = t
			}
		}
		var suffix string

		blocked := make(map[string]bool)
		for _, k := range []string{"X-Trace", "Backtrace", "Timestamp_u", "Hostname", "_V", "PID"} {
			blocked[k] = true
		}
		for k, v := range n.Map {
			if !blocked[k] {
				switch vv := v.(type) {
				case []byte:
					suffix += fmt.Sprintf("\\n%s: %s", k, v)
				case string:
					suffix += fmt.Sprintf("\\n%s: %s", k, strings.Replace(vv, `"`, `\"`, -1))
				default:
					suffix += fmt.Sprintf("\\n%s: %v", k, v)
				}
			}
		}
		fmt.Fprintf(output, "\top%s[shape=%s,label=\"%s\"];\n",
			opID, "box",
			fmt.Sprintf("%s: %s\\n%s\\n%0.3fms%s", n.Layer, n.Label, opID, float64(ts-minT)/1000.0, suffix))

		for _, target := range n.Edges {
			fmt.Fprintf(output, "\top%s -> op%s;\n", opID, target)
		}
	}

	fmt.Fprintln(output, "}")
}
