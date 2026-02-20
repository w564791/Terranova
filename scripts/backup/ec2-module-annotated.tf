# EC2 Module Variables with Inline Annotations
# 使用行尾注释格式标记变量属性

variable "name" {
  description = "Name to be used on EC2 instance created"  # @level:basic alias:实例名称
  type        = string
  default     = ""
}

variable "ami" {
  description = "ID of AMI to use for the instance"  # @level:basic force_new:true conflicts_with:ami_ssm_parameter source:ami_list widget:select searchable:true allowCustom:true alias:AMI镜像
  type        = string
  default     = null

  validation {
    condition     = var.ami == null || can(regex("^ami-", var.ami))
    error_message = "AMI ID must start with 'ami-'."
  }
}

variable "ami_ssm_parameter" {
  description = "SSM parameter name for the AMI ID"  # @conflicts_with:ami
  type        = string
  default     = null
}

variable "instance_type" {
  description = "The type of instance to start"  # @level:basic source:instance_types widget:select searchable:true alias:实例类型
  type        = string
  default     = "t3.micro"
}

variable "availability_zone" {
  description = "AZ to start the instance in"  # @source:availability_zones widget:select alias:可用区
  type        = string
  default     = null
}

variable "subnet_id" {
  description = "The VPC Subnet ID to launch in"  # @level:basic force_new:true source:subnet_list widget:select searchable:true dependsOn:vpc_id alias:子网
  type        = string
  default     = null
}

variable "vpc_security_group_ids" {
  description = "A list of security group IDs to associate with"  # @level:basic source:security_groups widget:multi-select searchable:true dependsOn:vpc_id alias:安全组
  type        = list(string)
  default     = null
}

variable "key_name" {
  description = "Key name of the Key Pair to use for the instance"  # @level:basic source:key_pairs widget:select searchable:true alias:密钥对
  type        = string
  default     = null
}

variable "iam_instance_profile" {
  description = "IAM Instance Profile to launch the instance with"  # @level:basic source:iam_instance_profiles widget:select searchable:true alias:IAM实例配置文件
  type        = string
  default     = null
}

variable "associate_public_ip_address" {
  description = "Whether to associate a public IP address with an instance in a VPC"  # @level:basic alias:公网IP
  type        = bool
  default     = null
}

variable "private_ip" {
  description = "Private IP address to associate with the instance in a VPC"  # @force_new:true alias:私有IP
  type        = string
  default     = null
}

variable "secondary_private_ips" {
  description = "A list of secondary private IPv4 addresses to assign to the instance's primary network interface"  # @widget:tags
  type        = list(string)
  default     = null
}

variable "ipv6_address_count" {
  description = "A number of IPv6 addresses to associate with the primary network interface"
  type        = number
  default     = null
}

variable "ipv6_addresses" {
  description = "Specify one or more IPv6 addresses from the range of the subnet to associate with the primary network interface"  # @widget:tags
  type        = list(string)
  default     = null
}

variable "ebs_optimized" {
  description = "If true, the launched EC2 instance will be EBS-optimized"  # @alias:EBS优化
  type        = bool
  default     = null
}

variable "root_block_device" {
  description = "Customize details about the root block device of the instance"  # @group:storage widget:object alias:根卷配置
  type = list(object({
    delete_on_termination = optional(bool, true)   # @alias:终止时删除
    encrypted             = optional(bool, true)   # @alias:加密
    iops                  = optional(number)       # @alias:IOPS
    kms_key_id            = optional(string)       # @alias:KMS密钥
    throughput            = optional(number)       # @alias:吞吐量
    volume_size           = optional(number, 20)   # @alias:卷大小(GB) min:8 max:16384
    volume_type           = optional(string, "gp3") # @alias:卷类型 widget:select enum:gp2,gp3,io1,io2,st1,sc1
    tags                  = optional(map(string))  # @alias:标签 widget:key-value
  }))
  default = []
}

variable "ebs_block_device" {
  description = "Additional EBS block devices to attach to the instance"  # @group:storage widget:object-list alias:附加EBS卷
  type = list(object({
    device_name           = string                  # @alias:设备名称 placeholder:/dev/sdf
    delete_on_termination = optional(bool, true)    # @alias:终止时删除
    encrypted             = optional(bool, true)    # @alias:加密
    iops                  = optional(number)        # @alias:IOPS
    kms_key_id            = optional(string)        # @alias:KMS密钥
    snapshot_id           = optional(string)        # @alias:快照ID
    throughput            = optional(number)        # @alias:吞吐量
    volume_size           = optional(number)        # @alias:卷大小(GB)
    volume_type           = optional(string, "gp3") # @alias:卷类型 widget:select
    tags                  = optional(map(string))   # @alias:标签 widget:key-value
  }))
  default = []
}

