// Package parser implements a JSON payload parser for each of the
// transaction tags.
package parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"

	"github.com/perlin-network/wavelet"
	"github.com/pkg/errors"
)

// Parser defines a generic JSON transaction payload parser.
type Parser struct {
	TransactionTag string // Transaction tag

	Ledger *wavelet.Ledger // Ledger

	PublicKey edwards25519.PublicKey // Public key
}

var (
	// ErrNoTag defines an error describing an empty TransactionTag.
	ErrNoTag = errors.New("no tag specified")

	// ErrInvalidTag defines an error describing an invalid TransactionTag.
	ErrInvalidTag = errors.New("tag is invalid")

	// ErrCouldNotParse defines an error describing the inability to parse a given payload.
	ErrCouldNotParse = errors.New("could not parse the given payload")

	// ErrInvalidRecipientID defines an error describing an account ID of invalid length.
	ErrInvalidRecipientID = errors.New("invalid account ID specified")

	// ErrNilField defines an error describing a field value equal to nil.
	ErrNilField = errors.New("field is nil")
)

/* BEGIN EXPORTED METHODS */

// NewParser initializes a new parser with the given transaction type.
func NewParser(transactionTag string) *Parser {
	return &Parser{
		TransactionTag: transactionTag, // Set transaction tag
		Ledger: ledger, // Set ledger
	} // Initialize parser
}

// ParseJSON parses the given JSON payload input.
func (parser *Parser) ParseJSON(data []byte) ([]byte, error) {
	if parser.TransactionTag == "" { // Check no transaction tag
		return nil, ErrNoTag // Return no tag error
	}

	if !getValidTags()[parser.TransactionTag] { // Check invalid
		return nil, ErrInvalidTag // Return error
	}

	parsedJSON, err := parseJSONBytes(data) // Parse JSON
	if err != nil {                         // Check for errors
		return nil, err // Return found error
	}

	switch parser.TransactionTag { // Handle different tag types
	case "nop":
		return nil, nil // Nothing to do!
	case "transfer":
		return parser.parseTransfer(parsedJSON) // Parse
	case "stake":
		return parser.parseStake(parsedJSON) // Parse
	case "contract":
		return parser.parseContract(parsedJSON) // Parse
	case "batch":
		return parser.parseBatch(parsedJSON) // Parse
	}

	return nil, ErrCouldNotParse // Return error (shouldn't ever get here)
}

/* END EXPORTED METHODS */

/* BEGIN INTERNAL METHODS */

// parseTransfer parses a transaction payload with the transfer tag.
func (parser *Parser) parseTransfer(json map[string]interface{}) ([]byte, error) {
	payload := bytes.NewBuffer(nil) // Initialize buffer

	recipient, recipientID, err := parseRecipient(json) // Parse recipient
	if err != nil {                        // Check for errors
		return nil, err // Return found error
	}

	_, err = payload.Write(recipient[:]) // Write recipient value

	if err != nil { // Check for errors
		return nil, err // Return found error
	}

	amount, err := parseAmount(json) // Parse amount
	if err != nil { // Check for errors
		return nil, err // Return found error
	}

	_, err = payload.Write(amount[:]) // Write amount value
	if err != nil { // Check for errors
		return nil, err // Return found error
	}

	_, codeAvailable := wavelet.ReadAccountContractCode(parser.Ledger.Snapshot(), recipientID) // Check code is available
}

// parseStake parses a transaction payload with the stake tag.
func (parser *Parser) parseStake(json map[string]interface{}) ([]byte, error) {

}

// parseContract parses a transaction payload with the contract tag.
func (parser *Parser) parseContract(json map[string]interface{}) ([]byte, error) {

}

// parseBatch parses a transaction payload with the batch tag.
func (parser *Parser) parseBatch(json map[string]interface{}) ([]byte, error) {

}

// parseAmount gets the amount of PERLs sent in a given transaction.
func parseAmount(json map[string]interface{}) ([8]byte, error) {
	if json["amount"] == nil { // Check no value
		return [8]byte{}, ErrNilField // Return nil field error
	}

	amount := uint64(json["amount"].(float64)) // Get uint64 amount value

	var intBuf [8]byte                               // Initialize integer buffer
	binary.LittleEndian.PutUint64(intBuf[:], amount) // Write to buffer

	return intBuf, nil // Return buffer contents
}

// parseRecipient gets the recipient of the transaction from the JSON map,
// and performs address length safety checks immediately after.
func parseRecipient(json map[string]interface{}) ([]byte, wavelet.AccountID, error) {
	recipient, err := hex.DecodeString(json["recipient"].(string))
	if err != nil { // Check for errors
		return nil, wavelet.AccountID{}, err // Return found error
	}

	if len(recipient) != wavelet.SizeAccountID {
		return nil, wavelet.AccountID{}, err // Return invalid recipient ID error
	}

	var recipientID wavelet.AccountID // Initialize ID buffer
	copy(recipientID[:], recipient) // Copy address to buffer

	return recipient, recipientID, nil // Return recipient
}

// parseJSONBytes parses a given raw JSON input, and converts such an input
// to a string -> interface map.
func parseJSONBytes(data []byte) (map[string]interface{}, error) {
	var parsedPayloadJSON map[string]interface{} // Initialize parsed buffer

	err := json.Unmarshal(data, &parsedPayloadJSON) // Unmarshal JSON
	if err != nil {                                 // Check for errors
		return nil, err // Return error
	}

	return parsedPayloadJSON, nil // Return parsed
}

// getValidTags gets a populated map of valid tags.
func getValidTags() map[string]bool {
	var tagStrings = []string{"nop", "transfer", "stake", "contract", "batch"} // Declare valid tag strings

	tags := make(map[string]bool) // Init tags map

	for _, tagString := range tagStrings { // Iterate through tag string representations
		tags[tagString] = true // Set valid
	}

	return tags // Return tags
}

/* END INTERNAL METHODS */
