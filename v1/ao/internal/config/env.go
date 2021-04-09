// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/log"
	"github.com/pkg/errors"
)

func toBool(s string) (bool, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "yes" || s == "true" || s == "enabled" {
		return true, nil
	} else if s == "no" || s == "false" || s == "disabled" {
		return false, nil
	}
	return false, fmt.Errorf("cannot convert %s to bool", s)
}

// c must be a pointer to a struct object
func loadEnvsInternal(c interface{}) {
	cv := reflect.Indirect(reflect.ValueOf(c))
	ct := cv.Type()

	if !cv.CanSet() {
		return
	}

	for i := 0; i < ct.NumField(); i++ {
		fieldV := reflect.Indirect(cv.Field(i))
		if !fieldV.CanSet() || ct.Field(i).Anonymous {
			continue
		}

		field := ct.Field(i)
		fieldK := fieldV.Kind()
		if fieldK == reflect.Struct {
			// Need to use its pointer, otherwise it won't be addressable after
			// passed into the nested method
			loadEnvsInternal(getValPtr(cv.Field(i)).Interface())
		} else {
			tagV := field.Tag.Get("env")
			if tagV == "" {
				continue
			}

			envVal := os.Getenv(tagV)
			if envVal == "" {
				continue
			}

			val, err := stringToValue(envVal, fieldV.Type())
			if err == nil {
				setField(c, "Set", field, val)
			}
		}
	}
}

// stringToValue converts a string to a value of the type specified by typ.
func stringToValue(s string, typ reflect.Type) (reflect.Value, error) {
	s = strings.TrimSpace(s)

	var val interface{}
	var err error

	kind := typ.Kind()
	switch kind {
	case reflect.Int:
		if s == "" {
			s = "0"
		}
		val, err = strconv.Atoi(s)
		if err != nil {
			log.Warningf("Ignore invalid int value: %s", s)
		}
	case reflect.Int64:
		if s == "" {
			s = "0"
		}
		val, err = strconv.ParseInt(s, 10, 64)
		if err != nil {
			log.Warningf("Ignore invalid int64 value: %s", s)
		}
	case reflect.Float64:
		if s == "" {
			s = "0"
		}
		val, err = strconv.ParseFloat(s, 64)
		if err != nil {
			log.Warningf("Ignore invalid float64 value: %s", s)
		}
	case reflect.String:
		val = s
	case reflect.Bool:
		if s == "" {
			s = "false"
		}
		val, err = toBool(s)
		if err != nil {
			log.Warningf("Ignore invalid bool value: %s", errors.Wrap(err, s))
		}
	case reflect.Slice:
		if s == "" {
			return reflect.Zero(typ), nil
		} else {
			panic(fmt.Sprintf("Slice with non-empty value is not supported"))
		}

	default:
		panic(fmt.Sprintf("Unsupported kind: %v, val: %s", kind, s))
	}
	// convert to the target type as `typ` may be a user defined type
	return reflect.ValueOf(val).Convert(typ), err
}