variable "ephemeral_block_device" {
  description = "Customize Ephemeral (also known as Instance Store) volumes on the instance"  # @group:storage widget:object-list
  type = list(object({
    device_name  = string  # @alias:设备名称
    no_device    = optional(bool)
    virtual_name = optional(string)  # @alias:虚拟名称
  }))
  default = []
}

variable "network_interface" {
  description = "Customize network interfaces to be attached at instance boot time"  # @group:network widget:object-list
  type = list(object({
    device_index          = number  # @alias:设备索引
    network_interface_id  = string  # @alias:网络接口ID
    delete_on_termination = optional(bool, false)  # @alias:终止时删除
  }))
  default = []
}

variable "source_dest_check" {
  description = "Controls if traffic is routed to the instance when the destination address does not match the instance"  # @group:network
  type        = bool
  default     = true
}

variable "disable_api_termination" {
  description = "If true, enables EC2 Instance Termination Protection"  # @alias:终止保护
  type        = bool
  default     = null
}

variable "disable_api_stop" {
  description = "If true, enables EC2 Instance Stop Protection"  # @alias:停止保护
  type        = bool
  default     = null
}

variable "instance_initiated_shutdown_behavior" {
  description = "Shutdown behavior for the instance"  # @widget:select enum:stop,terminate
  type        = string
  default     = null
}

variable "placement_group" {
  description = "The Placement Group to start the instance in"  # @force_new:true
  type        = string
  default     = null
}

variable "tenancy" {
  description = "The tenancy of the instance"  # @force_new:true widget:select enum:default,dedicated,host
  type        = string
  default     = null
}

variable "host_id" {
  description = "ID of a dedicated host that the instance will be assigned to"  # @force_new:true
  type        = string
  default     = null
}

variable "cpu_core_count" {
  description = "Sets the number of CPU cores for an instance"  # @deprecated:Use_cpu_options_instead
  type        = number
  default     = null
}

variable "cpu_threads_per_core" {
  description = "Sets the number of CPU threads per core for an instance"  # @deprecated:Use_cpu_options_instead
  type        = number
  default     = null
}

variable "cpu_options" {
  description = "Defines CPU options to apply to the instance at launch time"  # @group:compute widget:object alias:CPU选项
  type = object({
    core_count       = optional(number)  # @alias:核心数
    threads_per_core = optional(number)  # @alias:每核线程数
    amd_sev_snp      = optional(string)  # @alias:AMD SEV-SNP widget:select enum:enabled,disabled
  })
  default = {}
}

variable "capacity_reservation_specification" {
  description = "Describes an instance's Capacity Reservation targeting option"  # @group:compute widget:object
  type = object({
    capacity_reservation_preference = optional(string)  # @widget:select enum:open,none
    capacity_reservation_target = optional(object({
      capacity_reservation_id                 = optional(string)
      capacity_reservation_resource_group_arn = optional(string)
    }))
  })
  default = {}
}

variable "user_data" {
  description = "The user data to provide when launching the instance"  # @widget:textarea alias:用户数据
  type        = string
  default     = null
}

variable "user_data_base64" {
  description = "Can be used instead of user_data to pass base64-encoded binary data directly"  # @widget:textarea
  type        = string
  default     = null
}

variable "user_data_replace_on_change" {
  description = "When used in combination with user_data or user_data_base64 will trigger a destroy and recreate when set to true"  # @force_new:true
  type        = bool
  default     = false
}

variable "hibernation" {
  description = "If true, the launched EC2 instance will support hibernation"  # @alias:休眠支持
  type        = bool
  default     = null
}

variable "enclave_options_enabled" {
  description = "Whether Nitro Enclaves will be enabled on the instance"  # @alias:Nitro Enclave
  type        = bool
  default     = null
}

variable "enable_volume_tags" {
  description = "Whether to enable volume tags"  # @alias:启用卷标签
  type        = bool
  default     = true
}

variable "volume_tags" {
  description = "A mapping of tags to assign to the devices created by the instance at launch time"  # @widget:key-value alias:卷标签
  type        = map(string)
  default     = {}
}

variable "tags" {
  description = "A mapping of tags to assign to the resource"  # @level:basic widget:key-value alias:标签
  type        = map(string)
  default     = {}
}

variable "instance_tags" {
  description = "Additional tags for the instance"  # @widget:key-value
  type        = map(string)
  default     = {}
}

variable "monitoring" {
  description = "If true, the launched EC2 instance will have detailed monitoring enabled"  # @group:monitoring alias:详细监控
  type        = bool
  default     = null
}

variable "get_password_data" {
  description = "If true, wait for password data to become available and retrieve it"  # @group:monitoring
  type        = bool
  default     = null
}

variable "create" {
  description = "Whether to create an instance"  # @hidden:true
  type        = bool
  default     = true
}

