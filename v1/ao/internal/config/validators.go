// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// InvalidEnv returns a string indicating invalid environment variables
func InvalidEnv(env string, val string) string {
	return fmt.Sprintf("invalid env, discarded - %s: \"%s\"", env, val)
}

// MissingEnv returns a string indicating missing environment variables
func MissingEnv(env string) string {
	return fmt.Sprintf("missing env - %s", env)
}

// NonDefaultEnv returns a string indicating non-default environment variables
func NonDefaultEnv(env string, val string) string {
	return fmt.Sprintf("env found - %s: \"%s\"", env, val)
}

const (
	validServiceKeyPattern = `^[a-zA-Z0-9]{64}:.{1,255}$`

	serviceKeyPartsCnt  = 2
	serviceKeyDelimiter = ":"

	spacesPattern  = `\s`
	spacesReplacer = "-"

	invalidCharacters   = `[^a-z0-9.:_-]`
	invalidCharReplacer = ""
)

var (
	// IsValidServiceKey verifies if the service key is a valid one.
	// A valid service key is something like 'service_token:service_name'.
	// The service_token should be of 64 characters long and the size of
	// service_name is larger than 0 but up to 255 characters.
	IsValidServiceKey = regexp.MustCompile(validServiceKeyPattern).MatchString

	// ReplaceSpacesWith replaces all the spaces with valid characters (hyphen)
	ReplaceSpacesWith = regexp.MustCompile(spacesPattern).ReplaceAllString

	// RemoveInvalidChars remove invalid characters
	RemoveInvalidChars = regexp.MustCompile(invalidCharacters).ReplaceAllString
)

// ToServiceKey converts a string to a service key. The argument should be
// a valid service key string.
//
// It doesn't touch the service key but does the following to the original
// service name:
// - convert all characters to lowercase
// - convert spaces to hyphens
// - remove invalid characters ( [^a-z0-9.:_-])
func ToServiceKey(s string) interface{} {
	parts := strings.SplitN(s, serviceKeyDelimiter, serviceKeyPartsCnt)
	if len(parts) != serviceKeyPartsCnt {
		// This should not happen as this method is called after service key
		// validation, which rejects a key without the delimiter. This check
		// is added here to avoid out-of-bound slice access later.
		return s
	}

	sToken, sName := parts[0], parts[1]

	sName = strings.ToLower(sName)
	sName = ReplaceSpacesWith(sName, spacesReplacer)
	sName = RemoveInvalidChars(sName, invalidCharReplacer)

	return strings.Join([]string{sToken, sName}, serviceKeyDelimiter)
}

// IsValidHost verifies if the host is in a valid format
func IsValidHost(host string) bool {
	// TODO
	return true
}

// ToHost converts a string to a host in interface{} format
func ToHost(s string) interface{} {
	return s
}

// IsValidFileString checks if the string represents a valid file.
func IsValidFileString(file string) bool {
	// TODO
	return true
}

// ToFileString converts a string to an interface{} represents a file path
func ToFileString(file string) interface{} {
	path, _ := filepath.Abs(file)
	return path
}

// IsValidReporterType checks if the reporter type is valid.
func IsValidReporterType(t string) bool {
	t = strings.ToLower(strings.TrimSpace(t))
	return t == "ssl" || t == "udp"
}

// ToReporterType converts a string to a reporter type
func ToReporterType(t string) interface{} {
	return t
}

// IsValidTracingMode checks if the mode is valid
func IsValidTracingMode(m string) bool {
	t := strings.ToLower(strings.TrimSpace(m))
	return t == "never" || t == "always"
}

// IsValidSampleRate checks if the rate is valid
func IsValidSampleRate(m string) bool {
	rate, err := strconv.Atoi(m)
	if err != nil {
		return false
	}
	return rate >= 0 && rate <= maxSampleRate
}

// ToTracingMode converts a string to a tracing mode
func ToTracingMode(m string) interface{} {
	return strings.ToLower(strings.TrimSpace(m))
}

// IsValidBool checks if the string represents a valid boolean value
func IsValidBool(b string) bool {
	t := strings.ToLower(strings.TrimSpace(b))
	return t == "true" || t == "false"
}

// ToBool converts a string to a boolean, the string must have been validated.
func ToBool(b string) interface{} {
	return strings.ToLower(strings.TrimSpace(b)) == "true"
}

// IsValidHostnameAlias checks if the alias is valid
func IsValidHostnameAlias(a string) bool {
	return a != ""
}

// ToHostnameAlias converts a string to a hostname alias
func ToHostnameAlias(a string) interface{} {
	return a
}

// IsValidInteger checks if the string represents a valid integer
func IsValidInteger(i string) bool {
	_, valid := strconv.Atoi(i)
	return valid == nil
}

// ToInteger converts a string to an integer
func ToInteger(i string) interface{} {
	n, _ := strconv.Atoi(i)
	return n
}

// ToInt64 converts a string to an int64
func ToInt64(i string) interface{} {
	n, _ := strconv.Atoi(i)
	return int64(n)
}

// MaskServiceKey masks the middle part of the token and returns the
// masked service key. For example:
// key: "ae38315f6116585d64d82ec2455aa3ec61e02fee25d286f74ace9e4fea189217:go"
// masked:"ae38********************************************************9217:go"
func MaskServiceKey(validKey string) string {
	var sep = ":"
	var hLen, tLen = 4, 4
	var mask = "*"

	s := strings.Split(validKey, sep)
	tk := s[0]

	if len(tk) <= hLen+tLen {
		return validKey
	}

	tk = tk[0:4] + strings.Repeat(mask,
		utf8.RuneCountInString(tk)-hLen-tLen) + tk[len(tk)-4:]

	return tk + sep + s[1]
}
