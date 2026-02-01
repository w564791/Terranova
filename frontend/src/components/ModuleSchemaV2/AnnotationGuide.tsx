import React from 'react';
import { Modal, Typography, Divider } from 'antd';
import styles from './ModuleSchemaV2.module.css';

const { Title, Paragraph, Text } = Typography;

interface AnnotationGuideProps {
  visible: boolean;
  onClose: () => void;
}

const AnnotationGuide: React.FC<AnnotationGuideProps> = ({ visible, onClose }) => {
  return (
    <Modal
      title="Terraform 变量注释规范说明"
      open={visible}
      onCancel={onClose}
      footer={null}
      width={800}
    >
      <div className={styles.guideContainer}>
        <Typography>
          <Paragraph>
            通过在 Terraform 变量的 <Text code>description</Text> 行末尾添加特殊注释，
            可以控制变量在表单中的显示方式和行为。
          </Paragraph>

          <Divider />

          <div className={styles.guideSection}>
            <Title level={4}>基本格式</Title>
            <Paragraph>
              注释格式为 <Text code>@key:value</Text>，多个注释用空格分隔。
            </Paragraph>
            <pre className={styles.guideCode}>{`variable "instance_type" {
  description = "EC2 instance type" # @level:basic @widget:select @source:instance_types
  type        = string
  default     = "t3.micro"
}`}</pre>
          </div>

          <Divider />

          <div className={styles.guideSection}>
            <Title level={4}>支持的注释</Title>
            <table className={styles.annotationTable}>
              <thead>
                <tr>
                  <th>注释</th>
                  <th>说明</th>
                  <th>可选值</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>@level</code></td>
                  <td>字段分组级别</td>
                  <td><code>basic</code> (基础配置), <code>advanced</code> (高级配置)</td>
                </tr>
                <tr>
                  <td><code>@alias</code></td>
                  <td>字段中文别名/标签</td>
                  <td>任意中文文本，如 <code>@alias:实例类型</code></td>
                </tr>
                <tr>
                  <td><code>@widget</code></td>
                  <td>UI 组件类型</td>
                  <td>
                    <code>text</code>, <code>textarea</code>, <code>number</code>, 
                    <code>select</code>, <code>switch</code>, <code>tags</code>, 
                    <code>key-value</code>, <code>object</code>, <code>object-list</code>, 
                    <code>json-editor</code>
                  </td>
                </tr>
                <tr>
                  <td><code>@source</code></td>
                  <td>外部数据源 ID</td>
                  <td>
                    <code>ami_list</code>, <code>instance_types</code>, 
                    <code>availability_zones</code>, <code>subnet_list</code>, 
                    <code>security_groups</code>, <code>key_pairs</code>
                  </td>
                </tr>
                <tr>
                  <td><code>@order</code></td>
                  <td>字段排序顺序</td>
                  <td>数字，如 <code>@order:1</code></td>
                </tr>
                <tr>
                  <td><code>@placeholder</code></td>
                  <td>输入框占位符</td>
                  <td>任意文本</td>
                </tr>
                <tr>
                  <td><code>@searchable</code></td>
                  <td>下拉框是否可搜索</td>
                  <td><code>true</code>, <code>false</code></td>
                </tr>
                <tr>
                  <td><code>@allowcustom</code></td>
                  <td>下拉框是否允许自定义值</td>
                  <td><code>true</code>, <code>false</code></td>
                </tr>
                <tr>
                  <td><code>@enum</code></td>
                  <td>枚举值列表</td>
                  <td>逗号分隔的值，如 <code>@enum:dev,staging,prod</code></td>
                </tr>
              </tbody>
            </table>
          </div>

          <Divider />

          <div className={styles.guideSection}>
            <Title level={4}>完整示例</Title>
            <pre className={styles.guideCode}>{`# 基础配置字段
variable "name" {
  description = "Resource name" # @level:basic @alias:资源名称 @order:1
  type        = string
}

variable "environment" {
  description = "Deployment environment" # @level:basic @widget:select @enum:dev,staging,prod
  type        = string
  default     = "dev"
}

# 带外部数据源的字段
variable "instance_type" {
  description = "EC2 instance type" # @level:basic @source:instance_types @searchable:true
  type        = string
  default     = "t3.micro"
}

variable "ami_id" {
  description = "AMI ID for the instance" # @level:basic @source:ami_list @allowcustom:true
  type        = string
}

# 高级配置字段
variable "tags" {
  description = "Resource tags" # @level:advanced @widget:key-value
  type        = map(string)
  default     = {}
}

variable "security_group_ids" {
  description = "Security group IDs" # @level:advanced @source:security_groups
  type        = list(string)
  default     = []
}

variable "user_data" {
  description = "User data script" # @level:advanced @widget:textarea
  type        = string
  default     = ""
}

variable "enable_monitoring" {
  description = "Enable detailed monitoring" # @level:advanced
  type        = bool
  default     = false
}`}</pre>
          </div>

          <Divider />

          <div className={styles.guideSection}>
            <Title level={4}>Widget 类型说明</Title>
            <table className={styles.annotationTable}>
              <thead>
                <tr>
                  <th>Widget</th>
                  <th>适用类型</th>
                  <th>说明</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>text</code></td>
                  <td>string</td>
                  <td>单行文本输入框（默认）</td>
                </tr>
                <tr>
                  <td><code>textarea</code></td>
                  <td>string</td>
                  <td>多行文本输入框</td>
                </tr>
                <tr>
                  <td><code>number</code></td>
                  <td>number</td>
                  <td>数字输入框</td>
                </tr>
                <tr>
                  <td><code>select</code></td>
                  <td>string</td>
                  <td>下拉选择框，配合 @source 或 @enum 使用</td>
                </tr>
                <tr>
                  <td><code>switch</code></td>
                  <td>bool</td>
                  <td>开关组件（默认用于 bool 类型）</td>
                </tr>
                <tr>
                  <td><code>tags</code></td>
                  <td>list(string)</td>
                  <td>标签输入组件（默认用于字符串列表）</td>
                </tr>
                <tr>
                  <td><code>key-value</code></td>
                  <td>map(string)</td>
                  <td>键值对编辑器（默认用于 map 类型）</td>
                </tr>
                <tr>
                  <td><code>object</code></td>
                  <td>object</td>
                  <td>嵌套对象编辑器</td>
                </tr>
                <tr>
                  <td><code>object-list</code></td>
                  <td>list(object)</td>
                  <td>对象列表编辑器</td>
                </tr>
                <tr>
                  <td><code>json-editor</code></td>
                  <td>any</td>
                  <td>JSON 代码编辑器</td>
                </tr>
              </tbody>
            </table>
          </div>

          <Divider />

          <div className={styles.guideSection}>
            <Title level={4}>外部数据源说明</Title>
            <Paragraph>
              使用 <Text code>@source</Text> 注释可以让字段从外部 API 获取选项数据。
              系统预定义了以下数据源：
            </Paragraph>
            <table className={styles.annotationTable}>
              <thead>
                <tr>
                  <th>数据源 ID</th>
                  <th>说明</th>
                  <th>依赖字段</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>ami_list</code></td>
                  <td>AWS AMI 镜像列表</td>
                  <td>region</td>
                </tr>
                <tr>
                  <td><code>instance_types</code></td>
                  <td>EC2 实例类型列表</td>
                  <td>region</td>
                </tr>
                <tr>
                  <td><code>availability_zones</code></td>
                  <td>可用区列表</td>
                  <td>region</td>
                </tr>
                <tr>
                  <td><code>subnet_list</code></td>
                  <td>子网列表</td>
                  <td>region, vpc_id</td>
                </tr>
                <tr>
                  <td><code>security_groups</code></td>
                  <td>安全组列表</td>
                  <td>region, vpc_id</td>
                </tr>
                <tr>
                  <td><code>key_pairs</code></td>
                  <td>密钥对列表</td>
                  <td>region</td>
                </tr>
                <tr>
                  <td><code>iam_instance_profiles</code></td>
                  <td>IAM 实例配置文件列表</td>
                  <td>-</td>
                </tr>
              </tbody>
            </table>
          </div>
        </Typography>
      </div>
    </Modal>
  );
};

export default AnnotationGuide;