variable "create_spot_instance" {
  description = "Depicts if the instance is a spot instance"  # @group:spot alias:创建Spot实例
  type        = bool
  default     = false
}

variable "spot_price" {
  description = "The maximum price to request on the spot market"  # @group:spot alias:Spot价格
  type        = string
  default     = null
}

variable "spot_wait_for_fulfillment" {
  description = "If set, Terraform will wait for the Spot Request to be fulfilled"  # @group:spot
  type        = bool
  default     = null
}

variable "spot_type" {
  description = "If set to one-time, after the instance is terminated, the spot request will be closed"  # @group:spot widget:select enum:one-time,persistent
  type        = string
  default     = null
}

variable "spot_launch_group" {
  description = "A launch group is a group of spot instances that launch together and terminate together"  # @group:spot
  type        = string
  default     = null
}

variable "spot_block_duration_minutes" {
  description = "The required duration for the Spot instances, in minutes"  # @group:spot
  type        = number
  default     = null
}

variable "spot_instance_interruption_behavior" {
  description = "Indicates Spot instance behavior when it is interrupted"  # @group:spot widget:select enum:hibernate,stop,terminate
  type        = string
  default     = null
}

variable "spot_valid_until" {
  description = "The end date and time of the request, in UTC RFC3339 format"  # @group:spot widget:datetime
  type        = string
  default     = null
}

variable "spot_valid_from" {
  description = "The start date and time of the request, in UTC RFC3339 format"  # @group:spot widget:datetime
  type        = string
  default     = null
}

variable "metadata_options" {
  description = "Customize the metadata options of the instance"  # @group:metadata widget:object alias:元数据选项
  type = object({
    http_endpoint               = optional(string, "enabled")  # @widget:select enum:enabled,disabled alias:HTTP端点
    http_tokens                 = optional(string, "required") # @widget:select enum:optional,required alias:IMDSv2
    http_put_response_hop_limit = optional(number, 1)          # @alias:跳数限制 min:1 max:64
    http_protocol_ipv6          = optional(string)             # @widget:select enum:enabled,disabled
    instance_metadata_tags      = optional(string)             # @widget:select enum:enabled,disabled alias:实例标签
  })
  default = {}
}

variable "maintenance_options" {
  description = "The maintenance options for the instance"  # @group:maintenance widget:object
  type = object({
    auto_recovery = optional(string)  # @widget:select enum:default,disabled alias:自动恢复
  })
  default = {}
}

variable "private_dns_name_options" {
  description = "Customize the private DNS name options of the instance"  # @group:network widget:object
  type = object({
    enable_resource_name_dns_aaaa_record = optional(bool)
    enable_resource_name_dns_a_record    = optional(bool)
    hostname_type                        = optional(string)  # @widget:select enum:ip-name,resource-name
  })
  default = {}
}

variable "launch_template" {
  description = "Specifies a Launch Template to configure the instance"  # @widget:object alias:启动模板
  type = object({
    id      = optional(string)  # @alias:模板ID
    name    = optional(string)  # @alias:模板名称
    version = optional(string)  # @alias:版本
  })
  default = {}
}

variable "instance_market_options" {
  description = "The market (purchasing) option for the instance"  # @group:spot widget:object
  type = object({
    market_type = optional(string)  # @widget:select enum:spot
    spot_options = optional(object({
      block_duration_minutes         = optional(number)
      instance_interruption_behavior = optional(string)  # @widget:select enum:hibernate,stop,terminate
      max_price                      = optional(string)
      spot_instance_type             = optional(string)  # @widget:select enum:one-time,persistent
      valid_until                    = optional(string)
    }))
  })
  default = {}
}

variable "credit_specification" {
  description = "Customize the credit specification of the instance"  # @group:compute widget:object
  type = object({
    cpu_credits = optional(string)  # @widget:select enum:standard,unlimited alias:CPU积分
  })
  default = {}
}

variable "timeouts" {
  description = "Define maximum timeout for creating, updating, and deleting EC2 instance resources"  # @hidden:true widget:object
  type = object({
    create = optional(string)
    update = optional(string)
    delete = optional(string)
  })
  default = {}
}

# Computed outputs (read-only)
variable "arn" {
  description = "The ARN of the instance"  # @computed:true
  type        = string
  default     = null
}

variable "instance_state" {
  description = "The state of the instance"  # @computed:true
  type        = string
  default     = null
}

variable "public_ip" {
  description = "The public IP address assigned to the instance"  # @computed:true alias:公网IP地址
  type        = string
  default     = null
}

variable "public_dns" {
  description = "The public DNS name assigned to the instance"  # @computed:true alias:公网DNS
  type        = string
  default     = null
}

variable "private_dns" {
  description = "The private DNS name assigned to the instance"  # @computed:true alias:私有DNS
  type        = string
  default     = null
}
