import api from './api';

export interface CapabilityStats {
  capability: string;
  total: number;
  pass: number;
  fail: number;
  warn: number;
  pass_rate: number;
}

export interface RecentFailure {
  usage_log_id: string;
  capability: string;
  skill_name: string;
  verdict: string;
  missing_fields: string[];
  invalid_enum_fields: string[];
  assessed_at: string;
  content_hash: string;
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
  daily_trend: DailyTrendItem[];
}

export async function getAssessmentOverview(days: number = 7): Promise<AssessmentOverview> {
  return await api.get(`/admin/skill-assessment/overview?days=${days}`) as unknown as AssessmentOverview;
}
