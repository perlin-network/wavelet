package main

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/cmd/tui/tui/forms"
	"github.com/perlin-network/wavelet/cmd/tui/tui/helpkeyer"
	"github.com/perlin-network/wavelet/cmd/wavelet/node"
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/perlin-network/wavelet/wctl"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"

	_ "github.com/perlin-network/wavelet/cmd/tui/tui/clearbg"
	tuilogger "github.com/perlin-network/wavelet/cmd/tui/tui/logger"
)

var logger *tuilogger.Logger
var client *wctl.Client

func fatalln(v ...interface{}) {
	fmt.Fprintln(os.Stderr, v...)
}

func main() {
	/*
		// Add the config file flag
		var configFile string
		flag.StringVarP(&configFile, "config", "c", "",
			"Path to TOML config file, overrides command line arguments.")
	*/

	flag := pflag.NewFlagSet("wavelet-tui", pflag.ExitOnError)

	// promptConfigDialog when true starts the TUI with a dialog to change
	// parameters
	var promptConfigDialog bool
	flag.BoolVarP(&promptConfigDialog, "prompt-config", "p", true,
		"Start the TUI with a dialog to change parameters.")

	var logLevel string
	flag.StringVar(&logLevel, "loglevel", "debug",
		"Minimum log level to output. Possible values: debug, info, warn, "+
			"error, fatal, panic.")

	// Application-specific flag
	var serverAddr, updateURL string
	flag.StringVar(&serverAddr, "server", "",
		"Connect to a Wavelet server instead of hosting a new one if not blank.")
	flag.StringVar(&updateURL, "update-url", "https://updates.perlin.net/wavelet",
		"URL for updating Wavelet node.")

	// Add the server config flags
	wctlCfg := wctl.Config{}
	srvCfg := node.Config{}

	// Server config
	flag.BoolVar(&srvCfg.NAT, "nat", false,
		"Enable port forwarding, only required for PCs.")
	flag.StringVar(&srvCfg.Host, "host", "127.0.0.1",
		"Listen for peers on host address.")
	flag.UintVar(&srvCfg.Port, "port", 3000,
		"Listen for peers on port.")
	flag.StringVar(&srvCfg.Wallet, "wallet", "config/wallet.txt",
		"Path to file containing hex-encoded private key. If "+
			"the path specified is invalid, no file exists at the "+
			"specified path, or the string given is not a proper "+
			"hex-encoded private key, a random wallet will be generated. "+
			"Optionally, a 128-length hex-encoded private key to a "+
			"wallet may also be specified.")
	flag.UintVar(&srvCfg.APIPort, "api.port", 0,
		"Start a local API HTTP server at this port.")
	flag.StringVar(&srvCfg.Database, "db", "",
		"Directory path to the database. If empty, a temporary in-memory "+
			"database will be used instead.")
	flag.Uint64Var(&srvCfg.MaxMemoryMB, "memory-max", 0,
		"Maximum memory in MB allowed to be used by wavelet.")
	flag.StringVar(&srvCfg.APIHost, "api.host", "",
		"Host for the API HTTPS node.")
	flag.StringVar(&srvCfg.APICertsCache, "api.certs", "",
		"Directory path to cache HTTPS certificates.")

	var genesis string // outlier
	flag.StringVar(&genesis, "genesis", "",
		"Genesis JSON file contents representing initial fields of some set "+
			"of accounts at round 0.")

	var secret string
	flag.StringVar(&secret, "api.secret", "",
		"Shared secret to restrict access to some API endpoints.")

	// Add the flags for the sys package

	// Flags that can be set directly
	flag.Uint8Var(&sys.MinDifficulty, "sys.difficulty.min", sys.MinDifficulty,
		"Minimum difficulty to define a critical transaction.")
	flag.Float64Var(&sys.DifficultyScaleFactor, "sys.difficulty.scale",
		sys.DifficultyScaleFactor,
		"Factor to scale a transactions confidence down by to "+
			"compute the difficulty needed to define a critical transaction")
	flag.Uint64Var(&sys.DefaultTransactionFee, "sys.transaction_fee_amount",
		sys.DefaultTransactionFee, "")
	flag.Uint64Var(&sys.MinimumStake, "sys.min_stake", sys.MinimumStake,
		"Minimum stake to garner validator rewards and have importance in "+
			"consensus.")

	// Flags that need to be updated
	var (
		snowballK    int
		snowballBeta int
		queryTimeout time.Duration
		maxDepthDiff uint64
	)

	flag.IntVar(&snowballK, "sys.snowball.k", conf.GetSnowballK(),
		"Snowball consensus protocol parameter K.")
	flag.IntVar(&snowballBeta, "sys.snowball.beta", conf.GetSnowballBeta(),
		"Snowball consensus protocol parameter Beta.")
	flag.DurationVar(&queryTimeout, "sys.query_timeout", conf.GetQueryTimeout(),
		"Timeout in seconds for querying a transaction to K peers.")
	flag.Uint64Var(&maxDepthDiff, "sys.max_depth_diff", conf.GetMaxDepthDiff(),
		"Snowball consensus protocol parameter Beta.")

	flag.Float64Var(&sys.DifficultyScaleFactor, "sys.difficulty.scale", sys.DifficultyScaleFactor,
		"Factor to scale a transactions confidence down by to "+
			"compute the difficulty needed to define a critical transaction")

	// Parse the flags
	flag.Parse(os.Args)

	// Start the auto-updater
	// go periodicUpdateRoutine(updateURL)

	// Set the log level
	log.SetLevel(logLevel)

	// Generate the wallet
	w, err := wallet(srvCfg.Wallet)
	if err != nil {
		fatalln(err)
	}

	// Generate secret if empty
	if secret == "" {
		// If the secret is empty, derive the secret from the base64-hashed
		// sha224-hashed private key.
		h, err := hex.DecodeString(w)
		if err != nil {
			fatalln("Can't decode wallet:", err)
		}

		sha := sha512.Sum512_224(h)
		secret = base64.StdEncoding.EncodeToString(sha[:])
	}

	// Update sys variables
	conf.Update(
		conf.WithSnowballK(snowballK),
		conf.WithSnowballBeta(snowballBeta),
		conf.WithQueryTimeout(queryTimeout),
		conf.WithMaxDepthDiff(maxDepthDiff),
		conf.WithSecret(secret),
	)

	// Set the CLI settings
	wctlCfg.APISecret = secret

	if serverAddr == "" { // start a server
		// Set the server's config
		srvCfg.Wallet = w
		srvCfg.Peers = flag.Args()

		if genesis != "" {
			srvCfg.Genesis = &genesis
		}

		srv, err := node.New(&srvCfg)
		if err != nil {
			fatalln("Failed to host the node server:", err)
		}

		srv.Start()
		defer srv.Close()

		wctlCfg.Server = srv
		wctlCfg.APIPort = uint16(srvCfg.APIPort)
		wctlCfg.PrivateKey = srv.Keypair.PrivateKey()
		wctlCfg.APIHost = "127.0.0.1"

		if srvCfg.APIHost != "" {
			wctlCfg.APIHost = srvCfg.APIHost
			wctlCfg.UseHTTPS = true
		}
	} else {
		u, err := url.Parse(serverAddr)
		if err != nil {
			fatalln("Invalid address", serverAddr+":", err)
		}

		port, _ := strconv.ParseUint(u.Port(), 10, 16)

		wctlCfg.APIPort = uint16(port)
		wctlCfg.APIHost = u.Host
		wctlCfg.UseHTTPS = u.Scheme == "https"
	}

	cli, err := wctl.NewClient(wctlCfg)
	if err != nil {
		fatalln("Failed to start the wctl client:", err)

	}

	client = cli

	/*
		// Assign stuff to parse the config file if provided one
		if configFile != "" {
			// Bind (p)flag to Viper
			viper.BindPFlags(flag.CommandLine)

			// Set the config file path to the variable parsed
			viper.SetConfigFile(configFile)

			if err := viper.ReadInConfig(); err != nil {
				fatal("Failed to read config:", err)
			}
		}
	*/

	tview.Initialize().MouseSupport = false
	tview.Styles.PrimaryTextColor = -1
	tview.Styles.SecondaryTextColor = -1
	tview.Styles.TitleColor = -1

	/*
		if promptConfigDialog {
			tview.SetRoot(configUI())
		} else {*/

	ui := mainUI(cli)

	tview.SetRoot(ui, true)
	tview.SetFocus(ui)

	// }

	// TODO(diamond): actual TUI code lmao

	if err := tview.Run(); err != nil {
		fatal("Failed to run tview:", err)
	}
}

