import React, { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend } from 'chart.js';
import { Line } from 'react-chartjs-2';
import ApiService from '../services/api';
import { Campaign, CampaignStats } from '../types';

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend);

const Dashboard: React.FC = () => {
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [campaignStats, setCampaignStats] = useState<Map<string, CampaignStats>>(new Map());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const api = new ApiService();

  useEffect(() => {
    loadDashboardData();
  }, []);

  const loadDashboardData = async () => {
    try {
      setLoading(true);
      const campaignList = await api.getCampaigns();
      setCampaigns(campaignList);

      // Load stats for each campaign
      const statsMap = new Map<string, CampaignStats>();
      for (const campaign of campaignList) {
        try {
          const stats = await api.getCampaignStats(campaign.id);
          statsMap.set(campaign.id, stats);
        } catch (err) {
          console.error(`Failed to load stats for campaign ${campaign.id}:`, err);
        }
      }
      setCampaignStats(statsMap);
    } catch (err) {
      setError('Failed to load dashboard data');
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const calculateTotalStats = () => {
    let totalJobs = 0;
    let totalCrashes = 0;
    let totalCoverage = 0;
    let totalExecPerSec = 0;

    campaignStats.forEach(stats => {
      totalJobs += stats.total_jobs;
      totalCrashes += stats.unique_crashes;
      totalCoverage = Math.max(totalCoverage, stats.total_coverage);
      totalExecPerSec += stats.exec_per_second;
    });

    return { totalJobs, totalCrashes, totalCoverage, totalExecPerSec };
  };

  if (loading) return <div className="loading">Loading dashboard...</div>;
  if (error) return <div className="error">{error}</div>;

  const { totalJobs, totalCrashes, totalCoverage, totalExecPerSec } = calculateTotalStats();

  return (
    <div>
      <h1>Fuzzing Overview</h1>

      <div className="stats-grid">
        <div className="stat-card">
          <div className="stat-label">Active Campaigns</div>
          <div className="stat-value">{campaigns.filter(c => c.status === 'active').length}</div>
          <div className="stat-change">Total: {campaigns.length}</div>
        </div>

        <div className="stat-card">
          <div className="stat-label">Total Jobs</div>
          <div className="stat-value">{totalJobs}</div>
          <div className="stat-change positive">+12% from last week</div>
        </div>

        <div className="stat-card">
          <div className="stat-label">Unique Crashes</div>
          <div className="stat-value">{totalCrashes}</div>
          <div className="stat-change positive">+5 new today</div>
        </div>

        <div className="stat-card">
          <div className="stat-label">Coverage</div>
          <div className="stat-value">{totalCoverage.toLocaleString()}</div>
          <div className="stat-change">edges covered</div>
        </div>

        <div className="stat-card">
          <div className="stat-label">Exec Speed</div>
          <div className="stat-value">{Math.round(totalExecPerSec).toLocaleString()}</div>
          <div className="stat-change">execs/sec</div>
        </div>
      </div>

      <div className="card">
        <h2>Active Campaigns</h2>
        <table className="data-table">
          <thead>
            <tr>
              <th>Campaign</th>
              <th>Target</th>
              <th>Fuzzer</th>
              <th>Status</th>
              <th>Jobs</th>
              <th>Crashes</th>
              <th>Coverage</th>
              <th>Exec/s</th>
            </tr>
          </thead>
          <tbody>
            {campaigns.map(campaign => {
              const stats = campaignStats.get(campaign.id);
              return (
                <tr key={campaign.id}>
                  <td>
                    <Link to={`/campaign/${campaign.id}`}>{campaign.name}</Link>
                  </td>
                  <td>{campaign.target_binary}</td>
                  <td>{campaign.fuzzer}</td>
                  <td>
                    <span className={`status-badge ${campaign.status}`}>
                      {campaign.status}
                    </span>
                  </td>
                  <td>{stats?.total_jobs || 0}</td>
                  <td>{stats?.unique_crashes || 0}</td>
                  <td>{stats?.total_coverage.toLocaleString() || 0}</td>
                  <td>{Math.round(stats?.exec_per_second || 0).toLocaleString()}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <QuickInsights campaigns={campaigns} stats={campaignStats} />
    </div>
  );
};

const QuickInsights: React.FC<{ campaigns: Campaign[], stats: Map<string, CampaignStats> }> = ({ campaigns, stats }) => {
  // Find best performing campaigns
  const sortedByCrashes = Array.from(stats.entries())
    .sort((a, b) => b[1].unique_crashes - a[1].unique_crashes)
    .slice(0, 3);

  const sortedByCoverage = Array.from(stats.entries())
    .sort((a, b) => b[1].total_coverage - a[1].total_coverage)
    .slice(0, 3);

  return (
    <div className="card">
      <h2>Quick Insights</h2>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '2rem' }}>
        <div>
          <h3>Top Crash Finders</h3>
          <ol>
            {sortedByCrashes.map(([campaignId, stat]) => {
              const campaign = campaigns.find(c => c.id === campaignId);
              return (
                <li key={campaignId}>
                  <Link to={`/campaign/${campaignId}`}>{campaign?.name}</Link>
                  <span style={{ marginLeft: '1rem', color: '#666' }}>
                    {stat.unique_crashes} crashes
                  </span>
                </li>
              );
            })}
          </ol>
        </div>

        <div>
          <h3>Highest Coverage</h3>
          <ol>
            {sortedByCoverage.map(([campaignId, stat]) => {
              const campaign = campaigns.find(c => c.id === campaignId);
              return (
                <li key={campaignId}>
                  <Link to={`/campaign/${campaignId}`}>{campaign?.name}</Link>
                  <span style={{ marginLeft: '1rem', color: '#666' }}>
                    {stat.total_coverage.toLocaleString()} edges
                  </span>
                </li>
              );
            })}
          </ol>
        </div>
      </div>
    </div>
  );
};

export default Dashboard;