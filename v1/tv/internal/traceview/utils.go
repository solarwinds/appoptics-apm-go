// Copyright (C) 2017 Librato, Inc. All rights reserved.

package traceview

import (
	"log"
	"os"
	"bufio"
	"strings"
)

type DebugLevel uint8

const (
	DEBUG DebugLevel = iota
	INFO
	WARNING
	ERROR
)

// OboeLog print logs based on the debug level.
func OboeLog(level DebugLevel, msg string, err error) {
	if !debugLog { // remove it
		return
	}
	if level >= debugLevel {
		log.Printf("%s: %v", msg, err)
	}
}

// getLineByKeword reads a file, searches for the keyword and returns the matched line.
// It returns empty string "" if no match found or failed to open the path.
func getLineByKeyword(path string, keyword string) string {
	if path == "" {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		OboeLog(INFO, "Failed to open file", err) //TODO: does err reports the path name?
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
