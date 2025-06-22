import React, { useEffect, useState } from 'react';
import {
  Box,
  Card,
  CardContent,
  Grid,
  LinearProgress,
  Paper,
  TextField,
  Typography,
  MenuItem,
} from '@mui/material';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
  AreaChart,
  Area,
} from 'recharts';
import api from '../api/client';
import { CoverageResult } from '../types';

interface CoverageData {
  timestamp: string;
  coverage: number;
  edges: number;
  newEdges: number;
}

function Coverage() {
  const [coverageResults, setCoverageResults] = useState<CoverageResult[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedJob, setSelectedJob] = useState<string>('');
  const [jobs, setJobs] = useState<Array<{ id: string; name: string }>>([]);

  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true);
        
        // Fetch jobs for filter
        const jobsData = await api.getJobs({ limit: 100 });
        setJobs(jobsData.map((j) => ({ id: j.id, name: j.name })));
        
        // Fetch coverage results
        const params = selectedJob ? { job_id: selectedJob } : undefined;
        const data = await api.getCoverageResults({ ...params, limit: 1000 });
        setCoverageResults(data);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch coverage data');
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, [selectedJob]);

  if (loading && coverageResults.length === 0) {
    return <LinearProgress />;
  }

  if (error) {
    return (
      <Box p={2}>
        <Typography color="error">Error: {error}</Typography>
      </Box>
    );
  }

  // Process data for charts
  const processedData = coverageResults
    .sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime())
    .map((result) => ({
      timestamp: new Date(result.timestamp).toLocaleTimeString(),
      coverage: result.coverage_percent,
      edges: result.covered_edges,
      newEdges: result.new_edges,
    }));

  // Calculate statistics
  const latestCoverage = coverageResults.length > 0
    ? coverageResults[coverageResults.length - 1].coverage_percent
    : 0;
  
  const totalEdges = coverageResults.length > 0
    ? Math.max(...coverageResults.map((r) => r.edges))
    : 0;
  
  const coveredEdges = coverageResults.length > 0
    ? Math.max(...coverageResults.map((r) => r.covered_edges))
    : 0;
  
  const totalNewEdges = coverageResults.reduce((sum, r) => sum + r.new_edges, 0);

  // Group by job for job-specific stats
  const jobStats = coverageResults.reduce((acc, result) => {
    if (!acc[result.job_id]) {
      acc[result.job_id] = {
        jobId: result.job_id,
        botId: result.bot_id,
        maxCoverage: 0,
        totalNewEdges: 0,
        dataPoints: 0,
      };
    }
    acc[result.job_id].maxCoverage = Math.max(
      acc[result.job_id].maxCoverage,
      result.coverage_percent
    );
    acc[result.job_id].totalNewEdges += result.new_edges;
    acc[result.job_id].dataPoints++;
    return acc;
  }, {} as Record<string, any>);

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={2}>
        <Typography variant="h4">Coverage Analysis</Typography>
        <TextField
          select
          label="Filter by Job"
          value={selectedJob}
          onChange={(e) => setSelectedJob(e.target.value)}
          size="small"
          sx={{ minWidth: 200 }}
        >
          <MenuItem value="">All Jobs</MenuItem>
          {jobs.map((job) => (
            <MenuItem key={job.id} value={job.id}>
              {job.name}
            </MenuItem>
          ))}
        </TextField>
      </Box>

      {coverageResults.length === 0 ? (
        <Card>
          <CardContent>
            <Box display="flex" flexDirection="column" alignItems="center" py={4}>
              <Typography variant="h6" color="textSecondary" gutterBottom>
                No coverage data available yet
              </Typography>
              <Typography variant="body2" color="textSecondary">
                Coverage data will appear here as fuzzing jobs run
              </Typography>
            </Box>
          </CardContent>
        </Card>
      ) : (
      <Grid container spacing={3}>
        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom>
                Current Coverage
              </Typography>
              <Typography variant="h4">
                {latestCoverage.toFixed(1)}%
              </Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom>
                Covered Edges
              </Typography>
              <Typography variant="h4">
                {coveredEdges.toLocaleString()}
              </Typography>
              <Typography variant="body2" color="textSecondary">
                of {totalEdges.toLocaleString()}
              </Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom>
                New Edges Found
              </Typography>
              <Typography variant="h4">{totalNewEdges.toLocaleString()}</Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom>
                Active Jobs
              </Typography>
              <Typography variant="h4">
                {Object.keys(jobStats).length}
              </Typography>
            </CardContent>
          </Card>
        </Grid>

        <Grid item xs={12}>
          <Paper sx={{ p: 2 }}>
            <Typography variant="h6" gutterBottom>
              Coverage Over Time
            </Typography>
            <ResponsiveContainer width="100%" height={300}>
              <AreaChart data={processedData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="timestamp" />
                <YAxis domain={[0, 100]} />
                <Tooltip />
                <Legend />
                <Area
                  type="monotone"
                  dataKey="coverage"
                  stroke="#8884d8"
                  fill="#8884d8"
                  fillOpacity={0.6}
                  name="Coverage %"
                />
              </AreaChart>
            </ResponsiveContainer>
          </Paper>
        </Grid>

        <Grid item xs={12}>
          <Paper sx={{ p: 2 }}>
            <Typography variant="h6" gutterBottom>
              Edge Discovery Rate
            </Typography>
            <ResponsiveContainer width="100%" height={300}>
              <LineChart data={processedData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="timestamp" />
                <YAxis />
                <Tooltip />
                <Legend />
                <Line
                  type="monotone"
                  dataKey="edges"
                  stroke="#82ca9d"
                  name="Total Edges"
                />
                <Line
                  type="monotone"
                  dataKey="newEdges"
                  stroke="#ffc658"
                  name="New Edges"
                />
              </LineChart>
            </ResponsiveContainer>
          </Paper>
        </Grid>

        {Object.values(jobStats).length > 0 && (
          <Grid item xs={12}>
            <Paper sx={{ p: 2 }}>
              <Typography variant="h6" gutterBottom>
                Job Performance Summary
              </Typography>
              <Box sx={{ overflowX: 'auto' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ borderBottom: '2px solid #333' }}>
                      <th style={{ padding: '8px', textAlign: 'left' }}>Job ID</th>
                      <th style={{ padding: '8px', textAlign: 'left' }}>Bot ID</th>
                      <th style={{ padding: '8px', textAlign: 'right' }}>Max Coverage</th>
                      <th style={{ padding: '8px', textAlign: 'right' }}>New Edges</th>
                      <th style={{ padding: '8px', textAlign: 'right' }}>Data Points</th>
                    </tr>
                  </thead>
                  <tbody>
                    {Object.values(jobStats).map((stat: any) => (
                      <tr key={stat.jobId} style={{ borderBottom: '1px solid #555' }}>
                        <td style={{ padding: '8px', fontFamily: 'monospace' }}>
                          {stat.jobId.substring(0, 8)}...
                        </td>
                        <td style={{ padding: '8px', fontFamily: 'monospace' }}>
                          {stat.botId.substring(0, 8)}...
                        </td>
                        <td style={{ padding: '8px', textAlign: 'right' }}>
                          {stat.maxCoverage.toFixed(1)}%
                        </td>
                        <td style={{ padding: '8px', textAlign: 'right' }}>
                          {stat.totalNewEdges.toLocaleString()}
                        </td>
                        <td style={{ padding: '8px', textAlign: 'right' }}>
                          {stat.dataPoints}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </Box>
            </Paper>
          </Grid>
        )}
      </Grid>
      )}
    </Box>
  );
}

export default Coverage;