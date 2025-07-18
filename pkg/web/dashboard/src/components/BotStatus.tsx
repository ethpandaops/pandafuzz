import React, { useState, useEffect } from 'react';
import { Doughnut } from 'react-chartjs-2';
import {
  Chart as ChartJS,
  ArcElement,
  Tooltip,
  Legend
} from 'chart.js';
import ApiService from '../services/api';
import { Bot, Job } from '../types';

ChartJS.register(ArcElement, Tooltip, Legend);

const BotStatus: React.FC = () => {
  const [bots, setBots] = useState<Bot[]>([]);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [loading, setLoading] = useState(true);
  const [autoRefresh, setAutoRefresh] = useState(true);

  const api = new ApiService();

  useEffect(() => {
    loadBotData();
    
    if (autoRefresh) {
      const interval = setInterval(loadBotData, 5000); // Refresh every 5 seconds
      return () => clearInterval(interval);
    }
  }, [autoRefresh]);

  const loadBotData = async () => {
    try {
      const [botsData, jobsData] = await Promise.all([
        api.getBots(),
        api.getJobs(undefined, 'running')
      ]);
      
      setBots(botsData);
      setJobs(jobsData);
    } catch (error) {
      console.error('Failed to load bot data:', error);
    } finally {
      setLoading(false);
    }
  };

  const botStatusCounts = {
    idle: bots.filter(b => b.status === 'idle').length,
    busy: bots.filter(b => b.status === 'busy').length,
    offline: bots.filter(b => b.status === 'offline').length,
  };

  const statusChartData = {
    labels: ['Idle', 'Busy', 'Offline'],
    datasets: [{
      data: [botStatusCounts.idle, botStatusCounts.busy, botStatusCounts.offline],
      backgroundColor: [
        'rgba(75, 192, 192, 0.8)',
        'rgba(255, 206, 86, 0.8)',
        'rgba(255, 99, 132, 0.8)',
      ],
      borderColor: [
        'rgba(75, 192, 192, 1)',
        'rgba(255, 206, 86, 1)',
        'rgba(255, 99, 132, 1)',
      ],
      borderWidth: 1,
    }],
  };

  const calculateBotUtilization = () => {
    const totalBots = bots.length;
    const busyBots = botStatusCounts.busy;
    return totalBots > 0 ? (busyBots / totalBots * 100).toFixed(1) : '0';
  };

  const getJobForBot = (botId: string): Job | undefined => {
    return jobs.find(j => j.bot_id === botId);
  };

  const formatLastHeartbeat = (timestamp: string): string => {
    const now = new Date();
    const heartbeat = new Date(timestamp);
    const diffMs = now.getTime() - heartbeat.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    
    if (diffMins < 1) return 'Just now';
    if (diffMins < 60) return `${diffMins}m ago`;
    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return `${diffHours}h ago`;
    return `${Math.floor(diffHours / 24)}d ago`;
  };

  if (loading) return <div className="loading">Loading bot status...</div>;

  return (
    <div>
      <div className="card">
        <div className="card-header">
          <h1>Bot Fleet Status</h1>
          <div>
            <label style={{ marginRight: '1rem' }}>
              <input 
                type="checkbox" 
                checked={autoRefresh} 
                onChange={(e) => setAutoRefresh(e.target.checked)}
                style={{ marginRight: '0.5rem' }}
              />
              Auto-refresh
            </label>
            <button onClick={loadBotData} className="button secondary">
              Refresh Now
            </button>
          </div>
        </div>

        <div className="stats-grid">
          <div className="stat-card">
            <div className="stat-label">Total Bots</div>
            <div className="stat-value">{bots.length}</div>
            <div className="stat-change">in fleet</div>
          </div>

          <div className="stat-card">
            <div className="stat-label">Active Bots</div>
            <div className="stat-value">{botStatusCounts.busy}</div>
            <div className="stat-change positive">processing jobs</div>
          </div>

          <div className="stat-card">
            <div className="stat-label">Idle Bots</div>
            <div className="stat-value">{botStatusCounts.idle}</div>
            <div className="stat-change">available</div>
          </div>

          <div className="stat-card">
            <div className="stat-label">Utilization</div>
            <div className="stat-value">{calculateBotUtilization()}%</div>
            <div className="stat-change">fleet efficiency</div>
          </div>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '300px 1fr', gap: '1.5rem' }}>
        <div className="card">
          <h2>Status Distribution</h2>
          <div style={{ padding: '1rem' }}>
            <Doughnut 
              data={statusChartData} 
              options={{
                responsive: true,
                maintainAspectRatio: true,
                plugins: {
                  legend: {
                    position: 'bottom' as const,
                  }
                }
              }}
            />
          </div>
        </div>

        <div className="card">
          <h2>Bot Capabilities</h2>
          <CapabilityMatrix bots={bots} />
        </div>
      </div>

      <div className="card">
        <h2>Bot Details</h2>
        <table className="data-table">
          <thead>
            <tr>
              <th>Bot Name</th>
              <th>Hostname</th>
              <th>Status</th>
              <th>Current Job</th>
              <th>Capabilities</th>
              <th>Last Heartbeat</th>
            </tr>
          </thead>
          <tbody>
            {bots.map(bot => {
              const currentJob = getJobForBot(bot.id);
              return (
                <tr key={bot.id}>
                  <td>{bot.name}</td>
                  <td>{bot.hostname}</td>
                  <td>
                    <span className={`status-badge ${bot.status}`}>
                      {bot.status}
                    </span>
                  </td>
                  <td>
                    {currentJob ? (
                      <span>{currentJob.name}</span>
                    ) : (
                      <span style={{ color: '#999' }}>-</span>
                    )}
                  </td>
                  <td>
                    {bot.capabilities.map(cap => (
                      <span 
                        key={cap} 
                        className="status-badge"
                        style={{ 
                          marginRight: '0.25rem',
                          backgroundColor: '#e3f2fd',
                          color: '#1976d2'
                        }}
                      >
                        {cap}
                      </span>
                    ))}
                  </td>
                  <td>
                    {formatLastHeartbeat(bot.last_heartbeat)}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
};

const CapabilityMatrix: React.FC<{ bots: Bot[] }> = ({ bots }) => {
  // Extract all unique capabilities
  const allCapabilities = new Set<string>();
  bots.forEach(bot => bot.capabilities.forEach(cap => allCapabilities.add(cap)));
  const capabilities = Array.from(allCapabilities).sort();

  // Count bots per capability
  const capabilityCounts = capabilities.map(cap => ({
    capability: cap,
    total: bots.filter(bot => bot.capabilities.includes(cap)).length,
    active: bots.filter(bot => 
      bot.capabilities.includes(cap) && bot.status === 'busy'
    ).length,
    idle: bots.filter(bot => 
      bot.capabilities.includes(cap) && bot.status === 'idle'
    ).length,
  }));

  return (
    <table className="data-table">
      <thead>
        <tr>
          <th>Capability</th>
          <th>Total</th>
          <th>Active</th>
          <th>Idle</th>
          <th>Utilization</th>
        </tr>
      </thead>
      <tbody>
        {capabilityCounts.map(({ capability, total, active, idle }) => (
          <tr key={capability}>
            <td>{capability}</td>
            <td>{total}</td>
            <td>{active}</td>
            <td>{idle}</td>
            <td>
              <div style={{ 
                width: '100px', 
                height: '20px', 
                backgroundColor: '#e0e0e0',
                borderRadius: '10px',
                overflow: 'hidden'
              }}>
                <div style={{
                  width: `${total > 0 ? (active / total * 100) : 0}%`,
                  height: '100%',
                  backgroundColor: '#4caf50',
                  transition: 'width 0.3s ease'
                }} />
              </div>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
};

export default BotStatus;