---
name: region_mapping
layer: domain
description: 区域映射规则，定义用户自然语言描述与云服务商区域代码之间的映射关系
tags: ["domain", "region", "mapping", "aws", "location", "geography"]
---

## 区域映射规则

### 概述
本规范定义了用户自然语言描述（中文、英文）与云服务商区域代码之间的映射关系。帮助 AI 正确识别用户提到的区域，并转换为标准的区域代码。

### AWS 区域映射

#### 亚太区域
| 中文名称 | 英文名称 | 区域代码 | 可用区 |
|----------|----------|----------|--------|
| 东京 | Tokyo | `ap-northeast-1` | a, c, d |
| 首尔 | Seoul | `ap-northeast-2` | a, b, c, d |
| 大阪 | Osaka | `ap-northeast-3` | a, b, c |
| 新加坡 | Singapore | `ap-southeast-1` | a, b, c |
| 悉尼 | Sydney | `ap-southeast-2` | a, b, c |
| 雅加达 | Jakarta | `ap-southeast-3` | a, b, c |
| 孟买 | Mumbai | `ap-south-1` | a, b, c |
| 海得拉巴 | Hyderabad | `ap-south-2` | a, b, c |
| 香港 | Hong Kong | `ap-east-1` | a, b, c |
| 墨尔本 | Melbourne | `ap-southeast-4` | a, b, c |

#### 美洲区域
| 中文名称 | 英文名称 | 区域代码 | 可用区 |
|----------|----------|----------|--------|
| 美东(弗吉尼亚) | N. Virginia | `us-east-1` | a, b, c, d, e, f |
| 美东(俄亥俄) | Ohio | `us-east-2` | a, b, c |
| 美西(加利福尼亚) | N. California | `us-west-1` | a, b |
| 美西(俄勒冈) | Oregon | `us-west-2` | a, b, c, d |
| 加拿大(中部) | Canada | `ca-central-1` | a, b, d |
| 圣保罗 | São Paulo | `sa-east-1` | a, b, c |

#### 欧洲区域
| 中文名称 | 英文名称 | 区域代码 | 可用区 |
|----------|----------|----------|--------|
| 爱尔兰 | Ireland | `eu-west-1` | a, b, c |
| 伦敦 | London | `eu-west-2` | a, b, c |
| 巴黎 | Paris | `eu-west-3` | a, b, c |
| 法兰克福 | Frankfurt | `eu-central-1` | a, b, c |
| 苏黎世 | Zurich | `eu-central-2` | a, b, c |
| 斯德哥尔摩 | Stockholm | `eu-north-1` | a, b, c |
| 米兰 | Milan | `eu-south-1` | a, b, c |
| 西班牙 | Spain | `eu-south-2` | a, b, c |

#### 中东和非洲区域
| 中文名称 | 英文名称 | 区域代码 | 可用区 |
|----------|----------|----------|--------|
| 巴林 | Bahrain | `me-south-1` | a, b, c |
| 阿联酋 | UAE | `me-central-1` | a, b, c |
| 开普敦 | Cape Town | `af-south-1` | a, b, c |
| 特拉维夫 | Tel Aviv | `il-central-1` | a, b, c |

### 关键词识别

#### 区域关键词映射
```yaml
东京:
  keywords: ["东京", "tokyo", "日本", "japan", "jp"]
  region: "ap-northeast-1"

新加坡:
  keywords: ["新加坡", "singapore", "sg", "狮城"]
  region: "ap-southeast-1"

香港:
  keywords: ["香港", "hong kong", "hk"]
  region: "ap-east-1"

美东:
  keywords: ["美东", "us-east", "弗吉尼亚", "virginia", "美国东部"]
  region: "us-east-1"

美西:
  keywords: ["美西", "us-west", "俄勒冈", "oregon", "美国西部"]
  region: "us-west-2"

欧洲:
  keywords: ["欧洲", "europe", "eu", "法兰克福", "frankfurt", "德国"]
  region: "eu-central-1"
```

