package filechooser

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet/cmd/tui/tui/errdialog"
)

var (
	FolderColor = "blue"
	FileColor   = "white"
)

// FileChooser contains structs that make a file chooser
type FileChooser struct {
	*tview.List

	PWD   os.FileInfo
	Path  string
	Files []os.FileInfo

	// The callback when a file is selected
	FilePick func(filename string)

	lists []*tview.ListItem
}

// New creates a new file chooser
func New() *FileChooser {
	c := &FileChooser{
		List: tview.NewList(),
	}

	c.SetHighlightFullLine(true)
	c.ShowSecondaryText(false)
	c.SetSelectedTextColor(tcell.ColorBlack)
	c.SetSelectedFunc(func(i int, _, path string, _ rune) {
		// If the selected directory is ..
		if path == ".." {
			// Go to parent
			if err := c.Parent(); err != nil {
				go errdialog.CallDialog(err.Error(), nil)
			}

			return
		}

		// Find the file
		for _, f := range c.Files {
			if f.Name() != path {
				continue
			}

			path = filepath.Join(c.Path, path)

			// If the file is a directory
			if f.IsDir() {
				if err := c.ChangeDirectory(path); err != nil {
					go errdialog.CallDialog(err.Error(), nil)
				}

				return
			}

			// Else, run the callback
			if c.FilePick != nil {
				go c.FilePick(path)
				return
			}
		}
	})

	return c
}

// Parent is cd ..
func (c *FileChooser) Parent() error {
	return c.ChangeDirectory(filepath.Dir(c.Path))
}

// ChangeDirectory or cd changes the current directory.
func (c *FileChooser) ChangeDirectory(dir string) error {
	if dir == "" {
		dir = "."
	}

	// Get the absolute path of $PWD
	fullpath, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	// Open the directory
	pwd, err := os.Open(fullpath)
	if err != nil {
		return err
	}

	defer pwd.Close()

	// Read the directory for a FileInfo
	i, err := pwd.Stat()
	if err != nil {
		return err
	}

	// Read file stats in the directory
	list, err := pwd.Readdir(-1)
	if err != nil {
		return err
	}

	// Sort the files
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name() < list[j].Name()
	})

	// Set
	c.PWD = i
	c.Files = list
	c.Path = fullpath

	// Generate list items
	c.lists = make([]*tview.ListItem, 0, len(list)+1)

	if fullpath != "/" {
		c.lists = append(c.lists, &tview.ListItem{
			MainText:      "..",
			SecondaryText: "..",
		})
	}

	for _, dir := range list {
		var name = dir.Name()

		if dir.IsDir() {
			name += "/"
			name = "[" + FolderColor + "]" + tview.Escape(name) + "[-]"
		} else {
			name = "[" + FileColor + "]" + tview.Escape(name) + "[-]"
		}

		c.lists = append(c.lists, &tview.ListItem{
			MainText:      name,
			SecondaryText: dir.Name(),
		})
	}

	c.List.SetItems(c.lists)

	return nil
}
