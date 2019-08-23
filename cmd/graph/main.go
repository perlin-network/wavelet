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
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher"
	"github.com/perlin-network/noise/handshake"
	"github.com/perlin-network/noise/nat"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/api"
	"github.com/perlin-network/wavelet/internal/snappy"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

import _ "net/http/pprof"

func main() {
	pprofFlag := flag.Bool("pprof", false, "host pprof server on port 9000")
	apiPortFlag := flag.Int("api.port", 0, "api port")

	flag.Parse()

	if *pprofFlag {
		go func() {
			fmt.Println(http.ListenAndServe("localhost:9000", nil))
		}()
	}

	log.SetWriter(log.LoggerWavelet, log.NewConsoleWriter(nil, log.FilterFor(log.ModuleNode, log.ModuleSync, log.ModuleConsensus, log.ModuleMetrics)))

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))

	if flag.Arg(0) == "remote" && len(flag.Args()) > 1 {
		resolver := nat.NewPMP()

		if err := resolver.AddMapping("tcp",
			uint16(listener.Addr().(*net.TCPAddr).Port),
			uint16(listener.Addr().(*net.TCPAddr).Port),
			30*time.Minute,
		); err != nil {
			panic(err)
		}
	}

	if flag.Arg(0) == "remote" {
		resp, err := http.Get("http://myexternalip.com/raw")
		if err != nil {
			panic(err)
		}

		ip, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}

		if err := resp.Body.Close(); err != nil {
			panic(err)
		}

		addr = net.JoinHostPort(string(ip), strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
	}

	fmt.Println("Listening for peers on:", addr)

	keys, err := skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
	if err != nil {
		panic(err)
	}

	client := skademlia.NewClient(addr, keys,
		skademlia.WithC1(sys.SKademliaC1),
		skademlia.WithC2(sys.SKademliaC2),
		skademlia.WithDialOptions(grpc.WithDefaultCallOptions(grpc.UseCompressor(snappy.Name))),
	)

	client.SetCredentials(noise.NewCredentials(addr, handshake.NewECDH(), cipher.NewAEAD(), client.Protocol()))

	ledger := wavelet.NewLedger(store.NewInmem(), client)

	go func() {
		server := client.Listen()

		wavelet.RegisterWaveletServer(server, ledger.Protocol())

		if err := server.Serve(listener); err != nil {
			panic(err)
		}
	}()

	if *apiPortFlag > 0 {
		go api.New().StartHTTP(*apiPortFlag, client, ledger, keys)
	}

	if len(flag.Args()) > 0 {
		for _, addr := range flag.Args()[:] {
			if _, err := client.Dial(addr); err != nil {
				fmt.Printf("Error dialing %s: %v\n", addr, err)
			}
		}

		fmt.Println("Bootstrapped to peers:", client.Bootstrap())
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		line, _, err := reader.ReadLine()

		if err != nil {
			break
		}

		count, err := strconv.ParseUint(string(line), 10, 64)

		if err != nil {
			continue
		}

		var batch wavelet.Batch
		for i := 0; i < 40; i++ {
			if err := batch.AddNop(); err != nil {
				panic(err)
			}
		}

		for i := uint64(0); i < count; i++ {
			tx := wavelet.AttachSenderToTransaction(keys, wavelet.NewTransaction(keys, sys.TagBatch, batch.Marshal()))
			if err := ledger.AddTransaction(tx); err != nil && errors.Cause(err) != wavelet.ErrMissingParents {
				fmt.Printf("error adding tx to graph [%v]: %+v\n", err, tx)
			}
		}
	}

	select {}
}
