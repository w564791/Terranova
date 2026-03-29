import api from './api';

export interface SummaryCoverage {
  total_resources: number;
  with_summary: number;
  coverage_pct: number;
}

export interface AssessmentStats {
  total_assessed: number;
  l1_pass_rate: number;
  l2_avg_score: number;
  l3_avg_score: number;
  l1_pass: number;
  l1_warn: number;
  l1_fail: number;
}

export interface RuleMissCount {
  rule: string;
  miss_count: number;
}

export interface SecurityTagStats {
  total_expected: number;
  total_hit: number;
  hit_rate: number;
  misses_by_rule: RuleMissCount[];
}

export interface IssueCount {
  type: string;
  count: number;
}

export interface IssueDistribution {
  format_violations: IssueCount[];
  hallucination_suspects: number;
  security_tag_misses: number;
}

export interface SummaryDailyTrend {
  date: string;
  total: number;
  pass: number;
  warn: number;
  fail: number;
  pass_rate: number;
}

export interface ResourceTypeStats {
  resource_type: string;
  count: number;
  pass_rate: number;
  avg_score: number;
}

export interface SummaryAssessmentOverview {
  summary_coverage: SummaryCoverage;
  assessment: AssessmentStats;
  security_tag_stats: SecurityTagStats;
  issue_distribution: IssueDistribution;
  daily_trend: SummaryDailyTrend[];
  by_resource_type: ResourceTypeStats[];
}

export async function getSummaryAssessmentOverview(days: number): Promise<SummaryAssessmentOverview> {
  return await api.get(`/admin/summary-assessment/overview?days=${days}`) as unknown as SummaryAssessmentOverview;
}

export interface IssueResource {
  resource_id: number;
  resource_type: string;
  resource_name: string;
  resource_summary: string;
  verdict: string;
  score: number;
  details: string;
}

export async function getIssueResources(type: string, days: number): Promise<IssueResource[]> {
  return await api.get(`/admin/summary-assessment/issue-resources?type=${encodeURIComponent(type)}&days=${days}`) as unknown as IssueResource[];
}
