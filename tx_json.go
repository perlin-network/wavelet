package wavelet

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
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

	if json.Exists("fn_name") { // Check has function name
		funcName := string(json.GetStringBytes("fn_name")) // Get function name

		var funcNameLength [8]byte                                               // Initialize dedicated len buffer
		binary.LittleEndian.PutUint32(funcNameLength[:4], uint32(len(funcName))) // Write to buffer

		payload.Write(funcNameLength[:4]) // Write name length
		payload.WriteString(funcName)     // Write function name

		if json.Exists("fn_payload") { // Check has function payload
			var intBuf [8]byte // Initialize integer buffer

			params := bytes.NewBuffer(nil) // Initialize payload buffer

			for _, payloadValue := range json.GetArray("fn_payload") { // Iterate through payloads
				if !payloadValue.Exists("type") { // Check does not declare type
					return nil, ErrNilField // Return nil field error
				}

				payloadType := string(payloadValue.GetStringBytes("type")) // Get payload type

				switch payloadType { // Handle different payload types
				case "string":
					value := string(payloadValue.GetStringBytes("value")) // Get value

					binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(value))) // Write value length to buffer

					params.Write(intBuf[:4])  // Write to buffer
					params.WriteString(value) // Write value
				case "bytes":
					value := payloadValue.GetStringBytes("value") // Get value

					binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(value))) // Write value length to buffer

					params.Write(intBuf[:4]) // Write to buffer
					params.Write(value)      // Write value
				case "uint8", "uint16", "uint32", "uint64":
					value := payloadValue.GetInt64("value") // Get value

					switch payloadType { // Handle different int sizes
					case "uint8":
						params.WriteByte(byte(value)) // Write value
					case "uint16":
						binary.LittleEndian.PutUint16(intBuf[:2], uint16(value)) // Write to buffer

						params.Write(intBuf[:2]) // Write value
					case "uint32":
						binary.LittleEndian.PutUint32(intBuf[:4], uint32(value)) // Write to buffer

						params.Write(intBuf[:4]) // Write value
					case "uint64":
						binary.LittleEndian.PutUint64(intBuf[:8], uint64(value)) // Write to buffer

						params.Write(intBuf[:8]) // Write value
					}
				case "hex":
					value := string(payloadValue.GetStringBytes("value")) // Get value

					buf, err := hex.DecodeString(value) // Decode value
					if err != nil {                     // Check for errors
						return nil, err // Return found error
					}

					params.Write(buf) // Write value
				}
			}

			binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(params.Bytes()))) // Write length of function parameters to buffer

			payload.Write(intBuf[:4])     // Write length of function parameters
			payload.Write(params.Bytes()) // Write parameters
		}
	}

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

	if !json.Exists("operation") { // Check no value
		return nil, ErrNilField // Return nil field error
	}

	var operation byte // Initialize operation buffer

	operationInt := json.GetInt("operation") // Get operation code

	if byte(operationInt) >= sys.TagBatch || byte(operationInt) < 0 { // Check invalid value
		return nil, ErrInvalidOperation // Return invalid operation error
	}

	switch json.GetInt("operation") { // Handle different operations
	case 0:
		operation = sys.WithdrawStake // Set operation
	case 1:
		operation = sys.WithdrawReward // Set operation
	case 2:
		operation = sys.WithdrawReward // Set operation
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

	if !json.Exists("gas_limit") || !json.Exists("contract_code") { // Check no value
		return nil, ErrNilField // Return nil field error
	}

	decodedGasLimit := uint64(json.GetFloat64("gas_limit")) // Get uint64 gas limit value

	var gasLimit [8]byte                                        // Initialize integer buffer
	binary.LittleEndian.PutUint64(gasLimit[:], decodedGasLimit) // Write to buffer

	_, err = payload.Write(gasLimit[:]) // Write gas limit
	if err != nil {                     // Check for errors
		return nil, err // Return found error
	}

	if json.Exists("fn_payload") { // Check has function payload
		var intBuf [8]byte // Initialize integer buffer

		params := bytes.NewBuffer(nil) // Initialize payload buffer

		for _, payloadValue := range json.GetArray("fn_payload") { // Iterate through payloads
			if !payloadValue.Exists("type") { // Check does not declare type
				return nil, ErrNilField // Return nil field error
			}

			payloadType := string(payloadValue.GetStringBytes("type")) // Get payload type

			switch payloadType { // Handle different payload types
			case "string":
				value := string(payloadValue.GetStringBytes("value")) // Get value

				binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(value))) // Write value length to buffer

				params.Write(intBuf[:4])  // Write to buffer
				params.WriteString(value) // Write value
			case "bytes":
				value := payloadValue.GetStringBytes("value") // Get value

				binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(value))) // Write value length to buffer

				params.Write(intBuf[:4]) // Write to buffer
				params.Write(value)      // Write value
			case "uint8", "uint16", "uint32", "uint64":
				value := payloadValue.GetInt64("value") // Get value

				switch payloadType { // Handle different int sizes
				case "uint8":
					params.WriteByte(byte(value)) // Write value
				case "uint16":
					binary.LittleEndian.PutUint16(intBuf[:2], uint16(value)) // Write to buffer

					params.Write(intBuf[:2]) // Write value
				case "uint32":
					binary.LittleEndian.PutUint32(intBuf[:4], uint32(value)) // Write to buffer

					params.Write(intBuf[:4]) // Write value
				case "uint64":
					binary.LittleEndian.PutUint64(intBuf[:8], uint64(value)) // Write to buffer

					params.Write(intBuf[:8]) // Write value
				}
			case "hex":
				value := string(payloadValue.GetStringBytes("value")) // Get value

				buf, err := hex.DecodeString(value) // Decode value
				if err != nil {                     // Check for errors
					return nil, err // Return found error
				}

				params.Write(buf) // Write value
			}
		}

		binary.LittleEndian.PutUint32(intBuf[:4], uint32(len(params.Bytes()))) // Write length of function parameters to buffer

		payload.Write(intBuf[:4])     // Write length of function parameters
		payload.Write(params.Bytes()) // Write parameters
	}

	code, err := ioutil.ReadFile(string(json.GetStringBytes("contract_code"))) // Read contract code
	if err != nil {                                                            // Check for errors
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
