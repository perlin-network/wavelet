package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/perlin-network/wavelet/cmd/lens/statik"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/params"
	"github.com/rakyll/statik/fs"
	"github.com/urfave/cli"
)

type config struct {
	LensHost          string
	LensPort          uint
	APIHost           string
	APIPort           uint
	APIPrivateKeyFile string
}

func main() {

	app := cli.NewApp()

	app.Name = "lens"
	app.Author = "Perlin Network"
	app.Email = "support@perlin.net"
	app.Version = params.Version
	app.Usage = "web interface to a Perlin node's API"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "lens.host",
			Value: "localhost",
			Usage: "Host address that will serve lens web content `LENS_HOST`.",
		},
		cli.UintFlag{
			Name:  "lens.port",
			Value: 8080,
			Usage: "Port that will serve lens web content `LENS_PORT`.",
		},
		cli.StringFlag{
			Name:  "api.host",
			Value: "localhost",
			Usage: "Host of the local HTTP API `API_HOST`.",
		},
		cli.IntFlag{
			Name:  "api.port",
			Value: 9000,
			Usage: "Port of the local HTTP API `API_PORT`.",
		},
		cli.StringFlag{
			Name:  "api.private_key_file",
			Usage: "The file containing private key that will make transactions through the API `API_PRIVATE_KEY_FILE` (required).",
		},
	}

	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("Version:          %s\n", c.App.Version)
		fmt.Printf("Go Version:       %s\n", params.GoVersion)
		fmt.Printf("Git Commit:       %s\n", params.GitCommit)
		fmt.Printf("OS/Arch:          %s\n", params.OSArch)
		fmt.Printf("Lens Git Version: %s\n", statik.LensGitVersion)
		fmt.Printf("Built:            %s\n", c.App.Compiled.Format(time.ANSIC))
	}

	app.Action = func(c *cli.Context) {
		conf := &config{
			LensHost:          c.String("lens.host"),
			LensPort:          c.Uint("lens.port"),
			APIHost:           c.String("api.host"),
			APIPort:           c.Uint("api.port"),
			APIPrivateKeyFile: c.String("api.private_key_file"),
		}

		log.Info().
			Interface("config", conf).
			Msg("Lens is being served.")

		exit := make(chan os.Signal, 1)
		signal.Notify(exit, os.Interrupt)

		go func() {
			<-exit

			os.Exit(0)
		}()

		runServer(conf)
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal().Err(err).Msg("Failed to parse configuration/command-line arugments.")
	}
}

func runServer(c *config) {
	statikFS, err := fs.New()
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	http.Handle("/", http.FileServer(statikFS))
	log.Fatal().Err(http.ListenAndServe(fmt.Sprintf("%s:%d", c.LensHost, c.LensPort), nil)).Msg("")
}
