import React, { useState, useEffect } from 'react';
import api from '../services/api';
import styles from './Dashboard.module.css';

interface OverviewStats {
  active_projects: number;
  active_workspaces: number;
  total_applies: number;
  applies_this_month: number;
  average_applies_per_month: number;
  billable_managed_resources: number;
  billable_limit: number;
  concurrent_run_limit: number;
  concurrent_limit: number;
  active_agents: number;
  total_agents: number;
}

interface ComplianceStats {
  run_task_integrations: { current: number; total: number };
  workspace_run_tasks: { current: number; total: number };
  policy_sets: { current: number; total: number };
  policies: { current: number; total: number };
}

const Dashboard: React.FC = () => {
  const [overviewStats, setOverviewStats] = useState<OverviewStats | null>(null);
  const [complianceStats, setComplianceStats] = useState<ComplianceStats | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadStats();
  }, []);

  const loadStats = async () => {
    try {
      setLoading(true);
      const overview = await api.get('/dashboard/overview');
      const compliance = await api.get('/dashboard/compliance');
      setOverviewStats(overview);
      setComplianceStats(compliance);
    } catch (err) {
      console.error('Failed to load dashboard stats:', err);
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return <div className={styles.loading}>Loading...</div>;
  }

  return (
    <div className={styles.container}>
      {/* Usage Report Section */}
      <section className={styles.section}>
        <div className={styles.sectionHeader}>
          <h2 className={styles.sectionTitle}>Usage report</h2>
          <p className={styles.sectionSubtitle}>
            Welcome to IAC Management Platform
          </p>
        </div>

        <div className={styles.subsectionTitle}>Overview</div>

        <div className={styles.statsGrid}>
          {/* Row 1 */}
          <div className={styles.statCard}>
            <div className={styles.statLabel}>Active projects</div>
            <div className={styles.statValue}>{overviewStats?.active_projects || 0}</div>
          </div>

          <div className={styles.statCard}>
            <div className={styles.statLabel}>Active workspaces</div>
            <div className={styles.statValue}>{overviewStats?.active_workspaces || 0}</div>
          </div>

          <div className={styles.statCard}>
            <div className={styles.statLabel}>Total applies</div>
            <div className={styles.statValue}>{overviewStats?.total_applies || 0}</div>
            <div className={styles.statHint}>since plan started</div>
          </div>

          <div className={styles.statCard}>
            <div className={styles.statLabel}>Applies this month</div>
            <div className={styles.statValue}>{overviewStats?.applies_this_month || 0}</div>
          </div>

          <div className={styles.statCard}>
            <div className={styles.statLabel}>Average applies per month</div>
            <div className={styles.statValue}>{overviewStats?.average_applies_per_month || 0}</div>
          </div>

          <div className={styles.statCard}>
            <div className={styles.statLabel}>
              Managed Resources
              <span className={styles.infoIcon} title="Resources managed by Terraform">ⓘ</span>
            </div>
            <div className={styles.statValue}>{overviewStats?.billable_managed_resources || 0}</div>
          </div>

          {/* Row 2 */}
          <div className={styles.statCard}>
            <div className={styles.statLabel}>Concurrent running tasks</div>
            <div className={styles.statValue}>{overviewStats?.concurrent_run_limit || 0}</div>
          </div>

          <div className={styles.statCard}>
            <div className={styles.statLabel}>
              Active agents
              <span className={styles.infoIcon} title="Agents currently online">ⓘ</span>
            </div>
            <div className={styles.statValue}>{overviewStats?.active_agents || 0}</div>
            <div className={styles.statHint}>
              total agents: {overviewStats?.total_agents || 0}
            </div>
          </div>
        </div>
      </section>

      {/* Compliance Overview Section */}
      <section className={styles.section}>
        <h2 className={styles.sectionTitle}>Compliance overview</h2>

        <div className={styles.complianceGrid}>
          <div className={styles.complianceCard}>
            <div className={styles.complianceLabel}>Successful tasks</div>
            <div className={styles.complianceValue}>
              {complianceStats?.run_task_integrations.current || 0}
            </div>
          </div>

          <div className={styles.complianceCard}>
            <div className={styles.complianceLabel}>Failed tasks</div>
            <div className={styles.complianceValue}>
              {complianceStats?.workspace_run_tasks.current || 0}
            </div>
          </div>

          <div className={styles.complianceCard}>
            <div className={styles.complianceLabel}>Pending tasks</div>
            <div className={styles.complianceValue}>
              {complianceStats?.policy_sets.current || 0}
            </div>
          </div>

          <div className={styles.complianceCard}>
            <div className={styles.complianceLabel}>Total tasks</div>
            <div className={styles.complianceValue}>
              {complianceStats?.policies.current || 0}
            </div>
          </div>
        </div>
      </section>
    </div>
  );
};

export default Dashboard;
