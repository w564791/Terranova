package demo

func GetS3ModuleSchema() S3ModuleSchema {
	return S3ModuleSchema{
		Name:         "s3",
		Provider:     "kucoin",
		ResourceType: "module",
		Source:       "tfe-applications.kcprd.com/default/s3/kucoin",
		TopLevelKeys: []string{},
		Schema: S3Module{
			// Bucket Configuration
			Name: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.ForceNew = true
				s.Default = nil
				s.HiddenDefault = false
				s.Description = "Forces new resource,The name of the bucket. If omitted, Terraform will be destroy and re build resources."
				return s
			}(),
			BucketPrefix: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.ForceNew = true
				s.Default = nil
				s.Description = "Forces new resource, Creates a unique bucket name beginning with the specified prefix. Conflicts with bucket."
				return s
			}(),
			ACL: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = nil
				s.Description = "(Optional) The canned ACL to apply. Conflicts with `grant`"
				s.AtLeastOneOf = []string{"private", "public-read", "public-read-write", "aws-exec-read", "authenticated-read", "bucket-owner-read", "bucket-owner-full-control", "log-delivery-write"}
				return s
			}(),
			Policy: func() Schema {
				s := defaultSchema()
				s.Type = TypeJsonString
				s.Required = false
				s.Default = nil
				s.Description = "(Optional) A valid bucket policy JSON document."
				return s
			}(),
			ForceDestroy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "(Optional, Default:false ) A boolean that indicates all objects should be deleted from the bucket so that the bucket can be destroyed without error. These objects are not recoverable."
				return s
			}(),

			// Policy Attachment Configuration
			AttachELBLogDeliveryPolicy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Controls if S3 bucket should have ELB log delivery policy attached"
				return s
			}(),
			AttachLBLogDeliveryPolicy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Controls if S3 bucket should have ALB/NLB log delivery policy attached"
				return s
			}(),
			AttachAccessLogDeliveryPolicy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Controls if S3 bucket should have S3 access log delivery policy attached"
				return s
			}(),
			AttachCloudtrailLogDeliveryPolicy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Controls if S3 bucket should have CloudTrail log delivery policy attached"
				return s
			}(),
			AttachDenyInsecureTransportPolicy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Controls if S3 bucket should have deny non-SSL transport policy attached"
				return s
			}(),
			AttachRequireLatestTLSPolicy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Controls if S3 bucket should require the latest version of TLS"
				return s
			}(),
			AttachPolicy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Controls if S3 bucket should have bucket policy attached (set to `true` to use value of `policy` as bucket policy)"
				return s
			}(),
			AttachPublicPolicy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = true
				s.Description = "Controls if a user defined public bucket policy will be attached (set to `false` to allow upstream to apply defaults to the bucket)"
				return s
			}(),
			AttachInventoryDestinationPolicy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Controls if S3 bucket should have bucket inventory destination policy attached."
				return s
			}(),
			AttachAnalyticsDestinationPolicy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Controls if S3 bucket should have bucket analytics destination policy attached."
				return s
			}(),
			AttachDenyIncorrectEncryptionHeaders: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Controls if S3 bucket should deny incorrect encryption headers policy attached."
				return s
			}(),
			AttachDenyIncorrectKMSKeySSE: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Controls if S3 bucket policy should deny usage of incorrect KMS key SSE."
				return s
			}(),
			AttachDenyUnencryptedObjectUploads: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Controls if S3 bucket should deny unencrypted object uploads policy attached."
				return s
			}(),
			AttachDenySSECEncryptedObjectUploads: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Controls if S3 bucket should deny SSEC encrypted object uploads."
				return s
			}(),
			AttachWAFLogDeliveryPolicy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Controls if S3 bucket should have WAF log delivery policy attached"
				return s
			}(),

			// Encryption Configuration
			AllowedKMSKeyArn: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = nil
				s.Description = "The ARN of KMS key which should be allowed in PutObject"
				return s
			}(),

			// Tags
			Tags: Schema{
				Type:          TypeMap,
				Required:      true,
				HiddenDefault: false,
				MustInclude: []string{
					"business-line",
					"managed-by",
				},
			},
			DefaultTags: func() Schema {
				s := defaultSchema()
				s.Type = TypeMap
				s.Required = false
				s.Default = map[string]interface{}{
					"managed-by":           "ken",
					"business-line":        "ops",
					"managed-by-terraform": "true",
				}
				s.Description = "The default tags for s3 bucket"
				s.Elem = Schema{
					Type: TypeString,
				}
				return s
			}(),

			// Advanced Configuration
			AccelerationStatus: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = nil
				s.Description = "(Optional) Sets the accelerate configuration of an existing bucket. Can be Enabled or Suspended."
				s.AtLeastOneOf = []string{"Enabled", "Suspended"}
				return s
			}(),
			RequestPayer: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = nil
				s.Description = "(Optional) Specifies who should bear the cost of Amazon S3 data transfer. Can be either BucketOwner or Requester."
				s.AtLeastOneOf = []string{"BucketOwner", "Requester"}
				return s
			}(),

			// Website Configuration
			Website: func() Schema {
				s := defaultSchema()
				s.Type = TypeMap
				s.Required = false
				s.Default = map[string]interface{}{}
				s.Description = "Map containing static web-site hosting or redirect configuration."
				return s
			}(),

			// CORS Configuration
			CORSRule: func() Schema {
				s := defaultSchema()
				s.Type = TypeList
				s.Required = false
				s.Default = []interface{}{}
				s.Description = "List of maps containing rules for Cross-Origin Resource Sharing."
				return s
			}(),

			// Versioning Configuration
			Versioning: func() Schema {
				s := defaultSchema()
				s.Type = TypeMap
				s.Required = false
				s.Default = map[string]interface{}{}
				s.Description = "Map containing versioning configuration."
				s.Elem = Schema{
					Type: TypeString,
				}
				return s
			}(),

			// Logging Configuration
			Logging: func() Schema {
				s := defaultSchema()
				s.Type = TypeMap
				s.Required = false
				s.Default = map[string]interface{}{}
				s.Description = "Map containing access bucket logging configuration."
				return s
			}(),

			// Access Log Delivery Configuration
			AccessLogDeliveryPolicySourceBuckets: func() Schema {
				s := defaultSchema()
				s.Type = TypeList
				s.Required = false
				s.Default = []interface{}{}
				s.Description = "(Optional) List of S3 bucket ARNs which should be allowed to deliver access logs to this bucket."
				s.Elem = Schema{
					Type: TypeString,
				}
				return s
			}(),
			AccessLogDeliveryPolicySourceAccounts: func() Schema {
				s := defaultSchema()
				s.Type = TypeList
				s.Required = false
				s.Default = []interface{}{}
				s.Description = "(Optional) List of AWS Account IDs should be allowed to deliver access logs to this bucket."
				s.Elem = Schema{
					Type: TypeString,
				}
				return s
			}(),
			AccessLogDeliveryPolicySourceOrganizations: func() Schema {
				s := defaultSchema()
				s.Type = TypeList
				s.Required = false
				s.Default = []interface{}{}
				s.Description = "(Optional) List of AWS Organization IDs should be allowed to deliver access logs to this bucket."
				s.Elem = Schema{
					Type: TypeString,
				}
				return s
			}(),
			LBLogDeliveryPolicySourceOrganizations: func() Schema {
				s := defaultSchema()
				s.Type = TypeList
				s.Required = false
				s.Default = []interface{}{}
				s.Description = "(Optional) List of AWS Organization IDs should be allowed to deliver ALB/NLB logs to this bucket."
				s.Elem = Schema{
					Type: TypeString,
				}
				return s
			}(),

			// ACL Configuration
			Grant: func() Schema {
				s := defaultSchema()
				s.Type = TypeList
				s.Required = false
				s.Default = []interface{}{}
				s.Description = "An ACL policy grant. Conflicts with `acl`"
				return s
			}(),
			Owner: func() Schema {
				s := defaultSchema()
				s.Type = TypeMap
				s.Required = false
				s.Default = map[string]interface{}{}
				s.Description = "Bucket owner's display name and ID. Conflicts with `acl`"
				s.Elem = Schema{
					Type: TypeString,
				}
				return s
			}(),
			ExpectedBucketOwner: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = nil
				s.Description = "The account ID of the expected bucket owner"
				return s
			}(),

			// Lifecycle Configuration
			TransitionDefaultMinimumObjectSize: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = "all_storage_classes_128K"
				s.Description = "The default minimum object size behavior applied to the lifecycle configuration. Valid values: all_storage_classes_128K (default), varies_by_storage_class"
				s.AtLeastOneOf = []string{"all_storage_classes_128K", "varies_by_storage_class"}
				return s
			}(),
			LifecycleRule: func() Schema {
				s := defaultSchema()
				s.Type = TypeListObject
				s.Required = false
				s.Default = []map[string]interface{}{}
				s.Description = `<p>Provides an independent configuration resource for <a href="https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lifecycle-mgmt.html" target="_blank">S3 bucket lifecycle configuration</a>.</p>`

				s.Elem = map[string]Schema{
					"enabled": func() Schema {
						s := defaultSchema()
						s.Type = TypeBool
						s.Default = true
						s.HiddenDefault = false
						s.Required = true
						s.Description = "whether enable this lifecycle rule"
						return s
					}(),
					"filter": func() Schema {
						s := defaultSchema()
						s.Type = TypeObject
						s.HiddenDefault = true
						s.Required = false
						s.Description = `<p>Configuration block used to identify objects that a Lifecycle Rule applies to. <a href="https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_lifecycle_configuration#filter" target="_blank">See below</a>. If not specified, the rule will default to using prefix. One of filter or prefix should be specified.</p>`

						s.Elem = map[string]Schema{
							"object_size_greater_than": func() Schema {
								s := defaultSchema()
								s.Type = TypeInt
								s.Description = "Minimum object size (in bytes) to which the rule applies."
								return s
							}(),
							"object_size_less_than": func() Schema {
								s := defaultSchema()
								s.Type = TypeInt
								s.Description = "Maximum object size (in bytes) to which the rule applies."
								return s
							}(),
							"prefix": func() Schema {
								s := defaultSchema()
								s.Type = TypeString
								s.ForceNew = true
								s.Default = ""
								s.Color = WarningColor
								s.HiddenDefault = false
								s.Description = `Prefix identifying one or more objects to which the rule applies. Defaults to an empty string ("") if not specified.`
								return s
							}(),
							"tags": func() Schema {
								s := defaultSchema()
								s.Type = TypeMap
								s.Default = map[string]string{}
								s.Description = `<p>Configuration block for specifying a tag key and value. <a href="https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_lifecycle_configuration#tag" target="_blank">See below</a>.</p>`

								return s
							}(),
						}
						return s
					}(),
					"id": func() Schema {
						s := defaultSchema()
						s.HiddenDefault = false
						s.Type = TypeString
						s.Default = ""
						s.Required = true
						s.Description = "Unique identifier for the rule. The value cannot be longer than 255 characters."
						return s
					}(),
					"expiration": func() Schema {
						s := defaultSchema()
						s.HiddenDefault = false
						s.Type = TypeListObject
						s.Default = []map[string]interface{}{}
						s.Required = false
						s.Description = `<p>Configuration block that specifies the expiration for the lifecycle of the object in the form of date, days and, whether the object has a delete marker. <a href="https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_lifecycle_configuration#expiration" target="_blank">See below</a></p>`

						s.Elem = map[string]Schema{
							"days": func() Schema {
								s := defaultSchema()
								s.Type = TypeInt
								s.Default = 30
								s.HiddenDefault = false
								s.Required = true
								s.Description = "Lifetime, in days, of the objects that are subject to the rule. The value must be a non-zero positive integer."
								return s
							}(),
							"date": func() Schema {
								s := defaultSchema()
								s.Type = TypeString
								s.Default = ""
								s.HiddenDefault = false
								s.Required = true
								s.Description = "Date the object is to be moved or deleted. The date value must be in RFC3339 full-date format e.g. 2023-08-22."
								return s
							}(),
							"expired_object_delete_marker": func() Schema {
								s := defaultSchema()
								s.Type = TypeBool
								s.Default = nil
								s.HiddenDefault = true
								s.Required = false
								s.Description = "Indicates whether Amazon S3 will remove a delete marker with no noncurrent versions. If set to true, the delete marker will be expired; if set to false the policy takes no action."
								return s
							}(),
						}
						return s
					}(),
					"transition": func() Schema {
						s := defaultSchema()
						s.Type = TypeListObject
						s.HiddenDefault = true
						s.Default = []map[string]interface{}{
							{
								"days":          30,
								"storage_class": "ONEZONE_IA",
							},
							{
								"days":          60,
								"storage_class": "GLACIER",
							},
						}
						s.Required = false
						s.Description = "The transition configuration block supports the following arguments. Note: Only one of date or days should be specified. If neither are specified, the transition will default to 0 days."
						s.Elem = map[string]Schema{
							"days": func() Schema {
								s := defaultSchema()
								s.Type = TypeInt
								s.Default = 30
								s.HiddenDefault = false
								s.Required = true
								s.Description = `<p>Conflicts with date. Number of days after creation when objects are transitioned to the specified storage class. The value must be a positive integer. If both days and date are not specified, defaults to 0. Valid values depend on storage_class, see <a href="https://docs.aws.amazon.com/AmazonS3/latest/userguide/lifecycle-transition-general-considerations.html" target="_blank">Transition objects using Amazon S3 Lifecycle</a> for more details.</p>`

								return s
							}(),
							"storage_class": func() Schema {
								s := defaultSchema()
								s.Type = TypeString
								s.ForceNew = true
								s.Default = "STANDARD_IA"
								s.Color = WarningColor
								s.HiddenDefault = false
								s.Required = true
								s.Description = "Class of storage used to store the object. Valid Values: GLACIER, STANDARD_IA, ONEZONE_IA, INTELLIGENT_TIERING, DEEP_ARCHIVE, GLACIER_IR."
								return s
							}(),
						}
						return s
					}(),
					"abort_incomplete_multipart_upload_days": func() Schema {
						s := defaultSchema()
						s.Type = TypeInt
						s.HiddenDefault = true
						s.Default = 7
						s.Required = false
						s.Description = `<p>Configuration block that specifies the days since the initiation of an incomplete multipart upload that Amazon S3 will wait before permanently removing all parts of the upload. <a href="https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_lifecycle_configuration#abort_incomplete_multipart_upload" target="_blank">See below</a>.</p>`

						return s
					}(),
					"noncurrent_version_expiration": func() Schema {
						s := defaultSchema()
						s.HiddenDefault = false
						s.Type = TypeListObject
						s.Default = []map[string]interface{}{}
						s.Required = false
						s.Description = `<p>Configuration block that specifies when noncurrent object versions expire. <a href="https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_lifecycle_configuration#noncurrent_version_expiration" target="_blank">See below</a>.</p>`

						s.Elem = map[string]Schema{
							"days": func() Schema {
								s := defaultSchema()
								s.Type = TypeInt
								s.Default = 30
								s.HiddenDefault = false
								s.Required = true
								s.Description = "Number of days an object is noncurrent before Amazon S3 can perform the associated action."
								return s
							}(),
							"newer_noncurrent_versions": func() Schema {
								s := defaultSchema()
								s.Type = TypeInt
								s.Default = 2
								s.HiddenDefault = false
								s.Required = true
								s.Description = "Number of noncurrent versions Amazon S3 will retain. Must be a non-zero positive integer."
								return s
							}(),
						}
						return s
					}(),
					"noncurrent_version_transition": func() Schema {
						s := defaultSchema()
						s.Type = TypeListObject
						s.Default = []map[string]interface{}{}
						s.HiddenDefault = true
						s.Description = `<p>Set of configuration blocks that specify the transition rule for the lifecycle rule that describes when noncurrent objects transition to a specific storage class. <a href="https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_lifecycle_configuration#noncurrent_version_transition" target="_blank">See below</a>.</p>`

						s.Elem = map[string]Schema{
							"days": func() Schema {
								s := defaultSchema()
								s.Type = TypeInt
								s.Default = 30
								s.HiddenDefault = false
								s.Required = true
								s.Description = "Number of days an object is noncurrent before Amazon S3 can perform the associated action."
								return s
							}(),
							"storage_class": func() Schema {
								s := defaultSchema()
								s.Type = TypeString
								s.ForceNew = true
								s.Default = "STANDARD_IA"
								s.Color = WarningColor
								s.HiddenDefault = false
								s.Required = true
								s.Description = "Class of storage used to store the object. Valid Values: GLACIER, STANDARD_IA, ONEZONE_IA, INTELLIGENT_TIERING, DEEP_ARCHIVE, GLACIER_IR."
								return s
							}(),
							"newer_noncurrent_versions": func() Schema {
								s := defaultSchema()
								s.Type = TypeInt
								s.Default = 2
								s.HiddenDefault = false
								s.Required = true
								s.Description = "Number of noncurrent versions Amazon S3 will retain. Must be a non-zero positive integer."
								return s
							}(),
						}
						return s
					}(),
				}
				return s
			}(),

			// Replication Configuration
			ReplicationConfiguration: func() Schema {
				s := defaultSchema()
				s.Type = TypeMap
				s.Required = false
				s.Default = map[string]interface{}{}
				s.Description = "Map containing cross-region replication configuration."
				return s
			}(),

			// Server Side Encryption Configuration
			ServerSideEncryptionConfiguration: func() Schema {
				s := defaultSchema()
				s.Type = TypeMap
				s.Required = false
				s.Default = map[string]interface{}{}
				s.Description = "Map containing server-side encryption configuration."
				return s
			}(),

			// Intelligent Tiering Configuration
			IntelligentTiering: func() Schema {
				s := defaultSchema()
				s.Type = TypeMap
				s.Required = false
				s.Default = map[string]interface{}{}
				s.Description = "Map containing intelligent tiering configuration."
				return s
			}(),

			// Object Lock Configuration
			ObjectLockConfiguration: func() Schema {
				s := defaultSchema()
				s.Type = TypeMap
				s.Required = false
				s.Default = map[string]interface{}{}
				s.Description = "Map containing S3 object locking configuration."
				return s
			}(),
			ObjectLockEnabled: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Whether S3 bucket should have an Object Lock configuration enabled."
				return s
			}(),

			// Metrics Configuration
			MetricConfiguration: func() Schema {
				s := defaultSchema()
				s.Type = TypeList
				s.Required = false
				s.Default = []interface{}{}
				s.Description = "Map containing bucket metric configuration."
				return s
			}(),

			// Inventory Configuration
			InventoryConfiguration: func() Schema {
				s := defaultSchema()
				s.Type = TypeMap
				s.Required = false
				s.Default = map[string]interface{}{}
				s.Description = "Map containing S3 inventory configuration."
				return s
			}(),
			InventorySourceAccountID: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = nil
				s.Description = "The inventory source account id."
				return s
			}(),
			InventorySourceBucketArn: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = nil
				s.Description = "The inventory source bucket ARN."
				return s
			}(),
			InventorySelfSourceDestination: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Whether or not the inventory source bucket is also the destination bucket."
				return s
			}(),

			// Analytics Configuration
			AnalyticsConfiguration: func() Schema {
				s := defaultSchema()
				s.Type = TypeMap
				s.Required = false
				s.Default = map[string]interface{}{}
				s.Description = "Map containing bucket analytics configuration."
				return s
			}(),
			AnalyticsSourceAccountID: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = nil
				s.Description = "The analytics source account id."
				return s
			}(),
			AnalyticsSourceBucketArn: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = nil
				s.Description = "The analytics source bucket ARN."
				return s
			}(),
			AnalyticsSelfSourceDestination: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Whether or not the analytics source bucket is also the destination bucket."
				return s
			}(),

			// Public Access Block Configuration
			BlockPublicACLs: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = true
				s.Description = "Whether Amazon S3 should block public ACLs for this bucket."
				return s
			}(),
			BlockPublicPolicy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = true
				s.Description = "Whether Amazon S3 should block public bucket policies for this bucket."
				return s
			}(),
			IgnorePublicACLs: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = true
				s.Description = "Whether Amazon S3 should ignore public ACLs for this bucket."
				return s
			}(),
			RestrictPublicBuckets: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = true
				s.Description = "Whether Amazon S3 should restrict public bucket policies for this bucket."
				return s
			}(),

			// Object Ownership Configuration
			ControlObjectOwnership: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Whether to manage S3 Bucket Ownership Controls on this bucket."
				return s
			}(),
			ObjectOwnership: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = "BucketOwnerEnforced"
				s.Description = "Object ownership. Valid values: BucketOwnerEnforced, BucketOwnerPreferred or ObjectWriter."
				s.AtLeastOneOf = []string{"BucketOwnerEnforced", "BucketOwnerPreferred", "ObjectWriter"}
				return s
			}(),

			// Directory Bucket Configuration
			IsDirectoryBucket: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "If the s3 bucket created is a directory bucket"
				return s
			}(),
			DataRedundancy: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = nil
				s.Description = "Data redundancy. Valid values: `SingleAvailabilityZone`"
				s.AtLeastOneOf = []string{"SingleAvailabilityZone"}
				return s
			}(),
			Type: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = "Directory"
				s.Description = "Bucket type. Valid values: `Directory`"
				s.AtLeastOneOf = []string{"Directory"}
				return s
			}(),
			AvailabilityZoneID: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = nil
				s.Description = "Availability Zone ID or Local Zone ID"
				return s
			}(),
			LocationType: func() Schema {
				s := defaultSchema()
				s.Type = TypeString
				s.Required = false
				s.Default = nil
				s.Description = "Location type. Valid values: `AvailabilityZone` or `LocalZone`"
				s.AtLeastOneOf = []string{"AvailabilityZone", "LocalZone"}
				return s
			}(),

			// S3 Bucket Notification Configuration
			CreateNotification: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = false
				s.Description = "Whether to create S3 bucket notification resource"
				return s
			}(),
			CreateSNSPolicy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = true
				s.Description = "Whether to create a policy for SNS permissions or not?"
				return s
			}(),
			CreateSQSPolicy: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = true
				s.Description = "Whether to create a policy for SQS permissions or not?"
				return s
			}(),
			CreateLambdaPermission: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = true
				s.Description = "Whether to create Lambda permissions or not?"
				return s
			}(),
			EventBridge: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = nil
				s.Description = "Whether to enable Amazon EventBridge notifications"
				return s
			}(),
			LambdaNotifications: func() Schema {
				s := defaultSchema()
				s.Type = TypeMap
				s.Required = false
				s.Default = map[string]interface{}{}
				s.Description = "Map of S3 bucket notifications to Lambda function"
				return s
			}(),
			SQSNotifications: func() Schema {
				s := defaultSchema()
				s.Type = TypeMap
				s.Required = false
				s.Default = map[string]interface{}{}
				s.Description = "Map of S3 bucket notifications to SQS queue"
				return s
			}(),
			SNSNotifications: func() Schema {
				s := defaultSchema()
				s.Type = TypeMap
				s.Required = false
				s.Default = map[string]interface{}{}
				s.Description = "Map of S3 bucket notifications to SNS topic"
				return s
			}(),

			// Easter Egg
			PutinKhuylo: func() Schema {
				s := defaultSchema()
				s.Type = TypeBool
				s.Required = false
				s.Default = true
				s.Description = "Do you agree that Putin doesn't respect Ukrainian sovereignty and territorial integrity? More info: https://en.wikipedia.org/wiki/Putin_khuylo!"
				return s
			}(),
		},
	}
}

