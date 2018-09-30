//go:generate statik -f -src=$GOPATH/src/github.com/perlin-network/lens/build -p statik -dest .
//go:generate go run gen.go

package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/perlin-network/wavelet/cmd/lens/statik"
	"github.com/perlin-network/wavelet/cmd/utils"
	"github.com/perlin-network/wavelet/log"
	"github.com/rakyll/statik/fs"
	"github.com/urfave/cli"
)

func main() {

	app := cli.NewApp()

	app.Name = "lens"
	app.Author = "Perlin Network"
	app.Email = "support@perlin.net"
	app.Version = utils.Version
	app.Usage = "web interface to a Perlin node's API"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "host, address",
			Value: "localhost",
			Usage: "Listen for peers on host address `HOST`.",
		},
		cli.UintFlag{
			Name:  "port, p",
			Value: 8080,
			Usage: "Listen for peers on port `PORT`.",
		},
	}

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("Version: %s\n", c.App.Version)
		fmt.Printf("Go Version: %s\n", utils.GoVersion)
		fmt.Printf("Git Commit: %s\n", utils.GitCommit)
		fmt.Printf("Built: %s\n", c.App.Compiled.Format(time.ANSIC))
		fmt.Printf("Lens Git Version: %s\n", statik.LensGitVersion)
	}

	app.Action = func(c *cli.Context) {
		port := c.Uint("port")
		host := c.String("host")

		log.Info().
			Str("host", host).
			Uint("port", port).
			Msg("Lens is being served.")

		exit := make(chan os.Signal, 1)
		signal.Notify(exit, os.Interrupt)

		go func() {
			<-exit

			os.Exit(0)
		}()

		runServer(host, port)
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal().Err(err).Msg("Failed to parse configuration/command-line arugments.")
	}
}

func runServer(host string, port uint) {
	statikFS, err := fs.New()
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	http.Handle("/", http.FileServer(statikFS))
	log.Fatal().Err(http.ListenAndServe(fmt.Sprintf("%s:%d", host, port), nil)).Msg("")
}
