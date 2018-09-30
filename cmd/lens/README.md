## Build lens production assets

```bash
mkdir -p $GOPATH/src/github/perlin-network
cd $GOPATH/src/github/perlin-network
git clone git@github.com:perlin-network/lens.git
cd lens
yarn install
yarn build
```

# Build lens exec

```bash
go get github.com/rakyll/statik
cd $GOPATH/src/github/perlin-network/wavelet/cmd/lens
go generate
go run main.go

go build
./lens
  # browse localhost:3000
```
