variable "region" {
	type = string
	default = "ap-southeast-1"
}
variable "ami_id" {
	type = string
}
variable "instance_type" {
	type = string
	default = "t2.micro"
}
variable "key_name" {
	type = string
}
variable "rpc_port" {
	type = number
	default = 3000
}
variable "api_port" {
	type = number
	default = 9000
}
variable "node_private_keys" {
	type = list(string)
}
variable "testnet_address" {
	type = string
	default = "testnet.perlin.net:3000"
}

