package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher"
	"github.com/perlin-network/noise/handshake"
	"github.com/perlin-network/noise/nat"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/internal/snappy"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

import _ "net/http/pprof"

func main() {
	pprofFlag := flag.Bool("pprof", false, "host pprof server on port 9000")
	flag.Parse()

	if *pprofFlag {
		go func() {
			fmt.Println(http.ListenAndServe("localhost:9000", nil))
		}()
	}

	log.Register(log.NewConsoleWriter(log.FilterFor(log.ModuleNode, log.ModuleSync, log.ModuleConsensus, log.ModuleMetrics)))

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

	ledger := wavelet.NewLedger(client)

	go func() {
		server := client.Listen()

		wavelet.RegisterWaveletServer(server, ledger.Protocol())

		if err := server.Serve(listener); err != nil {
			panic(err)
		}
	}()

	if len(flag.Args()) > 1 {
		for _, addr := range flag.Args()[1:] {
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

		for i := uint64(0); i < count; i++ {
			tags := make([]byte, 40)
			payloads := make([][]byte, 40)
			tx := wavelet.AttachSenderToTransaction(keys, wavelet.NewBatchTransaction(keys, tags, payloads), ledger.Graph().FindEligibleParents()...)

			//tx := wavelet.AttachSenderToTransaction(keys, wavelet.NewTransaction(keys, sys.TagNop, nil), ledger.Graph().FindEligibleParents()...)

			if err := ledger.AddTransaction(tx); err != nil && errors.Cause(err) != wavelet.ErrMissingParents {
				fmt.Printf("error adding tx to graph [%v]: %+v\n", err, tx)
			}
		}
	}

	select {}
}
