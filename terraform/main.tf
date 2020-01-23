provider "aws" {
  region = var.region
}

locals {
	node_count = length(var.node_private_keys)
}

resource "aws_instance" "wavelet-node" {
	count = local.node_count
	
	ami = var.ami_id
	instance_type = var.instance_type
	key_name = var.key_name
	security_groups = [aws_security_group.wavelet-node-sg.name]

	user_data = element(data.template_file.wavelet-node-setup.*.rendered, count.index)

	tags = {
		Name = "wavelet-node-${count.index}"
	}
}

resource "aws_security_group" "wavelet-node-sg" {
	description = "Wavelet node security group"

	ingress {
		from_port = 3000
		to_port = 3000
		protocol = "tcp"
		cidr_blocks = ["0.0.0.0/0"]
		description = "Allow Wavelet RPC port from anywhere"
	}

	ingress {
		from_port = 9000
		to_port = 9000
		protocol = "tcp"
		cidr_blocks = ["0.0.0.0/0"]
		description = "Allow Wavelet API port from anywhere"
	}

	ingress {
		from_port = 22
		to_port = 22
		protocol = "tcp"
		cidr_blocks = ["0.0.0.0/0"]
		description = "Allow SSH from anywhere"
	}

	egress {
		from_port = 0
		to_port = 0
		protocol = "-1"
		cidr_blocks = ["0.0.0.0/0"]
	}
}

data "template_file" "wavelet-node-setup" {
	count = local.node_count
	template = file("./data/setup.tpl.sh")

	vars = {
		private_key = element(var.node_private_keys, count.index)
		rpc_port = var.rpc_port
		api_port = var.api_port
		peer_address = var.testnet_address
	}
}

