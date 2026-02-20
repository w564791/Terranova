-- 修复 muti_az_instances_location 字段的 Schema 定义
-- 问题：additionalProperties.type 是 "string"，应该是 "object"
-- 问题：widget 是 "key-value"，应该是 "dynamic-object"

-- 查看当前 Schema
SELECT 
    id,
    module_id,
    openapi_schema->'components'->'schemas'->'ModuleInput'->'properties'->'muti_az_instances_location' as field_schema,
    openapi_schema->'x-iac-platform'->'ui'->'fields'->'muti_az_instances_location' as ui_config
FROM schemas 
WHERE module_id = 55 AND schema_version = 'v2';

-- 更新 Schema 定义
UPDATE schemas
SET openapi_schema = jsonb_set(
    jsonb_set(
        jsonb_set(
            openapi_schema,
            '{components,schemas,ModuleInput,properties,muti_az_instances_location}',
            '{
                "type": "object",
                "title": "Muti Az Instances Location",
                "description": "多可用区实例位置配置",
                "x-dynamic-keys": true,
                "additionalProperties": {
                    "type": "object",
                    "properties": {
                        "group": {
                            "type": "string",
                            "title": "Group",
                            "description": "实例组"
                        },
                        "zone": {
                            "type": "string",
                            "title": "Zone",
                            "description": "可用区"
                        }
                    }
                }
            }'::jsonb
        ),
        '{x-iac-platform,ui,fields,muti_az_instances_location,widget}',
        '"dynamic-object"'
    ),
    '{x-iac-platform,ui,fields,muti_az_instances_location,group}',
    '"basic"'
)
WHERE module_id = 55 AND schema_version = 'v2';

-- 验证更新结果
SELECT 
    id,
    module_id,
    openapi_schema->'components'->'schemas'->'ModuleInput'->'properties'->'muti_az_instances_location' as field_schema,
    openapi_schema->'x-iac-platform'->'ui'->'fields'->'muti_az_instances_location' as ui_config
FROM schemas 
WHERE module_id = 55 AND schema_version = 'v2';
