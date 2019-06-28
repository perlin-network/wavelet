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

package api

import (
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"
	"testing"
)

func TestSendTransactionRequestTag(t *testing.T) {
	req := new(sendTransactionRequest)

	// test missing tag
	missing := `
		{
			"sender": "3132333435363738393031323334353637383930313233343536373839303132",
			"payload": "7061796C6F6164",
			"signature": "31323334353637383930313233343536373839303132333435363738393031323132333435363738393031323334353637383930313233343536373839303132"
		}
	`
	assert.Error(t, req.bind(&fastjson.Parser{}, []byte(missing)))

	// test send tag as string
	typeMismatch := `
		{
			"tag": "4",
			"sender": "3132333435363738393031323334353637383930313233343536373839303132",
			"payload": "7061796C6F6164",
			"signature": "31323334353637383930313233343536373839303132333435363738393031323132333435363738393031323334353637383930313233343536373839303132"
		}
	`
	assert.Error(t, req.bind(&fastjson.Parser{}, []byte(typeMismatch)))

	// test send tag as signed integer
	typeSigned := `
		{
			"tag": -1,
			"sender": "3132333435363738393031323334353637383930313233343536373839303132",
			"payload": "7061796C6F6164",
			"signature": "31323334353637383930313233343536373839303132333435363738393031323132333435363738393031323334353637383930313233343536373839303132"
		}
	`
	assert.Error(t, req.bind(&fastjson.Parser{}, []byte(typeSigned)))

	// test send invalid tag
	typeInvalid := `
		{
			"tag": 10,
			"sender": "3132333435363738393031323334353637383930313233343536373839303132",
			"payload": "7061796C6F6164",
			"signature": "31323334353637383930313233343536373839303132333435363738393031323132333435363738393031323334353637383930313233343536373839303132"
		}
	`
	assert.Error(t, req.bind(&fastjson.Parser{}, []byte(typeInvalid)))
}

func TestSendTransactionRequestMissingFields(t *testing.T) {
	req := new(sendTransactionRequest)

	// test missing sender
	missingSender := `
		{
			"tag": 4,
			"payload": "7061796C6F6164",
			"signature": "31323334353637383930313233343536373839303132333435363738393031323132333435363738393031323334353637383930313233343536373839303132"
		}
	`
	assert.Error(t, req.bind(&fastjson.Parser{}, []byte(missingSender)))

	// test missing payload
	missingPayload := `
		{
			"tag": 4,
			"sender": "3132333435363738393031323334353637383930313233343536373839303132",
			"signature": "31323334353637383930313233343536373839303132333435363738393031323132333435363738393031323334353637383930313233343536373839303132"
		}
	`
	assert.Error(t, req.bind(&fastjson.Parser{}, []byte(missingPayload)))

	// test missing signature
	missingSignature := `
		{
			"tag": "4",
			"sender": "3132333435363738393031323334353637383930313233343536373839303132",
			"payload": "7061796C6F6164"
		}
	`
	assert.Error(t, req.bind(&fastjson.Parser{}, []byte(missingSignature)))
}
