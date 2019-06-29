package wavelet

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/ioutil"

	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
)

// TransactionParserJSON defines a generic JSON transaction payload parser.
type TransactionParserJSON struct {
	Tag string // Transaction tag
}

var (
	// ErrNoTag defines an error describing an empty Tag.
	ErrNoTag = errors.New("no tag specified")

	// ErrInvalidTag defines an error describing an invalid TransactionTag.
	ErrInvalidTag = errors.New("tag is invalid")

	// ErrCouldNotParse defines an error describing the inability to parse a given payload.
	ErrCouldNotParse = errors.New("could not parse the given payload")

	// ErrNilField defines an error describing a field value equal to nil.
	ErrNilField = errors.New("field is nil")

	// ErrInvalidOperation defines an error describing an invalid operation value.
	ErrInvalidOperation = errors.New("operation is invalid")

	// ErrInvalidAccountIDSize defines an error describing an invalid account ID size.
	ErrInvalidAccountIDSize = errors.New("account ID is of an invalid size")
)

/* BEGIN EXPORTED METHODS */

// NewTransactionParserJSON initializes a new transaction JSON parser with the given transaction type.
func NewTransactionParserJSON(tag string) *TransactionParserJSON {
	return &TransactionParserJSON{
		Tag: tag, // Set tag
	} // Initialize parser
}

// ParseJSON parses the given JSON payload input.
func (parser *TransactionParserJSON) ParseJSON(data []byte) ([]byte, error) {
	if parser.Tag == "" { // Check no transaction tag
		return nil, ErrNoTag // Return no tag error
	}

	if !getValidTags()[parser.Tag] { // Check invalid
		return nil, ErrInvalidTag // Return error
	}

	switch parser.Tag { // Handle different tag types
	case "nop":
		return nil, nil // Nothing to do!
	case "transfer":
		return parser.parseTransfer(data) // Parse
	case "stake":
		return parser.parseStake(data) // Parse
	case "contract":
		return parser.parseContract(data) // Parse
	case "batch":
		return parser.parseBatch(data) // Parse
	}

	return nil, ErrCouldNotParse // Return error (shouldn't ever get here)
}

/* END EXPORTED METHODS */

/* BEGIN INTERNAL METHODS */

/*
	BEGIN TAG HANDLERS
*/

// parseTransfer parses a transaction payload with the transfer tag.
func (parser *TransactionParserJSON) parseTransfer(data []byte) ([]byte, error) {
	payload := bytes.NewBuffer(nil) // Initialize buffer

	var p fastjson.Parser // Initialize parser

	json, err := p.Parse(string(data)) // Parse json
	if err != nil {                    // Check for errors
		return nil, err // Return found error
	}

	if !json.Exists("recipient") || !json.Exists("amount") { // Check no value
		return nil, ErrNilField // Return nil field error
	}

	decodedRecipient, err := hex.DecodeString(string(json.GetStringBytes("recipient"))) // Decode recipient hex string
	if err != nil {                                                                     // Check for errors
		return nil, err // Return found error
	}

	if len(decodedRecipient) != SizeAccountID { // Check invalid length
		return nil, ErrInvalidAccountIDSize // Return invalid recipient ID error
	}

	var recipient AccountID              // Initialize ID buffer
	copy(recipient[:], decodedRecipient) // Copy address to buffer

	_, err = payload.Write(recipient[:]) // Write recipient value
	if err != nil {                      // Check for errors
		return nil, err // Return found error
	}

	decodedAmount := uint64(json.GetFloat64("amount")) // Get amount value

	var amount [8]byte                                      // Initialize integer buffer
	binary.LittleEndian.PutUint64(amount[:], decodedAmount) // Write to buffer

	_, err = payload.Write(amount[:]) // Write amount value
	if err != nil {                   // Check for errors
		return nil, err // Return found error
	}

	if json.Exists("gas_limit") { // Check has gas limit value
		decodedGasLimit := uint64(json.GetFloat64("gas_limit")) // Get uint64 gas limit value

		var gasLimit [8]byte                                        // Initialize integer buffer
		binary.LittleEndian.PutUint64(gasLimit[:], decodedGasLimit) // Write to buffer

		_, err = payload.Write(gasLimit[:]) // Write gas limit
		if err != nil {                     // Check for errors
			return nil, err // Return found error
		}
	}

	funcNameLength, funcName, funcParamsLength, funcParams, err := parseFunction(json) // Parse function
	if err != nil && err != ErrNilField {                                              // Check for errors
		return nil, err // Return found error
	}

	payload.Write(funcNameLength[:4])   // Write name length
	payload.WriteString(funcName)       // Write function name
	payload.Write(funcParamsLength[:4]) // Write length of function parameters
	payload.Write(funcParams)           // Write parameters

	return payload.Bytes(), nil // Return payload
}