#### 模糊匹配规则
- 大小写不敏感：`Tokyo` = `tokyo` = `TOKYO`
- 空格和连字符等价：`us-east-1` = `us east 1`
- 支持部分匹配：`东京` 匹配 `ap-northeast-1`

### 可用区格式

#### 格式规则
```
可用区 = 区域代码 + 字母后缀
示例: ap-northeast-1a, ap-northeast-1c, us-east-1b
```

#### 可用区选择策略
1. **默认选择**：选择区域的第一个可用区（通常是 `a`）
2. **高可用**：选择多个可用区（如 `a` 和 `c`）
3. **用户指定**：使用用户明确指定的可用区

### 区域一致性规则

#### 强制规则
1. **同一配置内的资源必须在同一区域**
   - VPC、子网、安全组必须在同一区域
   - EC2 实例必须与其子网在同一区域

2. **跨区域资源需要特别处理**
   - S3 存储桶可以跨区域访问
   - IAM 是全局服务，不受区域限制
   - Route 53 是全局服务

#### 验证规则
```
如果用户指定了区域 A，但 CMDB 返回的资源在区域 B：
→ 返回 need_more_info 状态
→ 提示用户区域不匹配
→ 建议使用区域 A 的资源或更改目标区域
```

### 默认区域

#### 默认区域选择
当用户未指定区域时：
1. 使用工作空间的默认区域
2. 如果工作空间未设置，使用 `ap-northeast-1`（东京）

#### 区域推断
从 CMDB 资源推断区域：
```
用户: "使用 exchange VPC"
CMDB 返回: vpc-xxx (region: ap-northeast-1)
推断: 目标区域为 ap-northeast-1
```

### 使用示例

#### 示例 1: 中文区域名称
```
用户输入: "在东京区域创建 EC2 实例"
识别结果:
  - 区域: ap-northeast-1
  - 默认可用区: ap-northeast-1a
```

#### 示例 2: 英文区域名称
```
用户输入: "Deploy in Singapore region"
识别结果:
  - 区域: ap-southeast-1
  - 默认可用区: ap-southeast-1a
```

#### 示例 3: 区域代码
```
用户输入: "使用 us-west-2 区域"
识别结果:
  - 区域: us-west-2
  - 默认可用区: us-west-2a
```

#### 示例 4: 多可用区
```
用户输入: "在东京区域部署高可用配置"
识别结果:
  - 区域: ap-northeast-1
  - 可用区: ["ap-northeast-1a", "ap-northeast-1c"]
```

### 跨云服务商区域映射

#### Azure 区域
| 中文名称 | Azure 区域 |
|----------|-----------|
| 东京 | `japaneast` |
| 新加坡 | `southeastasia` |
| 香港 | `eastasia` |
| 美东 | `eastus` |
| 欧洲 | `westeurope` |

#### GCP 区域
| 中文名称 | GCP 区域 |
|----------|---------|
| 东京 | `asia-northeast1` |
| 新加坡 | `asia-southeast1` |
| 香港 | `asia-east2` |
| 美东 | `us-east1` |
| 欧洲 | `europe-west1` |

### 区域选择建议

#### 延迟优化
| 用户位置 | 推荐区域 |
|----------|----------|
| 中国大陆 | `ap-northeast-1` (东京) 或 `ap-southeast-1` (新加坡) |
| 日本 | `ap-northeast-1` (东京) |
| 东南亚 | `ap-southeast-1` (新加坡) |
| 美国 | `us-east-1` 或 `us-west-2` |
| 欧洲 | `eu-central-1` (法兰克福) |

#### 合规要求
| 合规要求 | 推荐区域 |
|----------|----------|
| 数据本地化（日本） | `ap-northeast-1` |
| GDPR（欧盟） | `eu-central-1` 或 `eu-west-1` |
| 数据本地化（新加坡） | `ap-southeast-1` |