func fatal(i ...interface{}) {
	fmt.Fprintln(os.Stderr, i...)
	os.Exit(1)
}

/*
func configUI() tview.Primitive {
	form := tview.NewForm()
	form.AddInputField()
	return nil
}
*/

func mainUI(cli *wctl.Client) tview.Primitive {
	// Initialize
	forms.DefaultWidth = 120
	logger = tuilogger.NewLogger()

	// TODO(diamond): Add Tab key to cycle focus
	// TODO(diamond): Indicative borders?

	flex := tview.NewFlex()
	flex.SetDirection(tview.FlexRow)

	// Add the logger
	flex.AddItem(logger, 0, 1, false)

	// Make a statusbar
	stat := tview.NewTextView()
	stat.SetBackgroundColor(tcell.ColorGreen)
	stat.SetTextColor(tcell.ColorWhite)
	// TODO: add an event callback
	stat.SetText(fmt.Sprintf(
		"⣿ wavelet: %s - %d PERLs", "add a callback here", 0,
	))
	flex.AddItem(stat, 1, 1, false)

	// TODO(diamond): Dialog API to actually make this easier, or at least
	// break it down into functions on other files.
	// - [x] API: forms
	// - [x] Split callbacks to other functions
	// - [x] Autocompletion for IDs
	// - [x] File browser
	hk := helpkeyer.New()
	hk.Blocking = false
	hk.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		logger.InputHandler()(ev, nil)
		return ev
	})

	hk.Set('s', "status", keyStatus)
	hk.Set('p', "pay", keyPay)
	hk.Set('f', "find", keyFind)
	hk.Set('n', "spawn", keySpawn)
	hk.Set('a', "place stake", keyPlaceStake)
	hk.Set('w', "withdraw stake", keyWithdrawStake)
	hk.Set('r', "withdraw reward", keyWithdrawReward)

	flex.AddItem(hk, 2, 1, true)

	return flex
}

