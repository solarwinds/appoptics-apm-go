// Copyright (C) 2017 Librato, Inc. All rights reserved.

package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToBool(t *testing.T) {
	b, err := toBool("enabled")
	assert.True(t, b)
	assert.Nil(t, err)

	b, err = toBool("disabled")
	assert.False(t, b)
	assert.Nil(t, err)

	b, err = toBool("yes")
	assert.True(t, b)
	assert.Nil(t, err)

	b, err = toBool("true")
	assert.True(t, b)
	assert.Nil(t, err)

	b, err = toBool("YES")
	assert.True(t, b)
	assert.Nil(t, err)

	b, err = toBool("no")
	assert.False(t, b)
	assert.Nil(t, err)

	b, err = toBool("false")
	assert.False(t, b)
	assert.Nil(t, err)

	b, err = toBool("NO")
	assert.False(t, b)
	assert.Nil(t, err)

	b, err = toBool("invalid")
	assert.False(t, b)
	assert.NotNil(t, err)
}

func TestStringToValue(t *testing.T) {
	typInt := reflect.TypeOf(1)
	typInt64 := reflect.TypeOf(int64(1))
	typString := reflect.TypeOf("a")
	typBool := reflect.TypeOf(false)

	type NewStr string
	typNewStr := reflect.TypeOf(NewStr("newStr"))

	assert.Equal(t, stringToValue("1", typInt).Interface(), 1)
	assert.Equal(t, stringToValue("1", typInt).Kind(), reflect.Int)

	assert.Equal(t, stringToValue("", typInt).Interface(), 0)
	assert.Equal(t, stringToValue("a", typInt).Interface(), 0)

	assert.Equal(t, stringToValue("1", typInt64).Interface(), int64(1))
	assert.Equal(t, stringToValue("1", typInt64).Kind(), reflect.Int64)
	assert.Equal(t, stringToValue("", typInt64).Interface(), int64(0))
	assert.Equal(t, stringToValue("a", typInt64).Interface(), int64(0))

	assert.Equal(t, stringToValue("a", typString).Interface(), "a")
	assert.Equal(t, stringToValue("a", typString).Kind(), reflect.String)
	assert.Equal(t, stringToValue("1", typString).Interface(), "1")
	assert.Equal(t, stringToValue("A", typString).Interface(), "A")

	assert.Equal(t, stringToValue("yes", typBool).Interface(), true)
	assert.Equal(t, stringToValue("yes", typBool).Kind(), reflect.Bool)
	assert.Equal(t, stringToValue("false", typBool).Interface(), false)

	assert.Equal(t, stringToValue("hello", typNewStr).Interface(), NewStr("hello"))
	assert.Equal(t, stringToValue("hello", typNewStr).Type(), reflect.TypeOf(NewStr("hello")))
}
