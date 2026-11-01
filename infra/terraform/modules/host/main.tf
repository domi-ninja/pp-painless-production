terraform {
  required_version = ">= 1.10.0"
  required_providers {
    beszel = { source = "terraform.local/domi/beszel", version = "0.1.0" }
  }
}

variable "name" { type = string }
variable "address" { type = string }
variable "ssh_user" {
  type    = string
  default = "deploy"
}
variable "local_connection" {
  type    = bool
  default = false
}
variable "configure_base" {
  type    = bool
  default = true
}
variable "host_variables_json" {
  type      = string
  sensitive = true
  default   = "{}"
  validation {
    condition     = can(jsondecode(var.host_variables_json))
    error_message = "host_variables_json must be JSON."
  }
}
variable "alerts" {
  type    = map(string)
  default = { Disk = "85:5", Memory = "90:5", CPU = "95:10", Status = "0:2" }
}
resource "beszel_host" "this" {
  name   = var.name
  alerts = var.alerts
  lifecycle { prevent_destroy = true }
}
locals {
  role_path = "${path.module}/../../../ansible/roles/beszel_agent"
  role_hash = sha256(join("", [for f in sort(fileset(local.role_path, "**")) : filesha256("${local.role_path}/${f}")]))
  config = {
    name  = var.name, address = var.address, user = var.ssh_user,
    local = var.local_connection, configure_base = var.configure_base,
    variables = merge(jsondecode(var.host_variables_json), {
      beszel_enabled     = true,
      beszel_system_name = var.name,
      beszel_hub_url     = beszel_host.this.hub_url,
      beszel_public_key  = beszel_host.this.public_key,
      beszel_agent_token = beszel_host.this.agent_token,
    })
  }
}
resource "terraform_data" "configuration" {
  triggers_replace = [sha256(jsonencode(local.config)), local.role_hash, filesha256("${path.module}/provision.ts")]
  provisioner "local-exec" {
    command     = "node \"${path.module}/provision.ts\""
    environment = { PP_HOST_CONFIG = jsonencode(local.config) }
  }
}
output "beszel_system_id" { value = beszel_host.this.id }
