import api from './api';

export interface CapabilityStats {
  capability: string;
  total: number;
  pass: number;
  fail: number;
  warn: number;
  pass_rate: number;
  avg_score: number;
  avg_latency_ms: number;
}

export interface RecentFailure {
  usage_log_id: string;
  capability: string;
  skill_name: string;
  verdict: string;
  score: number;
  missing_fields: string[];
  invalid_enum_fields: string[];
  assessed_at: string;
  content_hash: string;
  latency_ms: number | null;
}

export interface DailyTrendItem {
  date: string;
  pass: number;
  fail: number;
  warn: number;
}

export interface AssessmentOverview {
  total_logs: number;
  assessed_logs: number;
  total_pass: number;
  total_fail: number;
  total_warn: number;
  pass_rate: number;
  active_skills: number;
  high_risk_skills: string[];
  by_capability: CapabilityStats[];
  recent_failures: RecentFailure[];
  failure_total: number;
  daily_trend: DailyTrendItem[];
  // Business metrics
  accept_rate: number | null;
  modify_rate: number | null;
  negative_feedback: number | null;
}

export async function getAssessmentOverview(days: number = 7, failPage: number = 1, failPageSize: number = 10): Promise<AssessmentOverview> {
  return await api.get(`/admin/skill-assessment/overview?days=${days}&fail_page=${failPage}&fail_page_size=${failPageSize}`) as unknown as AssessmentOverview;
}

// ========== Skill Detail API ==========

export interface VersionStats {
  content_hash: string;
  total: number;
  pass: number;
  fail: number;
  avg_score: number;
  pass_rate: number;
  first_seen: string;
  l1_pass_rate: number | null;
  l2_pass_rate: number | null;
  l2_avg_score: number | null;
  l3_pass_rate: number | null;
  l3_avg_score: number | null;
}

export interface AssessmentRecord {
  usage_log_id: string;
  layer: string;
  verdict: string;
  score: number;
  latency_ms: number | null;
  missing_fields: string[];
  invalid_enum_fields: string[];
  assessed_at: string;
  content_hash: string;
}

export interface FeedbackMatrix {
  pass_positive: number;
  pass_negative: number;
  pass_no_feedback: number;
  warn_positive: number;
  warn_negative: number;
  warn_no_feedback: number;
  fail_positive: number;
  fail_negative: number;
  fail_no_feedback: number;
}

export interface CapabilityDetail {
  capability: string;
  pass_rate: number;
  total: number;
  pass: number;
  fail: number;
  avg_score: number;
  avg_latency_ms: number;
  latest_hash: string;
  task_skill: string;
  versions: VersionStats[];
  assessments: AssessmentRecord[];
  assessment_total: number;
  feedback_matrix: FeedbackMatrix | null;
}

export async function getCapabilityDetail(capability: string, days: number = 7, page: number = 1, pageSize: number = 20): Promise<CapabilityDetail> {
  return await api.get(`/admin/skill-assessment/detail?capability=${encodeURIComponent(capability)}&days=${days}&page=${page}&page_size=${pageSize}`) as unknown as CapabilityDetail;
}
