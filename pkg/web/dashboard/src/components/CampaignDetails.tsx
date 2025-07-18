import React, { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import { Line, Bar } from 'react-chartjs-2';
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Title,
  Tooltip,
  Legend
} from 'chart.js';
import ApiService from '../services/api';
import { Campaign, CampaignStats, CoverageTrend, CrashRateMetrics, Job } from '../types';

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Title,
  Tooltip,
  Legend
);

const CampaignDetails: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const [campaign, setCampaign] = useState<Campaign | null>(null);
  const [stats, setStats] = useState<CampaignStats | null>(null);
  const [coverageTrend, setCoverageTrend] = useState<CoverageTrend | null>(null);
  const [crashRate, setCrashRate] = useState<CrashRateMetrics | null>(null);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedPeriod, setSelectedPeriod] = useState('24h');

  const api = new ApiService();

  useEffect(() => {
    if (id) {
      loadCampaignData(id);
    }
  }, [id, selectedPeriod]);

  const loadCampaignData = async (campaignId: string) => {
    try {
      setLoading(true);
      const [campaignData, statsData, coverageData, crashData, jobsData] = await Promise.all([
        api.getCampaign(campaignId),
        api.getCampaignStats(campaignId),
        api.getCoverageTrend(campaignId, selectedPeriod),
        api.getCrashRate(campaignId, selectedPeriod),
        api.getJobs(campaignId)
      ]);

      setCampaign(campaignData);
      setStats(statsData);
      setCoverageTrend(coverageData);
      setCrashRate(crashData);
      setJobs(jobsData);
    } catch (error) {
      console.error('Failed to load campaign data:', error);
    } finally {
      setLoading(false);
    }
  };

  if (loading) return <div className="loading">Loading campaign details...</div>;
  if (!campaign || !stats) return <div className="error">Campaign not found</div>;

  // Prepare chart data
  const coverageChartData = {
    labels: coverageTrend?.data_points.map(p => 
      new Date(p.timestamp).toLocaleTimeString()
    ) || [],
    datasets: [
      {
        label: 'Total Coverage',
        data: coverageTrend?.data_points.map(p => p.total_edges) || [],
        borderColor: 'rgb(75, 192, 192)',
        backgroundColor: 'rgba(75, 192, 192, 0.1)',
        tension: 0.4
      },
      {
        label: 'Exec/s',
        data: coverageTrend?.data_points.map(p => p.exec_per_sec) || [],
        borderColor: 'rgb(255, 159, 64)',
        backgroundColor: 'rgba(255, 159, 64, 0.1)',
        yAxisID: 'y1',
        tension: 0.4
      }
    ]
  };

  const coverageChartOptions = {
    responsive: true,
    maintainAspectRatio: false,
    interaction: {
      mode: 'index' as const,
      intersect: false,
    },
    plugins: {
      legend: {
        position: 'top' as const,
      },
      title: {
        display: true,
        text: 'Coverage Growth Over Time'
      }
    },
    scales: {
      y: {
        type: 'linear' as const,
        display: true,
        position: 'left' as const,
        title: {
          display: true,
          text: 'Coverage (edges)'
        }
      },
      y1: {
        type: 'linear' as const,
        display: true,
        position: 'right' as const,
        title: {
          display: true,
          text: 'Executions/sec'
        },
        grid: {
          drawOnChartArea: false,
        },
      },
    },
  };

  const jobStatusData = {
    labels: ['Pending', 'Running', 'Completed', 'Failed'],
    datasets: [{
      label: 'Jobs by Status',
      data: [
        jobs.filter(j => j.status === 'pending').length,
        jobs.filter(j => j.status === 'running').length,
        jobs.filter(j => j.status === 'completed').length,
        jobs.filter(j => j.status === 'failed').length,
      ],
      backgroundColor: [
        'rgba(255, 206, 86, 0.8)',
        'rgba(54, 162, 235, 0.8)',
        'rgba(75, 192, 192, 0.8)',
        'rgba(255, 99, 132, 0.8)',
      ],
    }]
  };

  return (
    <div>
      <div className="card">
        <div className="card-header">
          <h1>{campaign.name}</h1>
          <div>
            <select 
              value={selectedPeriod} 
              onChange={(e) => setSelectedPeriod(e.target.value)}
              className="button secondary"
              style={{ marginRight: '1rem' }}
            >
              <option value="1h">Last Hour</option>
              <option value="6h">Last 6 Hours</option>
              <option value="24h">Last 24 Hours</option>
              <option value="7d">Last 7 Days</option>
            </select>
            <span className={`status-badge ${campaign.status}`}>
              {campaign.status}
            </span>
          </div>
        </div>

        <div className="stats-grid">
          <div className="stat-card">
            <div className="stat-label">Total Jobs</div>
            <div className="stat-value">{stats.total_jobs}</div>
            <div className="stat-change">
              {stats.active_jobs} active, {stats.completed_jobs} completed
            </div>
          </div>

          <div className="stat-card">
            <div className="stat-label">Unique Crashes</div>
            <div className="stat-value">{stats.unique_crashes}</div>
            <div className="stat-change positive">
              {stats.total_crashes} total crashes
            </div>
          </div>

          <div className="stat-card">
            <div className="stat-label">Coverage</div>
            <div className="stat-value">{stats.total_coverage.toLocaleString()}</div>
            <div className="stat-change">edges covered</div>
          </div>

          <div className="stat-card">
            <div className="stat-label">Corpus Size</div>
            <div className="stat-value">{stats.corpus_size}</div>
            <div className="stat-change">files</div>
          </div>
        </div>
      </div>

      <div className="card">
        <h2>Coverage Trend</h2>
        <div className="chart-container">
          <Line data={coverageChartData} options={coverageChartOptions} />
        </div>
        {coverageTrend && (
          <div style={{ marginTop: '1rem', color: '#666' }}>
            <p>Growth Rate: {coverageTrend.growth_rate.toFixed(2)} edges/hour</p>
            <p>Total Growth: {coverageTrend.total_growth.toLocaleString()} edges</p>
          </div>
        )}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1.5rem' }}>
        <div className="card">
          <h2>Crash Rate Analysis</h2>
          {crashRate && (
            <div>
              <div className="stat-card">
                <div className="stat-label">Crash Rate</div>
                <div className="stat-value">{crashRate.crash_rate.toFixed(2)}</div>
                <div className="stat-change">crashes/hour</div>
              </div>
              <div className="stat-card">
                <div className="stat-label">Unique Crash Rate</div>
                <div className="stat-value">{crashRate.unique_crash_rate.toFixed(2)}</div>
                <div className="stat-change">unique/hour</div>
              </div>
              <div className="stat-card">
                <div className="stat-label">Trend</div>
                <div className={`stat-value ${crashRate.trend}`}>
                  {crashRate.trend}
                </div>
                <div className="stat-change">
                  {(crashRate.trend_confidence * 100).toFixed(0)}% confidence
                </div>
              </div>
            </div>
          )}
        </div>

        <div className="card">
          <h2>Job Distribution</h2>
          <div style={{ height: '200px' }}>
            <Bar 
              data={jobStatusData} 
              options={{
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                  legend: {
                    display: false
                  }
                }
              }} 
            />
          </div>
        </div>
      </div>

      <div className="card">
        <h2>Recent Jobs</h2>
        <table className="data-table">
          <thead>
            <tr>
              <th>Job Name</th>
              <th>Status</th>
              <th>Fuzzer</th>
              <th>Bot</th>
              <th>Started</th>
              <th>Duration</th>
            </tr>
          </thead>
          <tbody>
            {jobs.slice(0, 10).map(job => (
              <tr key={job.id}>
                <td>{job.name}</td>
                <td>
                  <span className={`status-badge ${job.status}`}>
                    {job.status}
                  </span>
                </td>
                <td>{job.fuzzer}</td>
                <td>{job.bot_id || '-'}</td>
                <td>
                  {job.started_at 
                    ? new Date(job.started_at).toLocaleString()
                    : '-'
                  }
                </td>
                <td>
                  {job.started_at && job.completed_at
                    ? formatDuration(
                        new Date(job.completed_at).getTime() - 
                        new Date(job.started_at).getTime()
                      )
                    : '-'
                  }
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
};

function formatDuration(ms: number): string {
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);

  if (hours > 0) {
    return `${hours}h ${minutes % 60}m`;
  } else if (minutes > 0) {
    return `${minutes}m ${seconds % 60}s`;
  } else {
    return `${seconds}s`;
  }
}

export default CampaignDetails;