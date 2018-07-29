// Copyright (C) 2017 Librato, Inc. All rights reserved.

package utils

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"gopkg.in/mgo.v2/bson"
)

// PrintBson prints the BSON message. It's not concurrent-safe and is for testing only
func PrintBson(message []byte) {
	m := make(map[string]interface{})
	bson.Unmarshal(message, m)
	b, _ := json.MarshalIndent(m, "", "  ")
	fmt.Println(time.Now().Format("15:04:05"), string(b))
}

// GetLineByKeyword reads a file, searches for the keyword and returns the matched line.
// It returns empty string "" if no match found or failed to open the path.
// Pass an empty string "" if you just need to get the first line.
func GetLineByKeyword(path string, keyword string) string {
	if path == "" {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if line := scanner.Text(); strings.Contains(line, keyword) {
			return line
		}
	}
	// ignore any scanner.Err(), just return an empty string.
	return ""
}

// GetStrByKeyword read a file, searches for the keyword and returns the matched line
// with trailing line-feed character trimmed.
func GetStrByKeyword(path string, keyword string) string {
	return strings.Trim(GetLineByKeyword(path, keyword), "\n")
}

// GetStrByKeywordFiles does the same thing as GetStrByKeyword but searches for a list
// of files and returns the first matched files and line
func GetStrByKeywordFiles(pathes []string, keyword string) (path string, line string) {
	for _, path = range pathes {
		line = GetStrByKeyword(path, keyword)
		if line != "" {
			return path, line
		}
	}
	return "", ""
}

// Min returns the lower value
func Min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// Max returns the greater value
func Max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

// Byte2String converts a byte array into a string
func Byte2String(bs []int8) string {
	b := make([]byte, len(bs))
	for i, v := range bs {
		b[i] = byte(v)
	}
	return string(b)
}

// CopyMap makes a copy of all elements of a map.
func CopyMap(from *map[string]string) map[string]string {
	to := make(map[string]string)
	for k, v := range *from {
		to[k] = v
	}

	return to
}

// IsHigherOrEqualGoVersion checks if go version is higher or equal to the given version
func IsHigherOrEqualGoVersion(version string) bool {
	goVersion := strings.Split(runtime.Version(), ".")
	compVersion := strings.Split(version, ".")
	for i := 0; i < len(goVersion) && i < len(compVersion); i++ {
		l := len(compVersion[i])
		if len(goVersion[i]) > l {
			l = len(goVersion[i])
		}
		compVersion[i] = strings.Repeat("0", l-len(compVersion[i])) + compVersion[i]
		goVersion[i] = strings.Repeat("0", l-len(goVersion[i])) + goVersion[i]
		if strings.Compare(goVersion[i], compVersion[i]) == 1 {
			return true
		} else if strings.Compare(goVersion[i], compVersion[i]) == -1 {
			return false
		}
	}
	return true
}

// SafeBuffer is goroutine-safe buffer. It is for internal test use only.
type SafeBuffer struct {
	buf bytes.Buffer
	sync.Mutex
}

func (b *SafeBuffer) Read(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.buf.Read(p)
}

func (b *SafeBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.buf.Write(p)
}

func (b *SafeBuffer) String() string {
	b.Lock()
	defer b.Unlock()
	return b.buf.String()
}

// Reset truncates the buffer
func (b *SafeBuffer) Reset() {
	b.Lock()
	defer b.Unlock()
	b.buf.Reset()
}
