-- Provider Instances Refactor: Template as Blueprint
-- 将 provider 配置从"1:1 模板映射"改为"实例引用模板"
-- 同一模板可在同一 workspace 实例化多次，每个实例独立设置 alias 和 overrides
--
-- 幂等：全程依赖 IF EXISTS / IF NOT EXISTS，数据搬迁只在旧列还存在时执行

-- 1. 新增 provider_instances 列
ALTER TABLE public.workspaces
    ADD COLUMN IF NOT EXISTS provider_instances jsonb;

COMMENT ON COLUMN public.workspaces.provider_instances IS
    'Provider instance array: [{template_id, alias, overrides}]. Same template can be instantiated multiple times with different aliases.';

-- 2. 数据搬迁（只在旧列仍存在时跑，保证重跑幂等）
DO $$
DECLARE
    has_old_cols boolean;
    has_old_alias boolean;
BEGIN
    SELECT COUNT(*) = 2 FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'workspaces'
      AND column_name IN ('provider_template_ids', 'provider_overrides')
    INTO has_old_cols;

    SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'provider_templates'
          AND column_name = 'alias'
    ) INTO has_old_alias;

    IF NOT has_old_cols THEN
        RAISE NOTICE 'provider_template_ids/provider_overrides already dropped, skipping data migration';
        RETURN;
    END IF;

    IF has_old_alias THEN
        -- 旧 alias 列还在：workspace override alias > template-level alias > 空
        EXECUTE $mig$
            UPDATE public.workspaces ws
            SET provider_instances = sub.instances
            FROM (
                SELECT ws2.id,
                       jsonb_agg(
                           jsonb_build_object(
                               'template_id', tid::int,
                               'alias', COALESCE(
                                   (ws2.provider_overrides->tid)->>'alias',
                                   (SELECT pt.alias FROM public.provider_templates pt WHERE pt.id = tid::int),
                                   ''
                               ),
                               'overrides', COALESCE(
                                   (SELECT jsonb_object_agg(key, value)
                                    FROM jsonb_each(COALESCE((ws2.provider_overrides->tid), '{}'::jsonb))
                                    WHERE key <> 'alias'),
                                   '{}'::jsonb
                               )
                           )
                       ) AS instances
                FROM public.workspaces ws2
                CROSS JOIN LATERAL jsonb_array_elements_text(
                    COALESCE(ws2.provider_template_ids, '[]'::jsonb)
                ) AS tid
                WHERE jsonb_typeof(ws2.provider_template_ids) = 'array'
                  AND jsonb_array_length(ws2.provider_template_ids) > 0
                GROUP BY ws2.id
            ) AS sub
            WHERE ws.id = sub.id
              AND ws.provider_instances IS NULL
        $mig$;
    ELSE
        -- 旧 alias 列已删：只能用 workspace override alias
        EXECUTE $mig$
            UPDATE public.workspaces ws
            SET provider_instances = sub.instances
            FROM (
                SELECT ws2.id,
                       jsonb_agg(
                           jsonb_build_object(
                               'template_id', tid::int,
                               'alias', COALESCE((ws2.provider_overrides->tid)->>'alias', ''),
                               'overrides', COALESCE(
                                   (SELECT jsonb_object_agg(key, value)
                                    FROM jsonb_each(COALESCE((ws2.provider_overrides->tid), '{}'::jsonb))
                                    WHERE key <> 'alias'),
                                   '{}'::jsonb
                               )
                           )
                       ) AS instances
                FROM public.workspaces ws2
                CROSS JOIN LATERAL jsonb_array_elements_text(
                    COALESCE(ws2.provider_template_ids, '[]'::jsonb)
                ) AS tid
                WHERE jsonb_typeof(ws2.provider_template_ids) = 'array'
                  AND jsonb_array_length(ws2.provider_template_ids) > 0
                GROUP BY ws2.id
            ) AS sub
            WHERE ws.id = sub.id
              AND ws.provider_instances IS NULL
        $mig$;
    END IF;
END $$;

-- 3. DROP 旧列
ALTER TABLE public.workspaces DROP COLUMN IF EXISTS provider_template_ids;
ALTER TABLE public.workspaces DROP COLUMN IF EXISTS provider_overrides;

-- 4. DROP provider_templates.alias（alias 归属 workspace 实例级）
ALTER TABLE public.provider_templates DROP COLUMN IF EXISTS alias;
