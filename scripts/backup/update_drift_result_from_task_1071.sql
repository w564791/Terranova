-- 更新 workspace_drift_results 表，使用任务 1071 的数据
-- 这个脚本可以在不重新运行 drift 检测的情况下更新结果

UPDATE workspace_drift_results 
SET 
    has_drift = true, 
    drift_count = 2, 
    check_status = 'success',
    error_message = '',
    last_check_at = NOW(),
    updated_at = NOW(),
    drift_details = '{
        "check_time": "2026-01-20T23:42:00+08:00",
        "terraform_version": "",
        "plan_output_summary": "Plan: 3 to add, 3 to change.",
        "resources": [
            {
                "resource_id": 84,
                "resource_name": "ai-generated",
                "resource_type": "AWS_eks-nodegroup-exchang",
                "has_drift": true,
                "drifted_children": [
                    {
                        "address": "module.AWS_eks-nodegroup-exchang_ai-generated.module.complete[\"giypeknsix\"].module.self_managed_node_group[\"terraform-managed\"].aws_autoscaling_group.this[0]",
                        "type": "aws_autoscaling_group",
                        "name": "this",
                        "action": "update"
                    },
                    {
                        "address": "module.AWS_eks-nodegroup-exchang_ai-generated.module.complete[\"giypeknsix\"].module.self_managed_node_group[\"terraform-managed\"].aws_iam_instance_profile.this[0]",
                        "type": "aws_iam_instance_profile",
                        "name": "this",
                        "action": "update"
                    },
                    {
                        "address": "module.AWS_eks-nodegroup-exchang_ai-generated.module.complete[\"giypeknsix\"].module.self_managed_node_group[\"terraform-managed\"].aws_launch_template.this[0]",
                        "type": "aws_launch_template",
                        "name": "this",
                        "action": "update"
                    }
                ]
            },
            {
                "resource_id": 80,
                "resource_name": "ddd-64d_clone_570404",
                "resource_type": "AWS_eks-nodegroup-exchang",
                "has_drift": true,
                "drifted_children": [
                    {
                        "address": "module.AWS_eks-nodegroup-exchang_ddd-64d_clone_570404.module.complete[\"giypeknsix\"].module.self_managed_node_group[\"terraform-managed\"].aws_autoscaling_group.this[0]",
                        "type": "aws_autoscaling_group",
                        "name": "this",
                        "action": "create"
                    },
                    {
                        "address": "module.AWS_eks-nodegroup-exchang_ddd-64d_clone_570404.module.complete[\"giypeknsix\"].module.self_managed_node_group[\"terraform-managed\"].aws_iam_instance_profile.this[0]",
                        "type": "aws_iam_instance_profile",
                        "name": "this",
                        "action": "create"
                    },
                    {
                        "address": "module.AWS_eks-nodegroup-exchang_ddd-64d_clone_570404.module.complete[\"giypeknsix\"].module.self_managed_node_group[\"terraform-managed\"].aws_launch_template.this[0]",
                        "type": "aws_launch_template",
                        "name": "this",
                        "action": "create"
                    }
                ]
            },
            {
                "resource_id": 83,
                "resource_name": "ddd-64d_clone_621574",
                "resource_type": "AWS_eks-nodegroup-exchang",
                "has_drift": false,
                "drifted_children": null
            }
        ]
    }'::jsonb
WHERE workspace_id = 'ws-5o7movp0e7';

-- 同时更新 workspace 表的统计字段
UPDATE workspaces 
SET 
    drift_count = 2,
    last_drift_check = NOW()
WHERE workspace_id = 'ws-5o7movp0e7';

-- 验证结果
SELECT workspace_id, has_drift, drift_count, check_status, last_check_at 
FROM workspace_drift_results 
WHERE workspace_id = 'ws-5o7movp0e7';
