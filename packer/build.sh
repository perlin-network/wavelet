#!/bin/sh

packer validate wavelet.json
packer inspect wavelet.json
packer build -only=amazon-ebs wavelet.json

