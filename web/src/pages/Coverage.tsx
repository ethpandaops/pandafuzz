import React, { useEffect, useState, useCallback } from 'react';
import {
  Box,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  IconButton,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import {
  Download as DownloadIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';
import api from '../api/client';
import { CoverageReport } from '../types/coverage';
import { formatDateTime } from '../utils/dateFormat';

function Coverage() {
  const [allCoverageReports, setAllCoverageReports] = useState<CoverageReport[]>([]);
  const [loading, setLoading] = useState(true);
  const [downloading, setDownloading] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const fetchCoverageData = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      
      // Fetch all jobs
      const jobsData = await api.getJobs({ limit: 100 });
      
      // Fetch coverage reports for ALL jobs (not just those with a specific flag)
      // The backend will return empty arrays for jobs without coverage
      const allReports: CoverageReport[] = [];
      
      for (const job of jobsData) {
        try {
          const reports = await api.getCoverageReports(job.id);
          // Add job name to each report for context
          if (reports && reports.length > 0) {
            const reportsWithJobInfo = reports.map(report => ({
              ...report,
              jobName: job.name,
              jobId: job.id
            }));
            allReports.push(...reportsWithJobInfo);
          }
        } catch (err) {
          // Silently skip jobs without coverage reports
          console.debug(`No coverage for job ${job.id}:`, err);
        }
      }
      
      // Sort by timestamp descending
      allReports.sort((a, b) => 
        new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
      );
      
      setAllCoverageReports(allReports);
      
      if (allReports.length === 0) {
        setError('No coverage reports found. Coverage reports will appear here when jobs complete fuzzing runs.');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch coverage data');
      setAllCoverageReports([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchCoverageData();
    // Refresh every 30 seconds
    const interval = setInterval(fetchCoverageData, 30000);
    return () => clearInterval(interval);
  }, [fetchCoverageData]);

  const handleDownloadReport = async (report: CoverageReport) => {
    if (!report.job_id || !report.id) {
      console.error('Missing job_id or report id for download');
      return;
    }

    try {
      setDownloading(report.id);
      const blob = await api.downloadCoverageReport(report.job_id, report.id);
      
      // Create download link
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `coverage-${report.jobName || report.job_id}-${report.id}-${report.format || 'report'}`;
      document.body.appendChild(link);
      link.click();
      
      // Cleanup
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
    } catch (err) {
      console.error('Failed to download coverage report:', err);
      setError(`Failed to download report: ${err instanceof Error ? err.message : 'Unknown error'}`);
    } finally {
      setDownloading(null);
    }
  };

  const formatFileSize = (bytes: number): string => {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };


  if (loading && allCoverageReports.length === 0) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="50vh">
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={2}>
        <Typography variant="h4">Coverage Reports</Typography>
        <Box display="flex" gap={2} alignItems="center">
          <Chip 
            label={`${allCoverageReports.length} reports`}
            color="primary"
            variant="outlined"
          />
          <Tooltip title="Refresh">
            <IconButton onClick={fetchCoverageData} disabled={loading}>
              <RefreshIcon />
            </IconButton>
          </Tooltip>
        </Box>
      </Box>

      {error && (
        <Box mb={2}>
          <Typography color="error">{error}</Typography>
        </Box>
      )}

      {allCoverageReports.length === 0 ? (
        <Card>
          <CardContent>
            <Box display="flex" flexDirection="column" alignItems="center" py={4}>
              <Typography variant="h6" color="textSecondary" gutterBottom>
                No coverage reports available
              </Typography>
              <Typography variant="body2" color="textSecondary" textAlign="center">
                Coverage reports will appear here when jobs with coverage enabled complete fuzzing runs.
              </Typography>
            </Box>
          </CardContent>
        </Card>
      ) : (
        <TableContainer component={Paper}>
          <Table>
            <TableHead>
              <TableRow>
                <TableCell>Job Name</TableCell>
                <TableCell>Format</TableCell>
                <TableCell>Size</TableCell>
                <TableCell>Created</TableCell>
                <TableCell>Bot ID</TableCell>
                <TableCell align="center">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {allCoverageReports.map((report) => (
                <TableRow key={`${report.job_id}-${report.id}`} hover>
                  <TableCell>
                    <Box>
                      <Typography variant="body2" fontWeight="medium">
                        {report.jobName || report.job_id}
                      </Typography>
                      {report.edges && (
                        <Typography variant="caption" color="textSecondary">
                          {report.edges.toLocaleString()} edges
                          {report.new_edges && ` (+${report.new_edges.toLocaleString()} new)`}
                        </Typography>
                      )}
                    </Box>
                  </TableCell>
                  <TableCell>
                    <Chip 
                      label={report.format || 'unknown'} 
                      size="small" 
                      variant="outlined"
                    />
                  </TableCell>
                  <TableCell>
                    {formatFileSize(report.size || 0)}
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2">
                      {formatDateTime(report.created_at)}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" fontFamily="monospace" fontSize="0.8em">
                      {report.bot_id ? report.bot_id.substring(0, 8) + '...' : 'N/A'}
                    </Typography>
                  </TableCell>
                  <TableCell align="center">
                    <Tooltip title="Download coverage report">
                      <IconButton
                        onClick={() => handleDownloadReport(report)}
                        disabled={downloading === report.id}
                        size="small"
                      >
                        {downloading === report.id ? (
                          <CircularProgress size={20} />
                        ) : (
                          <DownloadIcon />
                        )}
                      </IconButton>
                    </Tooltip>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Box>
  );
}

export default Coverage;