// returns hex-encoded
func wallet(wallet string) (string, error) {
	var keys *skademlia.Keypair

	logger := log.Node()

	privateKeyBuf, err := ioutil.ReadFile(wallet)

	// File exists
	if err == nil {
		// If the file content is the actual private key
		if hex.DecodedLen(len(privateKeyBuf)) == edwards25519.SizePrivateKey {
			return string(privateKeyBuf), nil
		}
	}

	if os.IsNotExist(err) {
		// If a private key is specified, simply use the provided private key instead.
		if len(wallet) == hex.EncodedLen(edwards25519.SizePrivateKey) {
			return wallet, nil
		}
	}

	keys, err = skademlia.NewKeys(sys.SKademliaC1, sys.SKademliaC2)
	if err != nil {
		return "", errors.New("failed to generate a new wallet")
	}

	privateKey := keys.PrivateKey()
	publicKey := keys.PublicKey()

	logger.Info().
		Hex("privateKey", privateKey[:]).
		Hex("publicKey", publicKey[:]).
		Msg("Existing wallet not found: generated a new one.")

	return hex.EncodeToString(privateKey[:]), nil
}

/*
func main() {
	app := tview.NewApplication()
	defer app.Stop()

	app.SetBeforeDrawFunc(func(s tcell.Screen) bool {
		s.Clear()
		return false
	})

	header := tview.NewTextView()
	header.SetBackgroundColor(tcell.GetColor("#5f06fe"))
	header.SetTextColor(tcell.ColorWhite)
	header.SetDynamicColors(true)
	header.SetText(`⣿ wavelet v0.1.0`)

	header.Highlight("b")

	content := tview.NewTextView()
	content.SetWrap(true)
	content.SetWordWrap(true)
	content.SetBackgroundColor(tcell.ColorDefault)
	content.SetTextColor(tcell.ColorDefault)
	content.SetDynamicColors(true)
	content.SetBorderPadding(1, 1, 1, 1)
	content.SetChangedFunc(func() {
		app.ForceDraw()
	})
	content.SetScrollable(true)

	logger := log.Node()

	info := tview.NewTextView()
	info.SetText("⣿ Logged in as [yellow:black][696937c2c8df35dba0169de72990b80761e51dd9e2411fa1fce147f68ade830a][default:default]. You have 0 [#5cba3a]PERLs[default] available.")
	info.SetBackgroundColor(tcell.ColorDefault)
	info.SetTextColor(tcell.ColorDefault)
	info.SetDynamicColors(true)

	input := tview.NewInputField()
	input.SetFieldBackgroundColor(tcell.ColorDefault)
	input.SetFieldTextColor(tcell.ColorDefault)
	input.SetLabel(tview.TranslateANSI(color.New(color.Bold).Sprint("127.0.0.1:3000: ")))
	input.SetLabelColor(tcell.ColorDefault)
	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Name() {
		case "Ctrl+V":
			txt, err := clipboard.ReadAll()
			if err != nil {
				return nil
			}

			input.SetText(input.GetText() + txt)

			return nil
		}

		return event
	})
	input.SetBackgroundColor(tcell.ColorDefault)
	input.SetPlaceholderTextColor(tcell.ColorDefault)
	input.SetPlaceholder("Enter a command...")
	input.SetBorderPadding(0, 0, 1, 0)
	input.SetBorderColor(tcell.ColorDefault)
	input.SetFieldWidth(0)
	input.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			cmd := input.GetText()

			if len(cmd) > 0 {
				content.Write([]byte("> " + cmd + "\n"))
				content.ScrollToEnd()

				input.SetText("")
			}
		}
	})
	input.SetBorder(true)

	log.SetWriter(log.LoggerWavelet, log.NewConsoleWriter(tview.ANSIWriter(content), log.FilterFor(log.ModuleNode)))

	logger.Info().Msg("Welcome to Wavelet.")
	logger.Info().Msg("Press " + color.New(color.Bold).Sprint("Enter") + " for a list of commands.")

	grid := tview.NewGrid().
		SetRows(1, 0, 1, 3).
		SetColumns(30, 0, 30)
	//SetBorders(true).
	//SetBordersColor(tcell.ColorDefault)

	grid.AddItem(header, 0, 0, 1, 30, 0, 0, false)
	grid.AddItem(content, 1, 0, 1, 30, 0, 0, false)

	grid.AddItem(info, 2, 0, 1, 30, 0, 0, false)
	grid.AddItem(input, 3, 0, 1, 30, 0, 0, true)

	app.SetRoot(grid, true)

	if err := app.Run(); err != nil {
		panic(err)
	}
}
*/
