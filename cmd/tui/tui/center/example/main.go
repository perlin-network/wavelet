package main

import (
	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet/cmd/tui/tui/center"
)

func main() {
	tview.Initialize()

	tv := tview.NewTextView()
	tv.SetWordWrap(true)
	tv.SetText(`	Lorem ipsum dolor sit amet, consectetur adipiscing elit. Vestibulum posuere leo vel fermentum porta. Etiam eget vulputate risus, ac suscipit eros. Mauris at orci eu elit facilisis laoreet a a augue. Integer a magna sed tortor consequat tempus. Ut ut tempor libero, id ornare elit. Aenean a rhoncus erat. Donec pulvinar blandit neque ac hendrerit. Phasellus vel est non purus faucibus mattis. Curabitur vel nunc sem. Aliquam interdum mi mauris, et auctor purus lobortis a. Nam vitae vulputate ipsum, non semper nisl. Fusce neque sapien, fermentum at fermentum eu, ornare sed dolor. Vestibulum ante ipsum primis in faucibus orci luctus et ultrices posuere cubilia Curae; Vivamus luctus finibus ligula, sit amet hendrerit magna egestas et. Nulla facilisi. Cras in efficitur lorem.

	Quisque gravida justo vel nibh volutpat euismod. In at congue enim, nec pellentesque nulla. In quis est sit amet neque mattis aliquam. Suspendisse potenti. Sed vitae justo scelerisque, fringilla enim in, sodales purus. Nullam ligula mauris, efficitur et iaculis eu, consequat non enim. Lorem ipsum dolor sit amet, consectetur adipiscing elit. Pellentesque habitant morbi tristique senectus et netus et malesuada fames ac turpis egestas.
	
	Etiam quis risus luctus, ultricies magna a, suscipit est. Praesent quis volutpat massa. Suspendisse auctor hendrerit ex, quis dictum tortor consectetur non. Vivamus vel diam venenatis, vulputate nisl et, vehicula neque. Donec blandit nisl ut ipsum condimentum varius et nec neque. Mauris viverra dolor quis tortor dapibus, sed congue mi commodo. Sed tincidunt elementum elit ut dictum.`)

	c := center.New(tv)
	c.MaxHeight = 10
	c.MaxWidth = 40

	tview.SetRoot(c, true)

	if err := tview.Run(); err != nil {
		panic(err)
	}
}
