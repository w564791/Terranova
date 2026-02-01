-- 修复 S3 模块的 lifecycle_rule Schema 定义
-- 问题：lifecycle_rule 的 items.properties 为空，导致无法渲染嵌套字段

-- 更新 module_id = 54 的 Schema
UPDATE schemas
SET openapi_schema = jsonb_set(
  openapi_schema,
  '{components,schemas,ModuleInput,properties,lifecycle_rule}',
  '{
    "type": "array",
    "items": {
      "type": "object",
      "properties": {
        "id": {
          "type": "string",
          "title": "ID",
          "description": "Unique identifier for the rule. The value cannot be longer than 255 characters."
        },
        "enabled": {
          "type": "boolean",
          "title": "启用",
          "default": true,
          "description": "Whether enable this lifecycle rule"
        },
        "filter": {
          "type": "object",
          "title": "过滤器",
          "description": "Configuration block used to identify objects that a Lifecycle Rule applies to.",
          "properties": {
            "prefix": {
              "type": "string",
              "title": "前缀",
              "description": "Prefix identifying one or more objects to which the rule applies."
            },
            "object_size_greater_than": {
              "type": "integer",
              "title": "对象大小大于",
              "description": "Minimum object size (in bytes) to which the rule applies."
            },
            "object_size_less_than": {
              "type": "integer",
              "title": "对象大小小于",
              "description": "Maximum object size (in bytes) to which the rule applies."
            },
            "tags": {
              "type": "object",
              "title": "标签",
              "description": "Configuration block for specifying a tag key and value.",
              "additionalProperties": {
                "type": "string"
              }
            }
          }
        },
        "expiration": {
          "type": "object",
          "title": "过期",
          "description": "Configuration block that specifies the expiration for the lifecycle of the object.",
          "properties": {
            "days": {
              "type": "integer",
              "title": "过期天数",
              "default": 30,
              "description": "Lifetime, in days, of the objects that are subject to the rule."
            },
            "date": {
              "type": "string",
              "title": "过期日期",
              "description": "Date the object is to be moved or deleted. The date value must be in RFC3339 full-date format e.g. 2023-08-22."
            },
            "expired_object_delete_marker": {
              "type": "boolean",
              "title": "过期对象删除标记",
              "description": "Indicates whether Amazon S3 will remove a delete marker with no noncurrent versions."
            }
          }
        },
        "transition": {
          "type": "array",
          "title": "内容转换",
          "description": "The transition configuration block supports the following arguments.",
          "items": {
            "type": "object",
            "properties": {
              "days": {
                "type": "integer",
                "title": "转换天数",
                "default": 30,
                "description": "Number of days after creation when objects are transitioned to the specified storage class."
              },
              "storage_class": {
                "type": "string",
                "title": "转换存储类型",
                "default": "STANDARD_IA",
                "description": "Class of storage used to store the object. Valid Values: GLACIER, STANDARD_IA, ONEZONE_IA, INTELLIGENT_TIERING, DEEP_ARCHIVE, GLACIER_IR."
              }
            }
          }
        },
        "abort_incomplete_multipart_upload_days": {
          "type": "integer",
          "title": "中止不完整分段上传天数",
          "default": 7,
          "description": "Configuration block that specifies the days since the initiation of an incomplete multipart upload."
        },
        "noncurrent_version_expiration": {
          "type": "object",
          "title": "非当前版本过期",
          "description": "Configuration block that specifies when noncurrent object versions expire.",
          "properties": {
            "days": {
              "type": "integer",
              "title": "转换天数",
              "default": 30,
              "description": "Number of days an object is noncurrent before Amazon S3 can perform the associated action."
            },
            "newer_noncurrent_versions": {
              "type": "integer",
              "title": "保留的非当前版本数",
              "default": 2,
              "description": "Number of noncurrent versions Amazon S3 will retain."
            }
          }
        },
        "noncurrent_version_transition": {
          "type": "array",
          "title": "非当前版本转换",
          "description": "Set of configuration blocks that specify the transition rule for noncurrent objects.",
          "items": {
            "type": "object",
            "properties": {
              "days": {
                "type": "integer",
                "title": "转换天数",
                "default": 30,
                "description": "Number of days an object is noncurrent before Amazon S3 can perform the associated action."
              },
              "storage_class": {
                "type": "string",
                "title": "内容存储类型",
                "default": "STANDARD_IA",
                "description": "Class of storage used to store the object. Valid Values: GLACIER, STANDARD_IA, ONEZONE_IA, INTELLIGENT_TIERING, DEEP_ARCHIVE, GLACIER_IR."
              },
              "newer_noncurrent_versions": {
                "type": "integer",
                "title": "保留的非当前版本数",
                "default": 2,
                "description": "Number of noncurrent versions Amazon S3 will retain."
              }
            }
          }
        }
      }
    },
    "title": "Lifecycle Rule",
    "description": "List of maps containing configuration of object lifecycle management."
  }'::jsonb
)
WHERE module_id = 54 AND schema_version = 'v2';

-- 验证更新结果
SELECT 
  id,
  module_id,
  openapi_schema->'components'->'schemas'->'ModuleInput'->'properties'->'lifecycle_rule' as lifecycle_rule_schema
FROM schemas 
WHERE module_id = 54 AND schema_version = 'v2';
