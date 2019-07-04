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

// PayloadFieldNames represents all valid JSON payload field names.
var PayloadFieldNames = []string{"recipient", "amount", "gas_limit", "fn_name", "fn_payload", "operation", "contract_code", "payloads"}

/* BEGIN EXPORTED METHODS */

// ParseJSON parses the given JSON payload input.
func ParseJSON(data []byte, tag string) ([]byte, error) {
	if tag == "" { // Check no transaction tag
		return nil, ErrNoTag // Return no tag error
	}

	if err := fastjson.ValidateBytes(data); err != nil { // Check not valid JSON
		return nil, err // Return found error
	}

	if intTag := sys.TagConversion[tag]; intTag >= sys.TagBatch || intTag < 0 { // Check invalid tag value
		return nil, ErrInvalidTag // Return invalid tag error
	}

	switch tag { // Handle different tag types
	case "nop":
		return nil, nil // Nothing to do!
	case "transfer":
		return parseTransfer(data) // Parse
	case "stake":
		return parseStake(data) // Parse
	case "contract":
		return parseContract(data) // Parse
	case "batch":
		return parseBatch(data) // Parse
	}

	return nil, ErrCouldNotParse // Return error (shouldn't ever get here)
}

/* END EXPORTED METHODS */

/* BEGIN INTERNAL METHODS */

/*
	BEGIN TAG HANDLERS
*/

// parseTransfer parses a transaction payload with the transfer tag.
func parseTransfer(data []byte) ([]byte, error) {
	payload := bytes.NewBuffer(nil) // Initialize buffer

	var p fastjson.Parser // Initialize parser

	json, err := p.Parse(string(data)) // Parse json
	if err != nil {                    // Check for errors
		return nil, err // Return found error
	}

	if !json.Exists("recipient") || !json.Exists("amount") { // Check no value
		return nil, ErrNilField // Return nil field error
	}

	decodedRecipient, err := hex.DecodeString(string(json.GetStringBytes(PayloadFieldNames[0]))) // Decode recipient hex string
	if err != nil {                                                                              // Check for errors
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

	decodedAmount := uint64(json.GetUint64(PayloadFieldNames[1])) // Get amount value

	var amount [8]byte                                      // Initialize integer buffer
	binary.LittleEndian.PutUint64(amount[:], decodedAmount) // Write to buffer

	_, err = payload.Write(amount[:]) // Write amount value
	if err != nil {                   // Check for errors
		return nil, err // Return found error
	}

	if json.Exists(PayloadFieldNames[2]) { // Check has gas limit value
		decodedGasLimit := uint64(json.GetFloat64(PayloadFieldNames[2])) // Get uint64 gas limit value

		var gasLimit [8]byte                                        // Initialize integer buffer
		binary.LittleEndian.PutUint64(gasLimit[:], decodedGasLimit) // Write to buffer

		_, err = payload.Write(gasLimit[:]) // Write gas limit
		if err != nil {                     // Check for errors
			return nil, err // Return found error
		}
	}

	if json.Exists(PayloadFieldNames[3]) { // Check has function name
		funcName := string(json.GetStringBytes(PayloadFieldNames[3])) // Get function name

		var funcNameLength [8]byte                                               // Initialize dedicated len buffer
		binary.LittleEndian.PutUint32(funcNameLength[:4], uint32(len(funcName))) // Write to buffer

		payload.Write(funcNameLength[:4]) // Write name length
		payload.WriteString(funcName)     // Write function name

		if json.Exists(PayloadFieldNames[4]) { // Check has function payload
			var intBuf [8]byte // Initialize integer buffer

			params := bytes.NewBuffer(nil) // Initialize payload buffer

			for _, payloadValue := range json.GetArray(PayloadFieldNames[4]) { // Iterate through payloads
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
func parseStake(data []byte) ([]byte, error) {
	payload := bytes.NewBuffer(nil) // Initialize buffer

	var p fastjson.Parser // Initialize parser

	json, err := p.Parse(string(data)) // Parse json
	if err != nil {                    // Check for errors
		return nil, err // Return found error
	}

	if !json.Exists(PayloadFieldNames[5]) { // Check no value
		return nil, ErrNilField // Return nil field error
	}

	var operation byte // Initialize operation buffer

	operationInt := json.GetInt(PayloadFieldNames[5]) // Get operation code

	if sys.Tag(operationInt) >= sys.TagBatch || sys.Tag(operationInt) < 0 { // Check invalid value
		return nil, ErrInvalidOperation // Return invalid operation error
	}

	switch json.GetInt(PayloadFieldNames[5]) { // Handle different operations
	case 0:
		operation = sys.WithdrawStake // Set operation
	case 1:
		operation = sys.WithdrawReward // Set operation
	case 2:
		operation = sys.WithdrawReward // Set operation
	}

	decodedAmount := uint64(json.GetFloat64(PayloadFieldNames[1])) // Get amount value

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
func parseContract(data []byte) ([]byte, error) {
	payload := bytes.NewBuffer(nil) // Initialize buffer

	var p fastjson.Parser // Initialize parser

	json, err := p.Parse(string(data)) // Parse json
	if err != nil {                    // Check for errors
		return nil, err // Return found error
	}

	if !json.Exists(PayloadFieldNames[2]) || !json.Exists("contract_code") { // Check no value
		return nil, ErrNilField // Return nil field error
	}

	decodedGasLimit := uint64(json.GetFloat64(PayloadFieldNames[2])) // Get uint64 gas limit value

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

	code, err := ioutil.ReadFile(string(json.GetStringBytes(PayloadFieldNames[6]))) // Read contract code
	if err != nil {                                                                 // Check for errors
		return nil, err // Return found error
	}

	_, err = payload.Write(code) // Write contract code to buffer
	if err != nil {              // Check for errors
		return nil, err // Return found error
	}

	return payload.Bytes(), nil // Return payload
}

// parseBatch parses a transaction payload with the batch tag.
func parseBatch(data []byte) ([]byte, error) {
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

		txPayload, err := ParseJSON(txJSON, string(tx.GetStringBytes("tag"))) // Parse JSON
		if err != nil {                                                       // Check for errors
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

/* END INTERNAL METHODS */
