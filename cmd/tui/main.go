package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/cmd/tui/server"
	"github.com/perlin-network/wavelet/cmd/tui/tui/forms"
	"github.com/perlin-network/wavelet/cmd/tui/tui/helpkeyer"
	"github.com/perlin-network/wavelet/cmd/tui/tui/logger"
	"github.com/perlin-network/wavelet/sys"
	flag "github.com/spf13/pflag"

	_ "github.com/perlin-network/wavelet/cmd/tui/tui/clearbg"
)

var log *logger.Logger
var cfg *server.Config
var srv *server.Server

var debug bool

func main() {
	/*
		// Add the config file flag
		var configFile string
		flag.StringVarP(&configFile, "config", "c", "",
			"Path to TOML config file, overrides command line arguments.")
	*/

	// promptConfigDialog when true starts the TUI with a dialog to change
	// parameters
	var promptConfigDialog bool
	flag.BoolVarP(&promptConfigDialog, "prompt-config", "p", true,
		"Start the TUI with a dialog to change parameters.")

	// Debug bool
	flag.BoolVar(&debug, "debug", false, "Dump stack trace on error")

	// Add the server config flags
	cfg = &server.Config{}

	flag.StringVar(&cfg.Host, "host", "127.0.0.1",
		"Listen for peers on host address.")
	flag.BoolVar(&cfg.NAT, "nat", false,
		"Enable port forwarding, only required for PCs.")
	flag.UintVar(&cfg.Port, "port", 3000,
		"Listen for peers on port.")
	flag.UintVar(&cfg.APIPort, "api.port", 0,
		"Start a local API HTTP server at this port.")
	flag.StringVar(&cfg.Wallet, "wallet", "config/wallet.txt",
		"Path to file containing hex-encoded private key. If "+
			"the path specified is invalid, no file exists at the "+
			"specified path, or the string given is not a proper "+
			"hex-encoded private key, a random wallet will be generated. "+
			"Optionally, a 128-length hex-encoded private key to a "+
			"wallet may also be specified.")
	flag.StringVar(&cfg.Genesis, "genesis", "",
		"Genesis JSON file contents representing initial fields of some set "+
			"of accounts at round 0.")
	flag.StringVar(&cfg.Database, "db", "",
		"Directory path to the database. If empty, a temporary in-memory "+
			"database will be used instead.")

	// Add the flags for the sys package
	flag.DurationVar(&sys.QueryTimeout, "sys.query_timeout", sys.QueryTimeout,
		"Timeout in seconds for querying a transaction to K peers.")
	flag.Uint64Var(&sys.MaxDepthDiff, "sys.max_depth_diff", sys.MaxDepthDiff,
		"Max graph depth difference to search for eligible transaction "+
			"parents from for our node.")
	flag.Uint64Var(&sys.TransactionFeeAmount, "sys.transaction_fee_amount",
		sys.TransactionFeeAmount,
		"Amount of transaction fee.")
	flag.Uint64Var(&sys.MinimumStake, "sys.min_stake", sys.MinimumStake,
		"Minimum stake to garner validator rewards and have importance in "+
			"consensus.")
	flag.IntVar(&sys.SnowballK, "sys.snowball.k", sys.SnowballK,
		"Snowball consensus protocol parameter K.")
	flag.Float64Var(&sys.SnowballAlpha, "sys.snowball.alpha", sys.SnowballAlpha,
		"Snowball consensus protocol parameter Alpha.")
	flag.IntVar(&sys.SnowballBeta, "sys.snowball.beta", sys.SnowballBeta,
		"Snowball consensus protocol parameter Beta.")
	flag.Float64Var(&sys.DifficultyScaleFactor, "sys.difficulty.scale",
		sys.DifficultyScaleFactor,
		"Factor to scale a transactions confidence down by to "+
			"compute the difficulty needed to define a critical transaction")

	// Oddballs
	difficulty := flag.Int("sys.min.difficulty", int(sys.MinDifficulty),
		"Minimum difficulty to define a critical transaction.")

	// Parse the flags
	flag.Parse()

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

	// Set the oddballs
	sys.MinDifficulty = byte(*difficulty)
	cfg.Peers = flag.Args()

	// Make a new global logger
	log = logger.NewLogger()

	tview.Initialize().MouseSupport = false
	tview.Styles.PrimaryTextColor = -1
	tview.Styles.SecondaryTextColor = -1
	tview.Styles.TitleColor = -1

	/*
		if promptConfigDialog {
			tview.SetRoot(configUI())
		} else {*/

	ui := mainUI()

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

func mainUI() tview.Primitive {
	s, err := server.New(*cfg, log)
	if err != nil {
		fatal("Failed to start server", err)
	}

	srv = s

	go srv.Start()

	forms.DefaultWidth = 120

	// TODO(diamond): Add Tab key to cycle focus
	// TODO(diamond): Indicative borders?

	flex := tview.NewFlex()
	flex.SetDirection(tview.FlexRow)

	// Add the logger
	flex.AddItem(log, 0, 1, false)

	// Make a statusbar
	stat := tview.NewTextView()
	stat.SetBackgroundColor(tcell.ColorGreen)
	stat.SetTextColor(tcell.ColorWhite)
	// Doesn't seem very bright, but this is updating the status bar 15
	// times every second
	go func() {
		pub := s.Keys.PublicKey()
		key := hex.EncodeToString(pub[:])

		// 15 redraws/sec == 15fps minimum
		for range time.NewTicker(time.Second / 15).C {
			snap := s.Ledger.Snapshot()
			bal, _ := wavelet.ReadAccountBalance(snap, pub)

			stat.SetText(fmt.Sprintf(
				"⣿ wavelet: %s - %d PERLs", key[:8], bal,
			))

			tview.Draw()
		}
	}()

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
		log.InputHandler()(ev, nil)
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
