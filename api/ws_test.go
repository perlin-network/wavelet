package api

import (
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"
	"testing"
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

	assert.True(t, valueEqual(v.Get("str"), "bar"))
	assert.True(t, valueEqual(v.Get("int"), "123"))
	assert.True(t, valueEqual(v.Get("float"), "1.23"))
	assert.True(t, valueEqual(v.Get("bool"), "true"))
	assert.True(t, valueEqual(v.Get("obj"), `{"key":"value"}`))
	assert.True(t, valueEqual(v.Get("arr"), `[1,"str"]`))
}
