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
	"fmt"
	wasm "github.com/perlin-network/life/wasm-validation"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/log"
	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultGenesis = `
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

// performInception loads genesis data from the a directory expected to exist at the birth of any node in this ledgers network.
//
// An account state may be specified within the genesis directory in the form of a [account address].json file with key-value pairs.
//
// A smart contract may be specified within the genesis directory in the form of a [contract address].wasm file with accompanying [contract address].[page index].dmp files representing the contracts memory pages.
//
// If the directory is nil, it'll load the hardcoded default genesis.
func performInception(tree *avl.Tree, dir *string) Round {
	logger := log.Node()

	var err error
	if dir != nil {
		var absDir string
		absDir, err = filepath.Abs(*dir)

		if err != nil {
			logger.Fatal().Err(err).Msgf("failed to determine absolute path of genesis directory %s", *dir)
		}

		err = loadGenesisFromDir(tree, absDir)

	} else {
		err = loadGenesisFromJSON(tree, []byte(defaultGenesis))
	}

	if err != nil {
		logger.Fatal().Err(err).Msg("genesis")
	}

	tx := Transaction{}
	tx.rehash()

	return NewRound(0, tree.Checksum(), 0, Transaction{}, tx)
}

func loadGenesisFromJSON(tree *avl.Tree, json []byte) error {
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

		err = loadAccounts(tree, id, val)
	})

	return nil
}

func loadGenesisFromDir(tree *avl.Tree, dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return errors.Wrapf(err, "directory %s does not exist", dir)
	}

	accounts := make(map[AccountID]*fastjson.Value)
	contracts := make(map[TransactionID][]byte)
	contractPages := make(map[TransactionID][][]byte)

	var p fastjson.Parser

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			// Nothing to do for directories.
			return nil
		}

		ext := filepath.Ext(path)
		filename := strings.TrimSuffix(filepath.Base(path), ext)

		if ext == ".json" {
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

			b, err := ioutil.ReadFile(path)
			if err != nil {
				return errors.Wrapf(err, "failed to read file %s", path)
			}

			val, err := p.ParseBytes(b)
			if err != nil {
				return errors.Wrapf(err, "failed to parse file %s", path)
			}

			accounts[id] = val

			return nil

		} else if ext == ".wasm" {
			var id TransactionID
			// filename is the id
			if n, err := hex.Decode(id[:], []byte(filename)); n != cap(id) || err != nil {
				if err != nil {
					return errors.Wrapf(err, "filename of %s has invalid contract ID", path)
				}
				return errors.Errorf("filename of %s has invalid contract ID", path)
			}

			// Check for duplicate
			if _, exist := contracts[id]; exist {
				return nil
			}

			code, err := ioutil.ReadFile(path)
			if err != nil {
				return errors.Wrapf(err, "failed to read file %s", path)
			}

			if err := wasm.GetValidator().ValidateWasm(code); err != nil {
				return errors.Wrapf(err, "file %s has invalid wasm", path)
			}

			contracts[id] = code

		} else if ext == ".dmp" {
			idxExt := filepath.Ext(filename)

			// Must at least contain a character. e.g. .1
			if len(idxExt) < 2 {
				return errors.Errorf("filename of %s has invalid name, expected index after contract address", path)
			}

			idx, err := strconv.Atoi(idxExt[1:])
			if err != nil {
				return errors.Wrapf(err, "filename of %s has invalid index, expected an unsigned integer", path)
			}
			if idx < 0 {
				return errors.Errorf("filename of %s has invalid index, expected an unsigned integer", path)
			}

			var id TransactionID
			// filename is the id
			if n, err := hex.Decode(id[:], []byte(strings.TrimSuffix(filename, idxExt))); n != cap(id) || err != nil {
				if err != nil {
					return errors.Wrapf(err, "filename of %s has invalid contract ID", path)
				}
				return errors.Errorf("filename of %s has invalid contract ID", path)
			}

			b, err := ioutil.ReadFile(path)
			if err != nil {
				return errors.Wrapf(err, "failed to read file %s", path)
			}

			// Check page size
			if size := len(b); size != 0 && size != PageSize {
				return errors.Errorf("contract page file %s has invalid page size %d. must be 0 or %d", path, size, PageSize)
			}

			pages := contractPages[id]

			// Grow the slice according to the page idx
			// The key of the slice is the page idx.
			if i := idx - cap(pages) + 1; i > 0 {
				newPages := make([][]byte, len(pages)+i)
				copy(newPages, pages)

				contractPages[id] = newPages
				pages = newPages
			}

			// Check if the page already exist
			if pages[idx] != nil {
				return nil
			}

			pages[idx] = b
		}

		return nil
	})

	if err != nil {
		return err
	}

	for id, account := range accounts {
		_ = loadAccounts(tree, id, account)
	}

	for id, contract := range contracts {
		WriteAccountContractCode(tree, id, contract)
	}

	for id, pages := range contractPages {
		_ = loadContractPages(tree, id, pages)
	}

	return nil
}

func loadAccounts(tree *avl.Tree, id AccountID, val *fastjson.Value) error {
	fields, err := val.Object()
	if err != nil {
		return err
	}

	var balance, stake, reward uint64

	fields.Visit(func(key []byte, v *fastjson.Value) {
		if err != nil {
			return
		}

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

			WriteAccountStake(tree, id, uint64(stake))
		case "reward":
			reward, err = v.Uint64()

			if err != nil {
				err = errors.Wrapf(err, "failed to cast type for key %q", key)
				return
			}

			WriteAccountReward(tree, id, uint64(reward))
		}
	})

	if err == nil {
		WriteAccountsLen(tree, ReadAccountsLen(tree)+1)
		WriteAccountNonce(tree, id, 1)
	}

	return err
}

func loadContractPages(tree *avl.Tree, id TransactionID, pages [][]byte) error {
	// Verify all of the pages have the correct size and not nil.
	// If even one of the page has invalid size or nil, we consider the pages to be invalid and return an error.
	for i := range pages {
		if pages[i] == nil {
			return fmt.Errorf("contract %x is missing page index %d", id, i)
		}

		if s := len(pages[i]); s != 0 && s != PageSize {
			return fmt.Errorf("contract %x page index %d has invalid size %d, expected page size 0 or %d", id, i, s, PageSize)
		}
	}

	for i := range pages {
		WriteAccountContractPage(tree, id, uint64(i), pages[i])
	}

	WriteAccountContractNumPages(tree, id, uint64(len(pages)))

	return nil
}