type S3ModuleSchema struct {
	ResourceType string   `json:"resource_type"`
	Provider     string   `json:"provider"`
	Name         string   `json:"name"`
	TopLevelKeys []string `json:"top_level_keys"`
	Schema       S3Module `json:"schema"`
	Source       string   `json:"source"`
}

type S3Module struct {

	// Bucket Configuration
	Name         Schema `json:"name"`
	BucketPrefix Schema `json:"bucket_prefix"`
	ACL          Schema `json:"acl"`
	Policy       Schema `json:"policy"`
	ForceDestroy Schema `json:"force_destroy"`

	// Policy Attachment Configuration
	AttachELBLogDeliveryPolicy           Schema `json:"attach_elb_log_delivery_policy"`
	AttachLBLogDeliveryPolicy            Schema `json:"attach_lb_log_delivery_policy"`
	AttachAccessLogDeliveryPolicy        Schema `json:"attach_access_log_delivery_policy"`
	AttachCloudtrailLogDeliveryPolicy    Schema `json:"attach_cloudtrail_log_delivery_policy"`
	AttachDenyInsecureTransportPolicy    Schema `json:"attach_deny_insecure_transport_policy"`
	AttachRequireLatestTLSPolicy         Schema `json:"attach_require_latest_tls_policy"`
	AttachPolicy                         Schema `json:"attach_policy"`
	AttachPublicPolicy                   Schema `json:"attach_public_policy"`
	AttachInventoryDestinationPolicy     Schema `json:"attach_inventory_destination_policy"`
	AttachAnalyticsDestinationPolicy     Schema `json:"attach_analytics_destination_policy"`
	AttachDenyIncorrectEncryptionHeaders Schema `json:"attach_deny_incorrect_encryption_headers"`
	AttachDenyIncorrectKMSKeySSE         Schema `json:"attach_deny_incorrect_kms_key_sse"`
	AttachDenyUnencryptedObjectUploads   Schema `json:"attach_deny_unencrypted_object_uploads"`
	AttachDenySSECEncryptedObjectUploads Schema `json:"attach_deny_ssec_encrypted_object_uploads"`
	AttachWAFLogDeliveryPolicy           Schema `json:"attach_waf_log_delivery_policy"`

	// Encryption Configuration
	AllowedKMSKeyArn Schema `json:"allowed_kms_key_arn"`

	// Tags
	Tags        Schema `json:"tags"`
	DefaultTags Schema `json:"default_tags"`

	// Advanced Configuration
	AccelerationStatus Schema `json:"acceleration_status"`
	RequestPayer       Schema `json:"request_payer"`

	// Website Configuration
	Website Schema `json:"website"`

	// CORS Configuration
	CORSRule Schema `json:"cors_rule"`

	// Versioning Configuration
	Versioning Schema `json:"versioning"`

	// Logging Configuration
	Logging Schema `json:"logging"`

	// Access Log Delivery Configuration
	AccessLogDeliveryPolicySourceBuckets       Schema `json:"access_log_delivery_policy_source_buckets"`
	AccessLogDeliveryPolicySourceAccounts      Schema `json:"access_log_delivery_policy_source_accounts"`
	AccessLogDeliveryPolicySourceOrganizations Schema `json:"access_log_delivery_policy_source_organizations"`
	LBLogDeliveryPolicySourceOrganizations     Schema `json:"lb_log_delivery_policy_source_organizations"`

	// ACL Configuration
	Grant               Schema `json:"grant"`
	Owner               Schema `json:"owner"`
	ExpectedBucketOwner Schema `json:"expected_bucket_owner"`

	// Lifecycle Configuration
	TransitionDefaultMinimumObjectSize Schema `json:"transition_default_minimum_object_size"`
	LifecycleRule                      Schema `json:"lifecycle_rule"`

	// Replication Configuration
	ReplicationConfiguration Schema `json:"replication_configuration"`

	// Server Side Encryption Configuration
	ServerSideEncryptionConfiguration Schema `json:"server_side_encryption_configuration"`

	// Intelligent Tiering Configuration
	IntelligentTiering Schema `json:"intelligent_tiering"`

	// Object Lock Configuration
	ObjectLockConfiguration Schema `json:"object_lock_configuration"`
	ObjectLockEnabled       Schema `json:"object_lock_enabled"`

	// Metrics Configuration
	MetricConfiguration Schema `json:"metric_configuration"`

	// Inventory Configuration
	InventoryConfiguration         Schema `json:"inventory_configuration"`
	InventorySourceAccountID       Schema `json:"inventory_source_account_id"`
	InventorySourceBucketArn       Schema `json:"inventory_source_bucket_arn"`
	InventorySelfSourceDestination Schema `json:"inventory_self_source_destination"`

	// Analytics Configuration
	AnalyticsConfiguration         Schema `json:"analytics_configuration"`
	AnalyticsSourceAccountID       Schema `json:"analytics_source_account_id"`
	AnalyticsSourceBucketArn       Schema `json:"analytics_source_bucket_arn"`
	AnalyticsSelfSourceDestination Schema `json:"analytics_self_source_destination"`

	// Public Access Block Configuration
	BlockPublicACLs       Schema `json:"block_public_acls"`
	BlockPublicPolicy     Schema `json:"block_public_policy"`
	IgnorePublicACLs      Schema `json:"ignore_public_acls"`
	RestrictPublicBuckets Schema `json:"restrict_public_buckets"`

	// Object Ownership Configuration
	ControlObjectOwnership Schema `json:"control_object_ownership"`
	ObjectOwnership        Schema `json:"object_ownership"`

	// Directory Bucket Configuration
	IsDirectoryBucket  Schema `json:"is_directory_bucket"`
	DataRedundancy     Schema `json:"data_redundancy"`
	Type               Schema `json:"type"`
	AvailabilityZoneID Schema `json:"availability_zone_id"`
	LocationType       Schema `json:"location_type"`

	// S3 Bucket Notification Configuration
	CreateNotification     Schema `json:"create_notification"`
	CreateSNSPolicy        Schema `json:"create_sns_policy"`
	CreateSQSPolicy        Schema `json:"create_sqs_policy"`
	CreateLambdaPermission Schema `json:"create_lambda_permission"`
	EventBridge            Schema `json:"eventbridge"`
	LambdaNotifications    Schema `json:"lambda_notifications"`
	SQSNotifications       Schema `json:"sqs_notifications"`
	SNSNotifications       Schema `json:"sns_notifications"`

	// Easter Egg
	PutinKhuylo Schema `json:"putin_khuylo"`
}

func defaultSchema() Schema {
	return Schema{
		Type:                  TypeString, // 示例默认类型
		Required:              false,
		Computed:              false,
		ForceNew:              false,
		DiffSuppressOnRefresh: false,
		Default:               nil,
		Description:           "",
		InputDefault:          "",
		Elem:                  nil,
		MaxItems:              0,
		MaxValue:              0,
		MinItems:              0,
		MinValue:              0,
		ComputedWhen:          nil,
		ConflictsWith:         nil,
		ExactlyOneOf:          nil,
		AtLeastOneOf:          nil,
		RequiredWith:          nil,
		Deprecated:            "",
		Sensitive:             false,
		WriteOnly:             false,
		MustInclude:           nil,
		UniqItems:             false,
		Color:                 InfoColor,
		HiddenDefault:         true,
	}
}
