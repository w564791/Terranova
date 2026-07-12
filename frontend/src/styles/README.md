# Design System — Colors & UI Primitives

Source of truth: `demo/button.html`（品牌主色 **E · 深青 `#1c6e8c`**）

## Files

| File | Purpose |
|------|---------|
| `theme.css` | Design tokens（`:root` CSS variables） |
| `buttons.css` | Global `.btn` system（solid / outline / ghost） |
| `feedback.css` | Dialog / toast / notice / banner / badge utilities |
| `antd-theme.ts` | Ant Design 5 `ConfigProvider` theme |
| `antd-overrides.css` | Ant Design 残留样式对齐 |
| `tokens.ts` | JS 侧颜色常量（charts、inline style） |
| `v3-theme.css` | UI v3 作用域 token，映射到同一色板 |
| `variables.css` | 兼容 shim（`@import theme.css`） |

`index.css` 自动引入 theme / buttons / feedback / antd-overrides。  
`App.tsx` 已挂载 `ConfigProvider` + `antdTheme`。

## React Button

```tsx
import { Button } from '../components/ui';

<Button variant="solid" tone="brand">保存</Button>
<Button>取消</Button>
<Button variant="outline" tone="red">删除</Button>
<Button variant="solid" tone="brand" loading>保存中</Button>
```

## Dialog / Confirm（统一圆角矩形）

```tsx
// 确认 / 删除警示（全项目通用）
import ConfirmDialog from '../components/ConfirmDialog';

<ConfirmDialog
  isOpen={open}
  title="删除 Manifest"
  message="确定删除？此操作不可恢复。"
  type="danger"          // info | warning | danger
  confirmText="删除"
  cancelText="取消"
  loading={busy}
  onConfirm={handleDelete}
  onCancel={() => setOpen(false)}
/>

// 自定义内容弹窗
import { Dialog } from '../components/ui';

<Dialog open={open} onClose={...} title="标题" tone="info" size="md"
  footer={<><button className="btn">取消</button><button className="btn solid brand">保存</button></>}>
  ...body...
</Dialog>
```

样式源：`styles/dialog.css`（`.tn-overlay` / `.tn-dialog`）。Ant Design `Modal` / `Popconfirm` 已通过同文件 + `antd-theme` 对齐。

## Design rules

1. **主色唯一**：`brand` 深青 = 主操作 + 品牌动作。无 primary 蓝按钮。
2. **语义独立**：`green` 确认/启用 · `red` 危险/删除 · `amber` 仅 badge/banner/toast，**不做按钮**。
3. **蓝只给非按钮**：`var(--blue)` 用于 link 文字 / 输入聚焦 / 进度条 fill。
4. **一屏至多一个 solid** 主操作；列表危险动作用 `outline red` / `ghost red`，**实心 red 仅二次确认弹窗**。

## Tokens (常用)

```css
/* Brand */
var(--brand)        /* #1c6e8c 主色 */
var(--brand-ink)    /* hover */
var(--brand-700)    /* active */
var(--brand-soft)   /* 浅底 */
var(--brand-line)   /* 描边 */

/* Semantic */
var(--green) / var(--green-soft)
var(--red)   / var(--red-soft)
var(--amber) / var(--amber-soft)
var(--blue)  / var(--blue-soft)   /* 非按钮 */

/* Surfaces */
var(--bg) var(--surface) var(--surface-2) var(--line) var(--ink)

/* Focus rings */
var(--ring-brand) var(--ring-green) var(--ring-red)
```

兼容旧变量：`var(--color-blue-500)` → brand，`var(--color-primary)` → brand，
`var(--color-green-500)` → green，`var(--color-red-500)` → red。

## Buttons

```html
<button class="btn solid brand">保存</button>
<button class="btn">取消</button>
<button class="btn outline red">删除</button>
<button class="btn ghost brand">刷新</button>
<button class="btn sm solid green">启用</button>
<button class="btn solid brand is-loading">保存中</button>
```

兼容旧 class：`.btn.primary` / `.btn.brand` → solid brand；`.btn.red` → outline red。

## Feedback

```html
<div class="tn-toast tn-toast--success">...</div>
<div class="tn-notice tn-notice--error">...</div>
<div class="tn-banner tn-banner--warning">...</div>
<span class="tn-badge tn-badge--amber">warn</span>
<div class="tn-dialog">...</div>
```

## JS

```ts
import { colors, statusColor, rings } from '../styles/tokens';

el.style.color = colors.brand;
el.style.background = statusColor('success');
```

## Adding new UI

1. 颜色只用 token，禁止新 hardcode hex（尤其不要再写 `#3b82f6`）。
2. 主操作按钮：`class="btn solid brand"` 或 `background: var(--brand)`。
3. 危险二次确认：`class="btn solid red"`。
4. 通知/Toast：用 `tn-toast` / `tn-notice` 或 `statusColor()`。
