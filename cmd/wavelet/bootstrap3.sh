#!/usr/bin/env bash
wavelets=(
	"--port 3000 --api.port 9000 --wallet config/wallet.txt"
	"--port 3001 --api.port 9001 --wallet config/wallet2.txt 127.0.0.1:3000"
	"--port 3002 --api.port 9002 --wallet config/wallet3.txt 127.0.0.1:3000"
)
file="/tmp/wavelet_counter"

[[ $1 == "reset" ]] && {
	echo > "$file"
	exit
}

[[ $1 ]] && counter=$[$1-1] || {
	[[ -f "$file" ]] && counter=$(< "$file")
	[[ $counter   ]] || counter=-1
	((  counter++ ))
	((  counter >= ${#wavelets[@]} )) && counter=0

	echo -n "$counter" > "$file"
}

(( counter >= ${#wavelets[@]} )) && exit 2

go run . ${wavelets[$counter]}
