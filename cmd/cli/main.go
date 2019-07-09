package main

import (
	"fmt"
	"os"

	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet/cmd/cli/server"
	"github.com/perlin-network/wavelet/cmd/cli/tui/logger"
	"github.com/perlin-network/wavelet/sys"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var log *logger.Logger

func main() {
	// Add the config file flag
	var configFile string
	flag.StringVarP(&configFile, "config", "c", "",
		"Path to TOML config file, overrides command line arguments.")

	// promptConfigDialog when true starts the TUI with a dialog to change
	// parameters
	var promptConfigDialog bool
	flag.BoolVarP(&promptConfigDialog, "prompt-config", "p", true,
		"Start the TUI with a dialog to change parameters.")

	// Add the server config flags
	c := server.Config{}

	flag.StringVar(&c.Host, "host", "127.0.0.1",
		"Listen for peers on host address.")
	flag.BoolVar(&c.NAT, "nat", false,
		"Enable port forwarding, only required for PCs.")
	flag.UintVar(&c.Port, "port", 3000,
		"Listen for peers on port.")
	flag.UintVar(&c.APIPort, "api.port", 0,
		"Start a local API HTTP server at this port.")
	flag.StringVar(&c.Wallet, "wallet", "config/wallet.txt",
		"Path to file containing hex-encoded private key. If "+
			"the path specified is invalid, or no file exists at the "+
			"specified path, a random wallet will be generated. "+
			"Optionally, a 128-length hex-encoded private key to a "+
			"wallet may also be specified.")
	flag.StringVar(&c.Genesis, "genesis", "",
		"Genesis JSON file contents representing initial fields of some set "+
			"of accounts at round 0.")
	flag.StringVar(&c.Database, "db", "",
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

	// Set the oddballs
	sys.MinDifficulty = byte(*difficulty)

	// Make a new global logger
	log = logger.NewLogger()

	tview.Initialize()

	// TODO(diamond): actual TUI code lmao
	/* This to be called in the dialog callback
	srv, err := server.New(c, log)
	if err != nil {
		fatal("Failed to start server", err)
	}
	*/

	if err := tview.Run(); err != nil {
		fatal(err)
	}
}

func fatal(i ...interface{}) {
	fmt.Fprintln(os.Stderr, i...)
	os.Exit(1)
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
