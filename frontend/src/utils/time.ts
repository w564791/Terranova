/**
 * 解析后端返回的时间字符串。
 *
 * 后端 DB 列是 timestamp without time zone，pgx 读取后将数值原样
 * 放入 time.Time 并标记为 UTC（带 Z 后缀），但实际值是本地时间。
 * 去掉时区后缀让 JavaScript 作为浏览器本地时间解析即可得到正确结果。
 */
export function parseBackendTime(dateString: string): Date {
  return new Date(dateString.replace(/([+-]\d{2}:\d{2}|Z)$/, ''));
}