// parseStake parses a transaction payload with the stake tag.
func (parser *TransactionParserJSON) parseStake(data []byte) ([]byte, error) {
	payload := bytes.NewBuffer(nil) // Initialize buffer

	var p fastjson.Parser // Initialize parser

	json, err := p.Parse(string(data)) // Parse json
	if err != nil {                    // Check for errors
		return nil, err // Return found error
	}

	operation, err := parseOperation(json) // Parse operation
	if err != nil {                        // Check for errors
		return nil, err // Return found error
	}

	decodedAmount := uint64(json.GetFloat64("amount")) // Get amount value

	var amount [8]byte                                      // Initialize integer buffer
	binary.LittleEndian.PutUint64(amount[:], decodedAmount) // Write to buffer

	err = payload.WriteByte(operation) // Write operation
	if err != nil {                    // Check for errors
		return nil, err // Return found error
	}

	_, err = payload.Write(amount[:]) // Write amount
	if err != nil {                   // Check for errors
		return nil, err // Return found error
	}

	return payload.Bytes(), nil // Return payload
}

// parseContract parses a transaction payload with the contract tag.
func (parser *TransactionParserJSON) parseContract(data []byte) ([]byte, error) {
	payload := bytes.NewBuffer(nil) // Initialize buffer

	var p fastjson.Parser // Initialize parser

	json, err := p.Parse(string(data)) // Parse json
	if err != nil {                    // Check for errors
		return nil, err // Return found error
	}

	if !json.Exists("gas_limit") { // Check no value
		return nil, ErrNilField // Return nil field error
	}

	decodedGasLimit := uint64(json.GetFloat64("gas_limit")) // Get uint64 gas limit value

	var gasLimit [8]byte                                        // Initialize integer buffer
	binary.LittleEndian.PutUint64(gasLimit[:], decodedGasLimit) // Write to buffer

	_, err = payload.Write(gasLimit[:]) // Write gas limit
	if err != nil {                     // Check for errors
		return nil, err // Return found error
	}

	functionPayloadLength, functionPayload, err := parseFunctionPayload(json) // Parse function payload
	if err != nil {                                                           // Check for errors
		return nil, err // Return found error
	}

	code, err := parseContractCode(json) // Parse contract code
	if err != nil {                      // Check for errors
		return nil, err // Return found error
	}

	_, err = payload.Write(gasLimit[:]) // Write to buffer
	if err != nil {                     // Check for errors
		return nil, err // Return found error
	}

	_, err = payload.Write(functionPayloadLength[:4]) // Write to buffer
	if err != nil {                                   // Check for errors
		return nil, err // Return found error
	}

	_, err = payload.Write(functionPayload) // Write to buffer
	if err != nil {                         // Check for errors
		return nil, err // Return found error
	}

	_, err = payload.Write(code) // Write contract code to buffer
	if err != nil {              // Check for errors
		return nil, err // Return found error
	}

	return payload.Bytes(), nil // Return payload
}

// parseBatch parses a transaction payload with the batch tag.
func (parser *TransactionParserJSON) parseBatch(data []byte) ([]byte, error) {
	payload := bytes.NewBuffer(nil) // Initialize buffer

	var p fastjson.Parser // Initialize parser

	json, err := p.Parse(string(data)) // Parse json
	if err != nil {                    // Check for errors
		return nil, err // Return found error
	}

	if !json.Exists("payloads") { // Check no value
		return nil, ErrNilField // Return nil field error
	}

	transactions := json.GetArray("payloads") // Get payloads

	var batchLength [8]byte // Initialize batch length buffer

	binary.LittleEndian.PutUint64(batchLength[:], uint64(len(transactions))) // Write batch length to buffer

	payload.Write(batchLength[:4]) // Write batch length

	for _, tx := range transactions { // Iterate through transactions
		txJSON, err := tx.StringBytes() // Get JSON
		if err != nil {                 // Check for errors
			return nil, err // Return found error
		}

		txParser := NewTransactionParserJSON(string(tx.GetStringBytes("tag"))) // Initialize parser

		txPayload, err := txParser.ParseJSON(txJSON) // Parse JSON
		if err != nil {                              // Check for errors
			return nil, err // Return found error
		}

		_, err = payload.Write(txPayload) // Write payload
		if err != nil {                   // Check for errors
			return nil, err // Return found error
		}
	}

	return payload.Bytes(), nil // Return payload
}

/*
	END TAG HANDLERS
*/

/*
	BEGIN PARSER HELPER METHODS
*/

