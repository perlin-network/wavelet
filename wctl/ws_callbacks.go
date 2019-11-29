package wctl

// Docs: https://wavelet.perlin.net/docs/ws

type (
	// Other callbacks always take a pointer to a structure as the only
	// argument. An example:
	//     func(*api.NetworkJoin)

	// OnError called on any WS error
	OnError = func(error)
)
