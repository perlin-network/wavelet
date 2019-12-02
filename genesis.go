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

package wavelet

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	wasm "github.com/perlin-network/life/wasm-validation"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fastjson"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const testingGenesis = `
{
  "400056ee68a7cc2695222df05ea76875bc27ec6e61e8e62317c336157019c405": {
    "balance": 10000000000000000000,
    "reward": 5000000
  },
  "696937c2c8df35dba0169de72990b80761e51dd9e2411fa1fce147f68ade830a": {
    "balance": 10000000000000000000
  },
  "f03bb6f98c4dfd31f3d448c7ec79fa3eaa92250112ada43471812f4b1ace6467": {
    "balance": 10000000000000000000
  }
}
`

const testnetGenesis = `
{
    "0f569c84d434fb0ca682c733176f7c0c2d853fce04d95ae131d2f9b4124d93d8": {
        "balance": 10000000000000000000
    }
}
`

var defaultGenesis = testingGenesis

func SetGenesisByNetwork(name string) error {
	switch name {
	case "testnet":
		defaultGenesis = testnetGenesis
	case "testing":
		defaultGenesis = testingGenesis
	default:
		return fmt.Errorf("invalid network: %s", name)
	}

	return nil
}

// performInception restore genesis data from the a directory or JSON contents expected to exist at the birth of any
// node in this ledgers network.
//
// An account state may be specified within the genesis directory in the form of a [account address].json file with key
// value pairs.
//
// A smart contract may be specified within the genesis directory in the form of a [contract address].wasm file with
// accompanying [contract address].[page index].dmp files representing the contracts memory pages.
//
// The AccountsLen in the restored tree may not match with the original tree.
//
// If the genesis is nil, restore from the hardcoded default genesis.
func performInception(tree *avl.Tree, genesis *string) Block {
	logger := log.Node()

	var err error

	if genesis != nil {
		isJSON := func(str string) bool {
			var js json.RawMessage
			return json.Unmarshal([]byte(str), &js) == nil
		}

		if isJSON(*genesis) {
			err = restoreFromJSON(tree, []byte(*genesis))
		} else {
			err = restoreFromDir(tree, *genesis)
		}
	} else {
		err = restoreFromJSON(tree, []byte(defaultGenesis))
	}

	if err != nil {
		logger.Fatal().Err(err).Msg("genesis")
	}

	block, err := NewBlock(0, tree.Checksum())
	if err != nil {
		logger.Fatal().Err(err).Msg("genesis block")
	}

	return block
}

func restoreFromJSON(tree *avl.Tree, json []byte) error {
	var p fastjson.Parser

	parsed, err := p.ParseBytes(json)
	if err != nil {
		return errors.Wrapf(err, "failed to parse JSON")
	}

	accounts, err := parsed.Object()
	if err != nil {
		return errors.Wrapf(err, "failed to parse JSON Object")
	}

	set := make(map[AccountID]struct{}) // Ensure that there are no duplicate account entries in the JSON.

	accounts.Visit(func(key []byte, val *fastjson.Value) {
		if err != nil {
			return
		}

		var id AccountID
		var n int

		n, err = hex.Decode(id[:], key)

		if n != cap(id) && err == nil {
			err = errors.Errorf("got an invalid account ID: %x", key)
			return
		}

		if err != nil {
			err = errors.Wrapf(err, "got an invalid account ID: %x", key)
			return
		}

		if _, exists := set[id]; exists {
			err = errors.Errorf("found duplicate entries for account ID %x in genesis file", id)
			return
		}

		set[id] = struct{}{}

		err = restoreAccount(tree, id, val)
	})

	return nil
}

func restoreContractGlobals(tree *avl.Tree, id TransactionID, path string) error {
	globalsBuf := make([]byte, sys.ContractMaxGlobals)

	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "failed to open contract globals file %s", path)
	}

	n, err := f.Read(globalsBuf)
	if err != nil && err != io.EOF {
		return errors.Wrapf(err, "failed to read contract globals %s", path)
	}

	if err == nil && n == cap(globalsBuf) {
		return errors.Wrapf(err, "failed to read contract globals %s, buffer is not enough", path)
	}

	// This assumes WriteAccountContractGlobals will make a copy of the bytes.
	WriteAccountContractGlobals(tree, id, globalsBuf[:n])

	return nil
}

func restoreFromDir(tree *avl.Tree, dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return errors.Wrapf(err, "directory %s does not exist", dir)
	}

	accounts := make(map[AccountID]struct{})
	contractPageFiles := make(map[TransactionID][]string)

	// This is should only used to check for duplicates.
	contractsExist := make(map[TransactionID]struct{})

	pool := &bytebufferpool.Pool{}

	var (
		p         fastjson.Parser
		walletBuf [512]byte
		contracts []TransactionID
	)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			// Nothing to do for directories.
			return nil
		}

		ext := filepath.Ext(path)
		switch ext {
		case ".json":
			return restoreFromDirJSON(tree, walletBuf, p, accounts, path)

		case ".wasm":
			return restoreFromDirWASM(tree, pool, &contracts, contractsExist, path)

		case ".dmp":
			return restoreFromDirDMP(tree, contractPageFiles, path)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Each contract must have Code.
	for _, id := range contracts {
		if _, exist := ReadAccountContractCode(tree, id); !exist {
			return errors.Errorf("contract %x is missing code", id)
		}
	}

	return restoreContractPages(tree, contracts, contractPageFiles)
}

func restoreFromDirJSON(
	tree *avl.Tree, walletBuf [512]byte, p fastjson.Parser, accounts map[AccountID]struct{}, path string,
) error {
	filename := strings.TrimSuffix(filepath.Base(path), ".json")

	var id AccountID
	// filename is the id
	if n, err := hex.Decode(id[:], []byte(filename)); n != cap(id) || err != nil {
		if err != nil {
			return errors.Wrapf(err, "filename of %s has invalid account ID", path)
		}

		return errors.Errorf("filename of %s has invalid account ID", path)
	}

	// Check for duplicate
	if _, exist := accounts[id]; exist {
		return nil
	}

	accounts[id] = struct{}{}

	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "failed to open file %s", path)
	}

	n, err := f.Read(walletBuf[:])
	if err != nil && err != io.EOF {
		return errors.Wrapf(err, "failed to read wallet %s", path)
	}

	if err == nil && n == cap(walletBuf) {
		return errors.Wrapf(err, "failed to read wallet %s, buffer is not enough", path)
	}

	val, err := p.ParseBytes(walletBuf[:n])
	if err != nil {
		return errors.Wrapf(err, "failed to parse file %s", path)
	}

	return restoreAccount(tree, id, val)
}

func restoreFromDirWASM(
	tree *avl.Tree,
	pool *bytebufferpool.Pool,
	contracts *[]TransactionID,
	contractsExist map[AccountID]struct{},
	path string,
) error {
	filename := strings.TrimSuffix(filepath.Base(path), ".wasm")

	var id TransactionID
	// filename is the id
	if n, err := hex.Decode(id[:], []byte(filename)); n != cap(id) || err != nil {
		if err != nil {
			return errors.Wrapf(err, "filename of %s has invalid contract ID", path)
		}

		return errors.Errorf("filename of %s has invalid contract ID", path)
	}

	// Check for duplicate
	if _, exist := contractsExist[id]; exist {
		return nil
	}

	contractsExist[id] = struct{}{}

	// Maybe instead of doing this, the function should return the contract id
	// and have the caller append it to the contracts list
	*contracts = append(*contracts, id)

	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "failed to open file %s", path)
	}

	buf := pool.Get()
	defer pool.Put(buf)

	_, err = buf.ReadFrom(f)
	if err != nil {
		return errors.Wrapf(err, "failed to open file %s", path)
	}

	if err := wasm.GetValidator().ValidateWasm(buf.Bytes()); err != nil {
		return errors.Wrapf(err, "file %s has invalid wasm", path)
	}

	WriteAccountContractCode(tree, id, buf.Bytes())

	return nil
}

func restoreFromDirDMP(tree *avl.Tree, contractPageFiles map[TransactionID][]string, path string) error {
	filename := strings.TrimSuffix(filepath.Base(path), ".dmp")
	secondExt := filepath.Ext(filename)

	var id TransactionID
	// filename is the id
	if n, err := hex.Decode(id[:], []byte(strings.TrimSuffix(filename, secondExt))); n != cap(id) || err != nil {
		if err != nil {
			return errors.Wrapf(err, "filename of %s has invalid contract ID", path)
		}

		return errors.Errorf("filename of %s has invalid contract ID", path)
	}

	// Restore contract globals.
	if secondExt == ".globals" {
		return restoreContractGlobals(tree, id, path)
	}

	// For contract pages, get all the contract pages first and group them by contract ID.
	// This is to make sure, for each contract the pages are complete and there are no missing pages.

	// Must at least contain a character. e.g. .1
	if len(secondExt) < 2 {
		return errors.Errorf("filename of %s has invalid name, expected index after contract address", path)
	}

	idx, err := strconv.Atoi(secondExt[1:])
	if err != nil {
		return errors.Wrapf(err, "filename of %s has invalid index, expected an unsigned integer", path)
	}

	if idx < 0 {
		return errors.Errorf("filename of %s has invalid index, expected an unsigned integer", path)
	}

	files := contractPageFiles[id]

	// Grow the slice according to the page idx
	// The key of the slice is the page idx.
	if i := idx - cap(files) + 1; i > 0 {
		newFiles := make([]string, len(files)+i)
		copy(newFiles, files)

		contractPageFiles[id] = newFiles
		files = newFiles
	}

	// Check if the page already exist
	if len(files[idx]) > 0 {
		return nil
	}

	files[idx] = path

	return nil
}

func restoreAccount(tree *avl.Tree, id AccountID, val *fastjson.Value) error {
	fields, err := val.Object()
	if err != nil {
		return err
	}

	var (
		balance, stake, reward, gasBalance uint64
		isContract                         bool
	)

	fields.Visit(func(key []byte, v *fastjson.Value) {
		switch string(key) {
		case "balance":
			balance, err = v.Uint64()
			if err != nil {
				err = errors.Wrapf(err, "failed to cast type for key %q", key)
				return
			}

			WriteAccountBalance(tree, id, balance)
		case "stake":
			stake, err = v.Uint64()
			if err != nil {
				err = errors.Wrapf(err, "failed to cast type for key %q", key)
				return
			}

			WriteAccountStake(tree, id, stake)
		case "reward":
			reward, err = v.Uint64()
			if err != nil {
				err = errors.Wrapf(err, "failed to cast type for key %q", key)
				return
			}

			WriteAccountReward(tree, id, reward)
		case "gas_balance":
			gasBalance, err = v.Uint64()
			if err != nil {
				err = errors.Wrapf(err, "failed to cast type for key %q", key)
				return
			}

			WriteAccountContractGasBalance(tree, id, gasBalance)
		case "is_contract":
			isContract, err = v.Bool()
			if err != nil {
				err = errors.Wrapf(err, "failed to cast type for key %q", key)
				return
			}
		}
	})

	if err != nil {
		return err
	}

	if !isContract {
		WriteAccountsLen(tree, ReadAccountsLen(tree)+1)
	}

	return nil
}

// For each contract, read all the pages into buffers first, and check for following conditions:
// 1. the file is successfully read into buffer,
// 2. the file size is either 0 or 65536,
//
// Considered success only if all the conditions are true, otherwise returns an error.
func restoreContractPages(
	tree *avl.Tree, contracts []TransactionID, contractPageFiles map[TransactionID][]string,
) error {
	pool := bytebufferpool.Pool{}

	for _, id := range contracts {
		files := contractPageFiles[id]

		WriteAccountContractNumPages(tree, id, uint64(len(files)))

		for i := range files {
			file := files[i]

			if len(file) == 0 {
				return errors.Errorf("contract %x is missing page index %d", id, i)
			}

			buf := pool.Get()

			f, err := os.Open(file)
			if err != nil {
				return errors.Wrapf(err, "failed to open contract page file %s", file)
			}

			n, err := buf.ReadFrom(f)
			if err != nil {
				return errors.Wrapf(err, "failed to read contract page file %s", file)
			}

			// Check page size
			if n != 0 && n != PageSize {
				return errors.Errorf(
					"contract page file %s has invalid page size %d. must be 0 or %d", file, n, PageSize,
				)
			}

			if buf.Len() == 0 {
				continue
			}

			// This assumes WriteAccountContractPage will make a copy of the bytes.
			WriteAccountContractPage(tree, id, uint64(i), buf.Bytes())

			pool.Put(buf)
		}
	}

	return nil
}

// Dump the ledger states from the tree into a directory.
// Existing files will be truncate.
//
// dir is the directory path which will be used to dump the states.
//
// isDumpContract is a flag which if true, the dump will include all the contracts
//
// useContractFolder is a flag which if true, each contract dump has it's own folder.
// The folder name will be hex-encoded of the contract ID.
func Dump(tree *avl.Tree, dir string, isDumpContract bool, useContractFolder bool) error {
	// Make sure the directory exists
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.Wrapf(err, "failed to create directory %s", dir)
	}

	type account struct {
		balance *uint64
		stake   *uint64
		reward  *uint64

		isContract bool
		gasBalance *uint64
	}

	var (
		filePerm os.FileMode = 0644
		err      error
	)

	accounts := make(map[AccountID]*account)

	tree.IteratePrefix(keyAccounts[:], func(key, value []byte) {
		var prefix [1]byte
		copy(prefix[:], key[1:])

		// Filter by prefixes relevant to wallet and contract code.
		if prefix != keyAccountBalance &&
			prefix != keyAccountStake &&
			prefix != keyAccountReward &&
			prefix != keyAccountContractCode {
			return
		}

		var id AccountID
		copy(id[:], key[2:])

		if _, exist := accounts[id]; exist {
			return
		}
		_, isContract := ReadAccountContractCode(tree, id)

		acc := &account{
			isContract: isContract,
		}
		accounts[id] = acc

		if !isDumpContract && acc.isContract {
			return
		}

		if balance, exist := ReadAccountBalance(tree, id); exist {
			acc.balance = &balance
		}

		if stake, exist := ReadAccountStake(tree, id); exist {
			acc.stake = &stake
		}

		if reward, exist := ReadAccountReward(tree, id); exist {
			acc.reward = &reward
		}

		if gasBalance, exist := ReadAccountContractGasBalance(tree, id); exist {
			acc.gasBalance = &gasBalance
		}

		var folder = dir

		if useContractFolder {
			folder = filepath.Join(dir, fmt.Sprintf("%x", id))

			// Make sure the folder exists
			if ferr := os.MkdirAll(folder, 0700); ferr != nil {
				err = errors.Wrapf(ferr, "failed to create directory %s", dir)
				return
			}
		}

		err = dumpContract(tree, id, folder)
	})

	if err != nil {
		return err
	}

	arena := &fastjson.Arena{}
	data := make([]byte, 0, 512)

	for id, v := range accounts {
		if !isDumpContract && v.isContract {
			continue
		}

		o := arena.NewObject()

		if v.isContract {
			o.Set("is_contract", arena.NewTrue())
		} else {
			o.Set("is_contract", arena.NewFalse())
		}

		if v.gasBalance != nil {
			o.Set("gas_balance", arena.NewNumberString(strconv.FormatUint(*v.gasBalance, 10)))
		}

		if v.balance != nil {
			o.Set("balance", arena.NewNumberString(strconv.FormatUint(*v.balance, 10)))
		}

		if v.stake != nil {
			o.Set("stake", arena.NewNumberString(strconv.FormatUint(*v.stake, 10)))
		}

		if v.reward != nil {
			o.Set("reward", arena.NewNumberString(strconv.FormatUint(*v.reward, 10)))
		}

		data = o.MarshalTo(data)
		filename := fmt.Sprintf("%x.json", id)

		err := ioutil.WriteFile(filepath.Join(dir, filename), data, filePerm)
		if err != nil {
			return errors.Wrapf(err, "failed to write %s", filename)
		}

		data = data[:0]

		arena.Reset()
	}

	return nil
}

func dumpContract(tree *avl.Tree, id AccountID, dir string) error {
	// Write contract globals.
	if globals, ok := ReadAccountContractGlobals(tree, id); ok {
		globalsFilename := fmt.Sprintf("%x.globals.dmp", id)

		err := ioutil.WriteFile(filepath.Join(dir, globalsFilename), globals, 0644)
		if err != nil {
			return errors.Wrapf(err, "failed to write globals %s", globalsFilename)
		}
	}

	// Write contract code.
	if code, ok := ReadAccountContractCode(tree, id); ok {
		wasmFilename := fmt.Sprintf("%x.wasm", id)

		err := ioutil.WriteFile(filepath.Join(dir, wasmFilename), code, 0644)
		if err != nil {
			return errors.Wrapf(err, "failed to write wasm %s", wasmFilename)
		}
	}

	// Write contract pages.
	if numPages, ok := ReadAccountContractNumPages(tree, id); ok && numPages > 0 {
		for i := uint64(0); i < numPages; i++ {
			pageFilename := fmt.Sprintf("%x.%d.dmp", id, i)

			page, _ := ReadAccountContractPage(tree, id, i)

			err := ioutil.WriteFile(filepath.Join(dir, pageFilename), page, 0644)
			if err != nil {
				return errors.Wrapf(err, "failed to write page %s", pageFilename)
			}
		}
	}

	return nil
}
