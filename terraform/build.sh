#!/bin/bash

function join_by { local IFS="$1"; shift; echo "$*"; }

# Get latest AMI ids

export TF_VAR_ami_id=$(aws ec2 describe-images \
  --filter 'Name=is-public,Values=false'  \
  --query 'Images[].[ImageId, Name]' \
  --output text | sort -k2 | grep 'perlin-wavelet' | tail -1 | cut -f1);

# Populate private keys

PRIVATE_KEYS=()
while read p; do
  PRIVATE_KEYS+=($(echo \"$p\"))
done <private_keys.txt;

export TF_VAR_node_private_keys="[$(join_by , ${PRIVATE_KEYS[@]})]"

terraform get
terraform init
terraform validate
terraform plan
terraform apply -auto-approve

