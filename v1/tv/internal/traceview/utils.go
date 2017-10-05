// Copyright (C) 2017 Librato, Inc. All rights reserved.

package traceview

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type DebugLevel uint8

const (
	DEBUG DebugLevel = iota
	INFO
	WARNING
	ERROR
)

var dbgLevels = map[DebugLevel]string{
	DEBUG:   "DEBUG",
	INFO:    "INFO ",
	WARNING: "WARN ",
	ERROR:   "ERROR",
}

// OboeLog print logs based on the debug level.
func OboeLog(level DebugLevel, msg string, args ...interface{}) {
	if !debugLog || level < debugLevel {
		return
	}
	var p string
	pc, f, l, ok := runtime.Caller(1)
	if ok {
		path := strings.Split(runtime.FuncForPC(pc).Name(), ".")
		name := path[len(path)-1]
		p = fmt.Sprintf("%s %s#%d %s(): ", dbgLevels[level], filepath.Base(f), l, name)
	} else {
		p = fmt.Sprintf("%s %s#%s %s(): ", level, "na", "na", "na")
	}
	if len(args) == 0 {
		log.Printf("%s%s", p, msg)
	} else {
		log.Printf("%s%s %v", p, msg, args)
	}
}

// getLineByKeword reads a file, searches for the keyword and returns the matched line.
// It returns empty string "" if no match found or failed to open the path.
// Pass an empty string "" if you just need to get the first line.
func getLineByKeyword(path string, keyword string) string {
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

// getStrByKeyword read a file, searches for the keyword and returns the matched line
// with trailing line-feed character trimmed.
func getStrByKeyword(path string, keyword string) string {
	return strings.Trim(getLineByKeyword(path, keyword), "\n")
}

// getStrByKeywordFiles does the same thing as getStrByKeyword but searches for a list
// of files and returns the first matched files and line
func getStrByKeywordFiles(pathes []string, keyword string) (path string, line string) {
	for _, path = range pathes {
		line = getStrByKeyword(path, keyword)
		if line != "" {
			return path, line
		}
	}
	return "", ""
}

func Min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func Max(x, y int) int {
	if x > y {
		return x
	}
	return y
}
