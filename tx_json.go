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

// Parser defines a generic JSON transaction payload parser.
type Parser struct {
	Tag string // Transaction tag
}

var (
	// ErrNoTag defines an error describing an empty TransactionTag.
	ErrNoTag = errors.New("no tag specified")

	// ErrInvalidTag defines an error describing an invalid TransactionTag.
	ErrInvalidTag = errors.New("tag is invalid")

	// ErrCouldNotParse defines an error describing the inability to parse a given payload.
	ErrCouldNotParse = errors.New("could not parse the given payload")

	// ErrNilField defines an error describing a field value equal to nil.
	ErrNilField = errors.New("field is nil")

	// ErrInvalidOperation defines an error describing an invalid operation value.
	ErrInvalidOperation = errors.New("operation is invalid")
)

/* BEGIN EXPORTED METHODS */

// NewParser initializes a new parser with the given transaction type.
func NewParser(transactionTag string) *Parser {
	return &Parser{
		Tag: transactionTag, // Set transaction tag
	} // Initialize parser
}

// ParseJSON parses the given JSON payload input.
func (parser *Parser) ParseJSON(data []byte) ([]byte, error) {
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
func (parser *Parser) parseTransfer(data []byte) ([]byte, error) {
	payload := bytes.NewBuffer(nil) // Initialize buffer

	parsedData, err := parseJSONBytes(data) // Parse data
	if err != nil {                         // Check for errors
		return nil, err // Return found error
	}

	recipient, err := parseRecipient(parsedData) // Parse recipient
	if err != nil {                              // Check for errors
		return nil, err // Return found error
	}

	_, err = payload.Write(recipient[:]) // Write recipient value
	if err != nil {                      // Check for errors
		return nil, err // Return found error
	}

	amount, err := parseAmount(parsedData) // Parse amount
	if err != nil {                        // Check for errors
		return nil, err // Return found error
	}

	_, err = payload.Write(amount[:]) // Write amount value
	if err != nil {                   // Check for errors
		return nil, err // Return found error
	}

	gasLimit, err := parseGasLimit(parsedData) // Parse gas imit
	if err != nil && err != ErrNilField {      // Check for errors
		return nil, err // Return found error
	}

	if err == nil {
		_, err = payload.Write(gasLimit[:]) // Write gas limit
		if err != nil {                     // Check for errors
			return nil, err // Return found error
		}
	}

	funcNameLength, funcName, funcParamsLength, funcParams, err := parseFunction(parsedData) // Parse function
	if err != nil && err != ErrNilField {                                                    // Check for errors
		return nil, err // Return found error
	}

	payload.Write(funcNameLength[:4])   // Write name length
	payload.WriteString(funcName)       // Write function name
	payload.Write(funcParamsLength[:4]) // Write length of function parameters
	payload.Write(funcParams)           // Write parameters

	return payload.Bytes(), nil // Return payload
}

// parseStake parses a transaction payload with the stake tag.
func (parser *Parser) parseStake(data []byte) ([]byte, error) {
	payload := bytes.NewBuffer(nil) // Initialize buffer

	parsedData, err := parseJSONBytes(data) // Parse data
	if err != nil {                         // Check for errors
		return nil, err // Return found error
	}

	operation, err := parseOperation(parsedData) // Parse operation
	if err != nil {                              // Check for errors
		return nil, err // Return found error
	}

	amount, err := parseAmount(parsedData) // Parse amount
	if err != nil {                        // Check for errors
		return nil, err // Return found error
	}

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
func (parser *Parser) parseContract(data []byte) ([]byte, error) {
	payload := bytes.NewBuffer(nil) // Initialize buffer

	parsedData, err := parseJSONBytes(data) // Parse data
	if err != nil {                         // Check for errors
		return nil, err // Return found error
	}

	gasLimit, err := parseGasLimit(parsedData) // Parse gas limit
	if err != nil {                            // Check for errors
		return nil, err // Return found error
	}

	functionPayloadLength, functionPayload, err := parseFunctionPayload(parsedData) // Parse function payload
	if err != nil {                                                                 // Check for errors
		return nil, err // Return found error
	}

	code, err := parseContractCode(parsedData) // Parse contract code
	if err != nil {                            // Check for errors
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
func (parser *Parser) parseBatch(data []byte) ([]byte, error) {
	payload := bytes.NewBuffer(nil) // Initialize buffer

	parsedData, err := parseJSONBytes(data) // Parse data
	if err != nil {                         // Check for errors
		return nil, err // Return found error
	}

	if !parsedData.Exists("payloads") { // Check no value
		return nil, ErrNilField // Return nil field error
	}

	transactions := parsedData.GetArray("payloads") // Get payloads

	var batchLength [8]byte // Initialize batch length buffer

	binary.LittleEndian.PutUint64(batchLength[:], uint64(len(transactions))) // Write batch length to buffer

	payload.Write(batchLength[:4]) // Write batch length

	for _, transaction := range transactions { // Iterate through transactions
		json, err := transaction.StringBytes() // Get JSON
		if err != nil {                        // Check for errors
			return nil, err // Return found error
		}

		txParser := NewParser(string(transaction.GetStringBytes("tag"))) // Initialize parser

		txPayload, err := txParser.ParseJSON(json) // Parse JSON
		if err != nil {                            // Check for errors
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

	switch string(json.GetStringBytes("operation")) { // Handle different operations
	case "0x00":
		return sys.WithdrawStake, nil // Return parsed
	case "0x01":
		return sys.PlaceStake, nil // Return parsed
	case "0x02":
		return sys.WithdrawReward, nil // Return parsed
	}

	return byte(0), ErrInvalidOperation // Return invalid operation error
}

// parseAmount gets the amount of PERLs sent in a given transaction.
func parseAmount(json *fastjson.Value) ([8]byte, error) {
	if !json.Exists("amount") { // Check no value
		return [8]byte{}, ErrNilField // Return nil field error
	}

	amount := uint64(json.GetFloat64("amount")) // Get amount value

	var intBuf [8]byte                               // Initialize integer buffer
	binary.LittleEndian.PutUint64(intBuf[:], amount) // Write to buffer

	return intBuf, nil // Return buffer contents
}

// parseGasLimit gets the gas limit of a particular transaction.
func parseGasLimit(json *fastjson.Value) ([8]byte, error) {
	if !json.Exists("gas_limit") { // Check no value
		return [8]byte{}, ErrNilField // Return nil field error
	}

	gasLimit := uint64(json.GetFloat64("gas_limit")) // Get uint64 gas limit value

	var intBuf [8]byte                                 // Initialize integer buffer
	binary.LittleEndian.PutUint64(intBuf[:], gasLimit) // Write to buffer

	return intBuf, nil // Return buffer contents
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

// parseRecipient gets the recipient of the transaction from the JSON map,
// and performs address length safety checks immediately after.
func parseRecipient(json *fastjson.Value) ([]byte, error) {
	if !json.Exists("recipient") { // Check no value
		return nil, ErrNilField // Return nil field error
	}

	recipient, err := hex.DecodeString(string(json.GetStringBytes("recipient"))) // Decode recipient hex string
	if err != nil {                                                              // Check for errors
		return nil, err // Return found error
	}

	if len(recipient) != SizeAccountID {
		return nil, err // Return invalid recipient ID error
	}

	var recipientID AccountID       // Initialize ID buffer
	copy(recipientID[:], recipient) // Copy address to buffer

	return recipient, nil // Return recipient
}

// parseJSONBytes parses a given raw JSON input, and converts such an input
// to a fastjson value pointer.
func parseJSONBytes(data []byte) (*fastjson.Value, error) {
	var p fastjson.Parser // Initialize parser

	v, err := p.Parse(string(data)) // Parse json
	if err != nil {                 // Check for errors
		return nil, err // Return found error
	}

	return v, nil // Return parsed
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
