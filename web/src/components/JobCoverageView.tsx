import React, { useEffect, useState } from 'react';
import {
  Box,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
  IconButton,
  Alert,
  CircularProgress,
} from '@mui/material';
import {
  Download as DownloadIcon,
} from '@mui/icons-material';
import { CoverageReport } from '../types/coverage';
import coverageAPI from '../api/coverage';
import { formatDateTime } from '../utils/dateFormat';

interface JobCoverageViewProps {
  jobId: string;
  onError?: (error: string) => void;
}

interface JobCoverageReport extends CoverageReport {
  jobId: string;
}

const JobCoverageView: React.FC<JobCoverageViewProps> = ({ jobId, onError }) => {
  const [reports, setReports] = useState<JobCoverageReport[]>([]);
  const [loading, setLoading] = useState(true);
  const [downloadingReports, setDownloadingReports] = useState<Set<string>>(new Set());
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchReports = async () => {
      try {
        setLoading(true);
        setError(null);
        
        // Fetch coverage reports for this job
        const coverageReports = await coverageAPI.fetchCoverageReports(jobId, {
          limit: 100,
          sort_by: 'created_at',
          sort_order: 'desc',
        });
        
        // Add jobId to each report (including raw format)
        const reportsWithJobId = coverageReports
          .map(report => ({
            ...report,
            jobId: jobId
          }));
        
        setReports(reportsWithJobId);
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to fetch coverage reports';
        setError(errorMessage);
        if (onError) {
          onError(errorMessage);
        }
      } finally {
        setLoading(false);
      }
    };

    fetchReports();
  }, [jobId, onError]);

  const handleDownloadReport = async (jobId: string, reportId: string, format?: string) => {
    try {
      setDownloadingReports(prev => new Set(prev).add(reportId));
      
      const blob = await coverageAPI.downloadCoverageReport(jobId, reportId);
      
      // Determine file extension based on format
      let extension = '.txt';
      if (format === 'json') extension = '.json';
      else if (format === 'lcov') extension = '.lcov';
      else if (format === 'html') extension = '.html';
      else if (format === 'raw') extension = '.zip';
      
      // Create download link
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `coverage-${jobId.substring(0, 8)}-${reportId.substring(0, 8)}${extension}`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      window.URL.revokeObjectURL(url);

    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to download coverage report';
      setError(errorMessage);
      if (onError) {
        onError(errorMessage);
      }
    } finally {
      setDownloadingReports(prev => {
        const newSet = new Set(prev);
        newSet.delete(reportId);
        return newSet;
      });
    }
  };


  if (loading) {
    return (
      <Box display="flex" justifyContent="center" py={4}>
        <CircularProgress />
      </Box>
    );
  }

  if (error && reports.length === 0) {
    return (
      <Alert severity="error" sx={{ mt: 2 }}>
        {error}
      </Alert>
    );
  }

  if (reports.length === 0) {
    return (
      <Box>
        <Typography variant="h6" gutterBottom>
          Coverage Reports
        </Typography>
        <TableContainer component={Paper}>
          <Table>
            <TableHead>
              <TableRow>
                <TableCell>Job ID</TableCell>
                <TableCell>Report ID</TableCell>
                <TableCell>Format</TableCell>
                <TableCell>Updated</TableCell>
                <TableCell align="center">Download</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              <TableRow>
                <TableCell colSpan={5} align="center">
                  <Typography variant="body2" color="text.secondary" py={2}>
                    No coverage reports available yet
                  </Typography>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </TableContainer>
      </Box>
    );
  }

  return (
    <Box>
      <Typography variant="h6" gutterBottom>
        Coverage Reports
      </Typography>

      {error && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Job ID</TableCell>
              <TableCell>Report ID</TableCell>
              <TableCell>Format</TableCell>
              <TableCell>Updated</TableCell>
              <TableCell align="center">Download</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {reports.map((report) => (
              <TableRow key={report.id}>
                <TableCell>
                  <Typography variant="body2" fontFamily="monospace">
                    {report.jobId}
                  </Typography>
                </TableCell>
                <TableCell>
                  <Typography variant="body2" fontFamily="monospace">
                    {report.id}
                  </Typography>
                </TableCell>
                <TableCell>
                  <Typography variant="body2" color={report.format === 'raw' ? 'primary' : 'inherit'}>
                    {report.format?.toUpperCase() || 'UNKNOWN'}
                  </Typography>
                </TableCell>
                <TableCell>
                  <Typography variant="body2">
                    {formatDateTime(report.created_at)}
                  </Typography>
                </TableCell>
                <TableCell align="center">
                  <IconButton
                    size="small"
                    onClick={() => handleDownloadReport(report.jobId, report.id, report.format)}
                    disabled={downloadingReports.has(report.id)}
                  >
                    {downloadingReports.has(report.id) ? (
                      <CircularProgress size={20} />
                    ) : (
                      <DownloadIcon />
                    )}
                  </IconButton>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
};

export default JobCoverageView;