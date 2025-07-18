import React, { useState, useEffect } from 'react';
import { Bar, Radar } from 'react-chartjs-2';
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  BarElement,
  RadialLinearScale,
  PointElement,
  LineElement,
  Filler,
  Title,
  Tooltip,
  Legend
} from 'chart.js';
import ApiService from '../services/api';
import { FuzzerPerformance as FuzzerPerformanceType } from '../types';

ChartJS.register(
  CategoryScale,
  LinearScale,
  BarElement,
  RadialLinearScale,
  PointElement,
  LineElement,
  Filler,
  Title,
  Tooltip,
  Legend
);

const FuzzerPerformance: React.FC = () => {
  const [performanceData, setPerformanceData] = useState<Map<string, FuzzerPerformanceType>>(new Map());
  const [loading, setLoading] = useState(true);
  const [selectedPeriod, setSelectedPeriod] = useState('24h');

  const api = new ApiService();
  const fuzzerTypes = ['afl++', 'libfuzzer', 'honggfuzz'];

  useEffect(() => {
    loadPerformanceData();
  }, [selectedPeriod]);

  const loadPerformanceData = async () => {
    try {
      setLoading(true);
      const perfMap = new Map<string, FuzzerPerformanceType>();
      
      for (const fuzzer of fuzzerTypes) {
        try {
          const perf = await api.getFuzzerPerformance(fuzzer, selectedPeriod);
          perfMap.set(fuzzer, perf);
        } catch (err) {
          console.error(`Failed to load performance for ${fuzzer}:`, err);
        }
      }
      
      setPerformanceData(perfMap);
    } catch (error) {
      console.error('Failed to load fuzzer performance:', error);
    } finally {
      setLoading(false);
    }
  };

  if (loading) return <div className="loading">Loading fuzzer performance...</div>;

  // Prepare comparison chart data
  const comparisonData = {
    labels: Array.from(performanceData.keys()),
    datasets: [
      {
        label: 'Success Rate (%)',
        data: Array.from(performanceData.values()).map(p => 
          p.total_jobs > 0 ? (p.successful_jobs / p.total_jobs * 100) : 0
        ),
        backgroundColor: 'rgba(75, 192, 192, 0.8)',
      },
      {
        label: 'Efficiency Score',
        data: Array.from(performanceData.values()).map(p => p.efficiency_score),
        backgroundColor: 'rgba(54, 162, 235, 0.8)',
      },
      {
        label: 'Avg Exec Speed (K/s)',
        data: Array.from(performanceData.values()).map(p => p.average_exec_speed / 1000),
        backgroundColor: 'rgba(255, 206, 86, 0.8)',
      }
    ]
  };

  // Prepare radar chart data for multi-dimensional comparison
  const radarData = {
    labels: ['Coverage', 'Speed', 'Crashes', 'Efficiency', 'Success Rate', 'Resource Usage'],
    datasets: Array.from(performanceData.entries()).map(([fuzzer, perf], index) => {
      const colors = ['rgba(255, 99, 132', 'rgba(54, 162, 235', 'rgba(255, 206, 86'];
      const color = colors[index % colors.length];
      
      return {
        label: fuzzer,
        data: [
          normalize(perf.coverage_gain, 10000),
          normalize(perf.average_exec_speed, 50000),
          normalize(perf.crashes_found, 50),
          perf.efficiency_score,
          perf.total_jobs > 0 ? (perf.successful_jobs / perf.total_jobs * 100) : 0,
          100 - normalize(perf.resource_usage?.average_cpu || 0, 100)
        ],
        borderColor: `${color}, 1)`,
        backgroundColor: `${color}, 0.2)`,
      };
    })
  };

  return (
    <div>
      <div className="card">
        <div className="card-header">
          <h1>Fuzzer Performance Comparison</h1>
          <select 
            value={selectedPeriod} 
            onChange={(e) => setSelectedPeriod(e.target.value)}
            className="button secondary"
          >
            <option value="1h">Last Hour</option>
            <option value="6h">Last 6 Hours</option>
            <option value="24h">Last 24 Hours</option>
            <option value="7d">Last 7 Days</option>
          </select>
        </div>
      </div>

      <div className="stats-grid">
        {Array.from(performanceData.entries()).map(([fuzzer, perf]) => (
          <div key={fuzzer} className="card">
            <h3 style={{ textTransform: 'capitalize' }}>{fuzzer}</h3>
            <div className="stat-card">
              <div className="stat-label">Total Jobs</div>
              <div className="stat-value">{perf.total_jobs}</div>
              <div className="stat-change">
                {perf.successful_jobs} successful, {perf.failed_jobs} failed
              </div>
            </div>
            <div className="stat-card">
              <div className="stat-label">Crashes Found</div>
              <div className="stat-value">{perf.crashes_found}</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">Coverage Gain</div>
              <div className="stat-value">{perf.coverage_gain.toLocaleString()}</div>
              <div className="stat-change">edges</div>
            </div>
            <div className="stat-card">
              <div className="stat-label">Avg Runtime</div>
              <div className="stat-value">{formatRuntime(perf.average_runtime)}</div>
            </div>
          </div>
        ))}
      </div>

      <div className="card">
        <h2>Performance Metrics Comparison</h2>
        <div className="chart-container">
          <Bar 
            data={comparisonData} 
            options={{
              responsive: true,
              maintainAspectRatio: false,
              plugins: {
                legend: {
                  position: 'top' as const,
                },
                title: {
                  display: true,
                  text: 'Key Performance Indicators'
                }
              }
            }} 
          />
        </div>
      </div>

      <div className="card">
        <h2>Multi-Dimensional Analysis</h2>
        <div className="chart-container">
          <Radar 
            data={radarData} 
            options={{
              responsive: true,
              maintainAspectRatio: false,
              plugins: {
                legend: {
                  position: 'top' as const,
                },
                title: {
                  display: true,
                  text: 'Fuzzer Capabilities Overview'
                }
              },
              scales: {
                r: {
                  angleLines: {
                    display: false
                  },
                  suggestedMin: 0,
                  suggestedMax: 100
                }
              }
            }} 
          />
        </div>
      </div>

      <div className="card">
        <h2>Resource Utilization</h2>
        <table className="data-table">
          <thead>
            <tr>
              <th>Fuzzer</th>
              <th>Avg CPU (%)</th>
              <th>Avg Memory (MB)</th>
              <th>Peak Memory (MB)</th>
              <th>Disk Usage (MB)</th>
              <th>Network (MB/s)</th>
            </tr>
          </thead>
          <tbody>
            {Array.from(performanceData.entries()).map(([fuzzer, perf]) => (
              <tr key={fuzzer}>
                <td style={{ textTransform: 'capitalize' }}>{fuzzer}</td>
                <td>{perf.resource_usage?.average_cpu?.toFixed(1) || '-'}</td>
                <td>{((perf.resource_usage?.average_memory || 0) / 1024 / 1024).toFixed(0)}</td>
                <td>{((perf.resource_usage?.peak_memory || 0) / 1024 / 1024).toFixed(0)}</td>
                <td>{((perf.resource_usage?.disk_usage || 0) / 1024 / 1024).toFixed(0)}</td>
                <td>{((perf.resource_usage?.network_bandwidth || 0) / 1024 / 1024).toFixed(2)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
};

function normalize(value: number, max: number): number {
  return Math.min((value / max) * 100, 100);
}

function formatRuntime(runtime: string): string {
  // Parse duration string and format nicely
  // Assuming runtime is in a parseable format
  return runtime || '-';
}

export default FuzzerPerformance;