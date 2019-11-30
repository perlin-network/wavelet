// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

// +build !integration,unit

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"
)

func TestSinkEqual(t *testing.T) {
	var p fastjson.Parser
	v, err := p.Parse(`{
                "str": "bar",
                "int": 123,
                "float": 1.23,
                "bool": true,
				"obj" : {"key": "value"},
                "arr": [1, "str"]
        }`)
	assert.NoError(t, err)

	assert.True(t, fastjsonEquals(v.Get("str"), "bar"))
	assert.True(t, fastjsonEquals(v.Get("int"), "123"))
	assert.True(t, fastjsonEquals(v.Get("float"), "1.23"))
	assert.True(t, fastjsonEquals(v.Get("bool"), "true"))
	assert.True(t, fastjsonEquals(v.Get("obj"), `{"key":"value"}`))
	assert.True(t, fastjsonEquals(v.Get("arr"), `[1,"str"]`))
}