// parseOperation gets the operation of a particular stake transaction.
func parseOperation(json *fastjson.Value) (byte, error) {
	if !json.Exists("operation") { // Check no value
		return byte(0), ErrNilField // Return nil field error
	}

	operation := json.GetInt("operation") // Get operation code

	if byte(operation) >= sys.TagBatch || byte(operation) < 0 { // Check invalid value
		return byte(0), ErrInvalidOperation // Return invalid operation error
	}

	switch json.GetInt("operation") { // Handle different operations
	case 0:
		return sys.WithdrawStake, nil // Return parsed
	case 1:
		return sys.PlaceStake, nil // Return parsed
	case 2:
		return sys.WithdrawReward, nil // Return parsed
	}

	return byte(0), ErrInvalidOperation // Return invalid operation error
}

// parseFunction gets a payload's target function, as well as the parameters
// corresponding to such a function.
func parseFunction(json *fastjson.Value) ([8]byte, string, [8]byte, []byte, error) {
	if !json.Exists("fn_name") { // Check no value
		return [8]byte{}, "", [8]byte{}, nil, ErrNilField // Return nil field error
	}

	funcName := string(json.GetStringBytes("fn_name")) // Get function name

	funcParamsLength, funcParams, err := parseFunctionPayload(json) // Parse payload
	if err != nil {                                                 // Check for errors
		return [8]byte{}, "", [8]byte{}, nil, err // Return found error
	}

	var functionNameLength [8]byte                                               // Initialize dedicated len buffer
	binary.LittleEndian.PutUint32(functionNameLength[:4], uint32(len(funcName))) // Write to buffer

	return functionNameLength, funcName, funcParamsLength, funcParams, nil // Return params
}

// parseFunctionPayload gets a payload's target function's payload.
func parseFunctionPayload(json *fastjson.Value) ([8]byte, []byte, error) {
	if !json.Exists("fn_payload") { // Check no value
		return [8]byte{}, nil, ErrNilField // Return nil field error
	}

	var intBuf [8]byte // Initialize integer buffer

	params := bytes.NewBuffer(nil) // Initialize payload buffer

	for _, payloadValue := range json.GetArray("fn_payload") { // Iterate through payloads
		payload := payloadValue.String() // Convert to string

		switch payload[0] {
		case 'S':
			binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(payload[1:]))) // Write to buffer
			params.Write(intBuf[:4])                                            // Write to buffer
			params.WriteString(payload[1:])                                     // Write to buffer
		case 'B':
			binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(payload[1:]))) // Write to buffer
			params.Write(intBuf[:4])                                            // Write to buffer
			params.Write([]byte(payload[1:]))                                   // Write to buffer
		case '1', '2', '4', '8':
			var val uint64 // Initialize value buffer

			_, err := fmt.Sscanf(payload[1:], "%d", &val) // Scan payload into value buffer
			if err != nil {                               // Check for errors
				return [8]byte{}, nil, err // Return found error
			}

			switch payload[0] { // Handle different integer sizes
			case '1':
				params.WriteByte(byte(val)) // Write to buffer
			case '2':
				binary.LittleEndian.PutUint16(intBuf[:2], uint16(val)) // Write to buffer
				params.Write(intBuf[:2])                               // Write to buffer
			case '4':
				binary.LittleEndian.PutUint32(intBuf[:4], uint32(val)) // Write to buffer
				params.Write(intBuf[:4])                               // Write to buffer
			case '8':
				binary.LittleEndian.PutUint64(intBuf[:8], uint64(val)) // Write to buffer
				params.Write(intBuf[:8])                               // Write to buffer
			}
		case 'H':
			buf, err := hex.DecodeString(payload[1:]) // Decode hex string
			if err != nil {                           // Check for errors
				return [8]byte{}, nil, err // Return found error
			}

			params.Write(buf) // Write to params
		default:
			return [8]byte{}, nil, nil // No params
		}
	}

	binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(params.Bytes()))) // Write length of function parameters to buffer

	return intBuf, params.Bytes(), nil // Return params
}

// parseContractCode gets the code of a particular payload's corresponding
// contract.
func parseContractCode(json *fastjson.Value) ([]byte, error) {
	if !json.Exists("contract_code") { // Check no value
		return nil, ErrNilField // Return nil field error
	}

	code, err := ioutil.ReadFile(string(json.GetStringBytes("contract_code"))) // Read contract code
	if err != nil {                                                            // Check for errors
		return nil, err // Return found error
	}

	return code, nil // Return contract code
}

// getValidTags gets a populated map of valid tags.
func getValidTags() map[string]bool {
	tagStrings := []string{"nop", "transfer", "stake", "contract", "batch"} // Declare valid tag strings

	tags := make(map[string]bool) // Init tags map

	for _, tagString := range tagStrings { // Iterate through tag string representations
		tags[tagString] = true // Set valid
	}

	return tags // Return tags
}

/*
	END PARSER HELPER METHODS
*/

/* END INTERNAL METHODS */
