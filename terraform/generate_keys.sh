#!/bin/bash

rm -f private_keys.txt
TMPDIR=$(mktemp -d)

go run ../cmd/wallet/main.go -dir $TMPDIR -n $COUNT > /dev/null 2>&1

for i in $(seq 1 $COUNT)
do
  echo $(cat $TMPDIR/wallet$i.txt) >> private_keys.txt
done

rm -rf $TMPDIR

