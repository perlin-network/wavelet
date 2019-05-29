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
			"payload": "payload"
			"signature": "31323334353637383930313233343536373839303132333435363738393031323132333435363738393031323334353637383930313233343536373839303132"
		}
	`
	assert.Error(t, req.bind(&fastjson.Parser{}, []byte(missing)))

	// test send tag as string
	typeMismatch := `
		{
			"tag": "4",
			"sender": "3132333435363738393031323334353637383930313233343536373839303132",
			"payload": "payload"
			"signature": "31323334353637383930313233343536373839303132333435363738393031323132333435363738393031323334353637383930313233343536373839303132"
		}
	`
	assert.Error(t, req.bind(&fastjson.Parser{}, []byte(typeMismatch)))

	// test send tag as signed integer
	typeSigned := `
		{
			"tag": -1,
			"sender": "3132333435363738393031323334353637383930313233343536373839303132",
			"payload": "payload"
			"signature": "31323334353637383930313233343536373839303132333435363738393031323132333435363738393031323334353637383930313233343536373839303132"
		}
	`
	assert.Error(t, req.bind(&fastjson.Parser{}, []byte(typeSigned)))

	// test send invalid tag
	typeInvalid := `
		{
			"tag": 10,
			"sender": "3132333435363738393031323334353637383930313233343536373839303132",
			"payload": "payload"
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
			"payload": "payload"
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
			"payload": "payload"
		}
	`
	assert.Error(t, req.bind(&fastjson.Parser{}, []byte(missingSignature)))
}
