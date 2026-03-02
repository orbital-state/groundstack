variable "location" {
  type        = string
  description = "Azure location (emulated)."
  default     = "westeurope"
}

variable "resource_group_name" {
  type        = string
  description = "Resource group name."
  default     = "gs-basic-rg"
}

variable "storage_account_name" {
  type        = string
  description = "Storage account name (must be 3-24 chars, lowercase letters and numbers)."
  default     = "gsbasicstor1234"
}

variable "tags" {
  type        = map(string)
  description = "Tags applied to created resources."
  default     = {
    "groundstack" = "true"
    "example"     = "basic"
  }
}
