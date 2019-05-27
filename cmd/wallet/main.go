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

package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/rs/zerolog/log"
	"io/ioutil"
	"os"
	"path/filepath"
)

const GenPath = "config"

func main() {
	flagN := flag.Uint("n", 1, "Number of wallets to create.")
	flagC1 := flag.Uint("c1", 16, "S/Kademlia C1 protocol parameter.")
	flagC2 := flag.Uint("c2", 16, "S/Kademlia C2 protocol parameter.")
	flag.Parse()

	if err := os.Mkdir(GenPath, 0755); err != nil && !os.IsExist(err) {
		if os.IsPermission(err) {
			log.Fatal().Err(err).Msgf("Failed to get permission to create directory %q to store wallets in.", GenPath)
		}

		log.Fatal().Err(err).Msgf("An unknown error occured creating directory %q.", GenPath)
	}

	for i := uint(1); i <= *flagN; i++ {
		walletFilePath := filepath.Join(GenPath, fmt.Sprintf("wallet%d.txt", i))

		if buf, err := ioutil.ReadFile(walletFilePath); err == nil && len(buf) == hex.EncodedLen(edwards25519.SizePrivateKey) {
			continue
		}

		keys, err := skademlia.NewKeys(int(*flagC1), int(*flagC2))

		if err != nil {
			log.Fatal().Err(err).Msg("Failed to generate keypair.")
		}

		privateKey := keys.PrivateKey()

		privateKeyBuf := make([]byte, hex.EncodedLen(edwards25519.SizePrivateKey))

		if n := hex.Encode(privateKeyBuf[:], privateKey[:]); n != hex.EncodedLen(edwards25519.SizePrivateKey) {
			log.Fatal().Msg("An unknown error occurred marshaling a newly generated keypairs private key into hex.")
		}

		if err := ioutil.WriteFile(walletFilePath, privateKeyBuf, 0755); err != nil {
			log.Fatal().Err(err).Msg("Failed to write private key to file.")
		}

		log.Info().Str("path", walletFilePath).Msg("Generated a wallet.")
	}

	log.Info().Msg("All wallets have successfully been generated.")
}
