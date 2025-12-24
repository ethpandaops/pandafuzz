import React, { useEffect, useState, useCallback } from 'react';
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  LinearProgress,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
  Grid,
  Alert,
  Tooltip,
  Snackbar,
  Tab,
} from '@mui/material';
import {
  Add as AddIcon,
  Cancel as CancelIcon,
  Delete as DeleteIcon,
  Info as InfoIcon,
  Description as LogsIcon,
  CheckCircle as SuccessIcon,
  Error as ErrorIcon,
  Schedule as PendingIcon,
  PlayArrow as RunningIcon,
  Refresh as RefreshIcon,
  RestoreFromTrash as RecoverIcon,
} from '@mui/icons-material';
import api from '../api/client';
import { Job, JobStatus, JobPriority, Bot, JobCoverageConfig } from '../types';
import { SortableTableHeader, useSort } from '../components/SortableTableHeader';
import JobCreationForm, { JobFormData } from '../components/JobCreationForm';
import JobCoverageView from '../components/JobCoverageView';
import { formatDateTime, formatDuration as formatDurationUtil, formatTime } from '../utils/dateFormat';
import TabContext from '@mui/lab/TabContext';
import TabList from '@mui/lab/TabList';
import TabPanel from '@mui/lab/TabPanel';
import AssessmentIcon from '@mui/icons-material/Assessment';

const statusColors: Record<JobStatus, 'default' | 'primary' | 'success' | 'error' | 'warning'> = {
  [JobStatus.Pending]: 'default',
  [JobStatus.Assigned]: 'primary',
  [JobStatus.Running]: 'primary',
  [JobStatus.Completed]: 'success',
  [JobStatus.Failed]: 'error',
  [JobStatus.Cancelled]: 'warning',
};

const statusIcons: Record<JobStatus, React.ReactNode> = {
  [JobStatus.Pending]: <PendingIcon fontSize="small" />,
  [JobStatus.Assigned]: <PendingIcon fontSize="small" />,
  [JobStatus.Running]: <RunningIcon fontSize="small" />,
  [JobStatus.Completed]: <SuccessIcon fontSize="small" />,
  [JobStatus.Failed]: <ErrorIcon fontSize="small" />,
  [JobStatus.Cancelled]: <CancelIcon fontSize="small" />,
};

const priorityColors: Record<JobPriority, 'default' | 'primary' | 'error'> = {
  [JobPriority.Low]: 'default',
  [JobPriority.Normal]: 'primary',
  [JobPriority.High]: 'error',
};

function Jobs() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [bots, setBots] = useState<Bot[]>([]);
  const [loading, setLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedJob, setSelectedJob] = useState<Job | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [jobToDelete, setJobToDelete] = useState<Job | null>(null);
  const [recoveryDialogOpen, setRecoveryDialogOpen] = useState(false);
  const [recoveryLoading, setRecoveryLoading] = useState(false);
  const [logsDialogOpen, setLogsDialogOpen] = useState(false);
  const [logsJob, setLogsJob] = useState<Job | null>(null);
  const [jobLogs, setJobLogs] = useState<any>(null);
  const [logsLoading, setLogsLoading] = useState(false);
  
  // Sort state - default to created_at desc
  const { sortState, handleSort } = useSort({ key: 'created_at', direction: 'desc' });
  
  // Snackbar state
  const [snackbar, setSnackbar] = useState<{
    open: boolean;
    message: string;
    severity: 'success' | 'error' | 'info' | 'warning';
  }>({ open: false, message: '', severity: 'info' });
  
  // Job creation state
  const [createJobLoading, setCreateJobLoading] = useState(false);
  
  // Corpus collections state
  const [corpusCollections, setCorpusCollections] = useState<any[]>([]);
  
  // Tab state for job details dialog
  const [detailsTabValue, setDetailsTabValue] = useState('details');

  const fetchJobs = useCallback(async (showLoading = true) => {
    try {
      if (showLoading) {
        setLoading(true);
      } else {
        setIsRefreshing(true);
      }
      const data = await api.getJobs({ 
        limit: 100,
        sort_by: sortState?.key,
        sort_order: sortState?.direction
      });
      
      // Only update if data has actually changed
      setJobs(prevJobs => {
        // Simple comparison - could be more sophisticated
        if (JSON.stringify(prevJobs) === JSON.stringify(data)) {
          return prevJobs;
        }
        return data;
      });
      
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch jobs');
    } finally {
      if (showLoading) {
        setLoading(false);
      } else {
        setIsRefreshing(false);
      }
    }
  }, [sortState]);

  const fetchBots = async () => {
    try {
      const data = await api.getBots();
      setBots(data);
    } catch (err) {
      console.error('Failed to fetch bots:', err);
    }
  };

  const fetchCorpusCollections = async () => {
    try {
      const response = await fetch('/api/v1/corpus/collections');
      if (response.ok) {
        const data = await response.json();
        setCorpusCollections(data.collections || []);
      }
    } catch (err) {
      console.error('Failed to fetch corpus collections:', err);
    }
  };


  useEffect(() => {
    // Initial load with loading indicator
    fetchJobs(true);
    fetchBots();
    fetchCorpusCollections();
    
    // Subsequent updates without loading indicator
    const interval = setInterval(() => {
      fetchJobs(false);
    }, 10000); // Update every 10 seconds instead of 5
    
    return () => clearInterval(interval);
  }, [fetchJobs]);
  
  // Refetch when sort changes
  useEffect(() => {
    if (sortState) {
      fetchJobs(false);
    }
  }, [sortState, fetchJobs]);

  const getBotName = useCallback((botId: string | undefined): string => {
    if (!botId) return '-';
    const bot = bots.find(b => b.id === botId);
    return bot ? bot.name : botId;
  }, [bots]);

  const handleCreateJob = async (
    formData: JobFormData,
    useFileUpload: boolean,
    targetBinaryFile: File | null,
    seedCorpusFiles: File[]
  ) => {
    try {
      setCreateJobLoading(true);
      
      const jobData = {
        name: formData.name,
        fuzzer: formData.fuzzer,
        target: formData.target,
        target_args: formData.target_args.split(' ').filter(arg => arg),
        priority: formData.priority,
        timeout_sec: formData.timeout_sec,
        memory_limit: formData.memory_limit,
        collection_id: formData.collection_id || undefined,
        // Add coverage configuration
        ...(formData.enableCoverage && {
          config: {
            coverage: {
              enabled: true,
              format: formData.coverageFormat,
            } as JobCoverageConfig
          }
        })
      };
      
      if (useFileUpload && targetBinaryFile) {
        // Use upload endpoint
        await api.createJobWithUpload(jobData, targetBinaryFile, seedCorpusFiles);
      } else {
        // Use regular endpoint
        await api.createJob(jobData);
      }
      
      setCreateDialogOpen(false);
      setSnackbar({
        open: true,
        message: 'Job created successfully!',
        severity: 'success',
      });
      fetchJobs(false);
    } catch (err) {
      setSnackbar({
        open: true,
        message: err instanceof Error ? err.message : 'Failed to create job',
        severity: 'error',
      });
    } finally {
      setCreateJobLoading(false);
    }
  };

  const handleCancelJob = async (jobId: string) => {
    try {
      await api.cancelJob(jobId);
      setSnackbar({
        open: true,
        message: 'Job cancelled successfully',
        severity: 'info',
      });
      fetchJobs(false);
    } catch (err) {
      setSnackbar({
        open: true,
        message: err instanceof Error ? err.message : 'Failed to cancel job',
        severity: 'error',
      });
    }
  };

  const handleDeleteJob = async () => {
    if (!jobToDelete) return;
    
    try {
      await api.deleteJob(jobToDelete.id);
      setDeleteDialogOpen(false);
      setJobToDelete(null);
      setSnackbar({
        open: true,
        message: 'Job deleted successfully',
        severity: 'success',
      });
      fetchJobs(false);
    } catch (err) {
      setSnackbar({
        open: true,
        message: err instanceof Error ? err.message : 'Failed to delete job',
        severity: 'error',
      });
    }
  };

  const handleRecoverJobs = async () => {
    try {
      setRecoveryLoading(true);
      const result = await api.recoverOrphanedJobs();
      setRecoveryDialogOpen(false);
      setSnackbar({
        open: true,
        message: `${result.message} (${result.recovered_count} jobs recovered)`,
        severity: result.recovered_count > 0 ? 'success' : 'info',
      });
      // Refresh job list to show recovered jobs
      fetchJobs(false);
    } catch (err) {
      setSnackbar({
        open: true,
        message: err instanceof Error ? err.message : 'Failed to recover jobs',
        severity: 'error',
      });
    } finally {
      setRecoveryLoading(false);
    }
  };

  const handleViewLogs = async (job: Job) => {
    setLogsJob(job);
    setLogsDialogOpen(true);
    setLogsLoading(true);
    
    try {
      const logsData = await api.getJobLogs(job.id, 1000);
      setJobLogs(logsData);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch job logs');
      setJobLogs(null);
    } finally {
      setLogsLoading(false);
    }
  };

  const formatDuration = (start?: string, end?: string) => {
    if (!start) return '-';
    return formatDurationUtil(start, end);
  };

  if (loading && jobs.length === 0) {
    return <LinearProgress />;
  }

  if (error) {
    return (
      <Box p={2}>
        <Typography color="error">Error: {error}</Typography>
      </Box>
    );
  }

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={2}>
        <Box display="flex" alignItems="center" gap={2}>
          <Typography variant="h4">Fuzzing Jobs</Typography>
          {isRefreshing && (
            <CircularProgress size={20} sx={{ opacity: 0.5 }} />
          )}
        </Box>
        <Box display="flex" gap={2}>
          <Button
            variant="outlined"
            startIcon={<RecoverIcon />}
            onClick={() => setRecoveryDialogOpen(true)}
            color="warning"
          >
            Recover Jobs
          </Button>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => {
              setCreateDialogOpen(true);
              setError(null);
            }}
          >
            New Job
          </Button>
        </Box>
      </Box>

      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <SortableTableHeader
                label="Name"
                sortKey="name"
                currentSort={sortState}
                onSort={handleSort}
              />
              <SortableTableHeader
                label="Status"
                sortKey="status"
                currentSort={sortState}
                onSort={handleSort}
              />
              <SortableTableHeader
                label="Priority"
                sortKey="priority"
                currentSort={sortState}
                onSort={handleSort}
              />
              <SortableTableHeader
                label="Fuzzer"
                sortKey="fuzzer"
                currentSort={sortState}
                onSort={handleSort}
              />
              <TableCell>Target</TableCell>
              <TableCell>Bot</TableCell>
              <TableCell>Coverage</TableCell>
              <SortableTableHeader
                label="Created"
                sortKey="created_at"
                currentSort={sortState}
                onSort={handleSort}
              />
              <TableCell>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {jobs.map((job) => (
              <TableRow key={job.id}>
                <TableCell>{job.name}</TableCell>
                <TableCell>
                  <Chip
                    label={job.status}
                    size="small"
                    color={statusColors[job.status]}
                  />
                </TableCell>
                <TableCell>
                  {job.priority ? (
                    <Chip
                      label={job.priority}
                      size="small"
                      color={priorityColors[job.priority]}
                    />
                  ) : (
                    '-'
                  )}
                </TableCell>
                <TableCell>{job.fuzzer}</TableCell>
                <TableCell>
                  <Typography variant="body2" noWrap style={{ maxWidth: 200 }}>
                    {job.target}
                  </Typography>
                </TableCell>
                <TableCell>
                  {job.assigned_bot ? (
                    <Chip label={getBotName(job.assigned_bot)} size="small" />
                  ) : (
                    '-'
                  )}
                </TableCell>
                <TableCell>
                  {job.config?.coverage?.enabled ? (
                    <Tooltip title={`Format: ${job.config.coverage.format.toUpperCase()}`}>
                      <Chip 
                        icon={<AssessmentIcon />}
                        label="Enabled" 
                        size="small" 
                        color="primary" 
                        variant="outlined" 
                      />
                    </Tooltip>
                  ) : (
                    '-'
                  )}
                </TableCell>
                <TableCell>
                  {formatDuration(job.started_at, job.completed_at)}
                </TableCell>
                <TableCell>
                  <IconButton
                    size="small"
                    onClick={async () => {
                      // Fetch fresh job data to get coverage fields
                      try {
                        const freshJob = await api.getJob(job.id);
                        setSelectedJob(freshJob);
                      } catch (err) {
                        // Fallback to cached job if fetch fails
                        setSelectedJob(job);
                      }
                    }}
                    title="View details"
                  >
                    <InfoIcon />
                  </IconButton>
                  {job.status === JobStatus.Running && (
                    <IconButton
                      size="small"
                      onClick={() => handleCancelJob(job.id)}
                      title="Cancel job"
                    >
                      <CancelIcon />
                    </IconButton>
                  )}
                  {[JobStatus.Completed, JobStatus.Failed, JobStatus.Cancelled].includes(job.status) && (
                      <>
                        <Tooltip title="View logs">
                          <IconButton
                            size="small"
                            onClick={(e) => {
                              e.stopPropagation();
                              handleViewLogs(job);
                            }}
                          >
                            <LogsIcon />
                          </IconButton>
                        </Tooltip>
                        <Tooltip title="Delete job">
                          <IconButton
                            size="small"
                            onClick={(e) => {
                              e.stopPropagation();
                              setJobToDelete(job);
                              setDeleteDialogOpen(true);
                            }}
                          >
                            <DeleteIcon />
                          </IconButton>
                        </Tooltip>
                      </>
                    )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>

      {/* Create Job Dialog */}
      <Dialog
        open={createDialogOpen}
        onClose={() => setCreateDialogOpen(false)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>Create New Job</DialogTitle>
        <DialogContent>
          <JobCreationForm
            corpusCollections={corpusCollections}
            onSubmit={handleCreateJob}
            onCancel={() => setCreateDialogOpen(false)}
            error={error}
            loading={createJobLoading}
          />
        </DialogContent>
      </Dialog>

      {/* Job Details Dialog */}
      <Dialog
        open={selectedJob !== null}
        onClose={() => {
          setSelectedJob(null);
          setDetailsTabValue('details');
        }}
        maxWidth="lg"
        fullWidth
      >
        <DialogTitle>
          <Box display="flex" justifyContent="space-between" alignItems="center">
            Job Details: {selectedJob?.name}
            {selectedJob?.config?.coverage?.enabled && (
              <Chip
                icon={<AssessmentIcon />}
                label="Coverage Enabled"
                size="small"
                color="primary"
                variant="outlined"
              />
            )}
          </Box>
        </DialogTitle>
        <DialogContent>
          {selectedJob && (
            <TabContext value={detailsTabValue}>
              <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
                <TabList onChange={(e, newValue) => setDetailsTabValue(newValue)}>
                  <Tab label="Details" value="details" />
                  <Tab 
                    label="Coverage Reports" 
                    value="coverage"
                    icon={<AssessmentIcon />}
                    iconPosition="start"
                  />
                </TabList>
              </Box>
              <TabPanel value="details" sx={{ px: 0, pt: 3 }}>
                <Grid container spacing={2}>
                <Grid item xs={6}>
                  <Typography variant="subtitle2" color="textSecondary">
                    ID
                  </Typography>
                  <Typography variant="body1" gutterBottom>
                    {selectedJob.id}
                  </Typography>
                </Grid>
                <Grid item xs={6}>
                  <Typography variant="subtitle2" color="textSecondary">
                    Status
                  </Typography>
                  <Typography variant="body1" gutterBottom>
                    <Chip
                      label={selectedJob.status}
                      color={statusColors[selectedJob.status]}
                      size="small"
                    />
                  </Typography>
                </Grid>
                <Grid item xs={6}>
                  <Typography variant="subtitle2" color="textSecondary">
                    Fuzzer
                  </Typography>
                  <Typography variant="body1" gutterBottom>
                    {selectedJob.fuzzer}
                  </Typography>
                </Grid>
                {selectedJob.priority && (
                  <Grid item xs={6}>
                    <Typography variant="subtitle2" color="textSecondary">
                      Priority
                    </Typography>
                    <Typography variant="body1" gutterBottom>
                      <Chip
                        label={selectedJob.priority}
                        color={priorityColors[selectedJob.priority]}
                        size="small"
                      />
                    </Typography>
                  </Grid>
                )}
                <Grid item xs={12}>
                  <Typography variant="subtitle2" color="textSecondary">
                    Target
                  </Typography>
                  <Typography variant="body1" gutterBottom>
                    {selectedJob.target}
                  </Typography>
                </Grid>
                {selectedJob.target_args && selectedJob.target_args.length > 0 && (
                  <Grid item xs={12}>
                    <Typography variant="subtitle2" color="textSecondary">
                      Arguments
                    </Typography>
                    <Typography variant="body1" gutterBottom>
                      {selectedJob.target_args.join(' ')}
                    </Typography>
                  </Grid>
                )}
                <Grid item xs={6}>
                  <Typography variant="subtitle2" color="textSecondary">
                    Created
                  </Typography>
                  <Typography variant="body1" gutterBottom>
                    {formatDateTime(selectedJob.created_at)}
                  </Typography>
                </Grid>
                <Grid item xs={6}>
                  <Typography variant="subtitle2" color="textSecondary">
                    Timeout
                  </Typography>
                  <Typography variant="body1" gutterBottom>
                    {formatDateTime(selectedJob.timeout_at)}
                  </Typography>
                </Grid>
                {selectedJob.assigned_bot && (
                  <Grid item xs={6}>
                    <Typography variant="subtitle2" color="textSecondary">
                      Assigned Bot
                    </Typography>
                    <Typography variant="body1" gutterBottom>
                      {getBotName(selectedJob.assigned_bot)}
                    </Typography>
                  </Grid>
                )}
                {selectedJob.message && (
                  <Grid item xs={12}>
                    <Typography variant="subtitle2" color="textSecondary">
                      Message
                    </Typography>
                    <Typography variant="body1" gutterBottom>
                      {selectedJob.message}
                    </Typography>
                  </Grid>
                )}
                </Grid>
              </TabPanel>
              <TabPanel value="coverage" sx={{ px: 0, pt: 3 }}>
                <JobCoverageView 
                  jobId={selectedJob.id}
                  onError={(error) => {
                    setSnackbar({
                      open: true,
                      message: error,
                      severity: 'error',
                    });
                  }}
                />
              </TabPanel>
            </TabContext>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setSelectedJob(null)}>Close</Button>
        </DialogActions>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog
        open={deleteDialogOpen}
        onClose={() => setDeleteDialogOpen(false)}
      >
        <DialogTitle>Confirm Delete</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to delete job "{jobToDelete?.name}"?
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteDialogOpen(false)}>Cancel</Button>
          <Button onClick={handleDeleteJob} color="error">
            Delete
          </Button>
        </DialogActions>
      </Dialog>

      {/* Logs Dialog */}
      <Dialog
        open={logsDialogOpen}
        onClose={() => setLogsDialogOpen(false)}
        maxWidth="lg"
        fullWidth
      >
        <DialogTitle>
          <Box display="flex" justifyContent="space-between" alignItems="center">
            <Typography variant="h6">Job Logs - {logsJob?.name}</Typography>
            {jobLogs && jobLogs.has_more && jobLogs.logs && (
              <Typography variant="body2" color="textSecondary">
                Showing {jobLogs.logs.length} of {jobLogs.total_lines} lines
              </Typography>
            )}
          </Box>
        </DialogTitle>
        <DialogContent>
          {logsLoading ? (
            <Box display="flex" justifyContent="center" p={4}>
              <CircularProgress />
            </Box>
          ) : jobLogs && jobLogs.logs && jobLogs.logs.length > 0 ? (
            <Box
              sx={{
                backgroundColor: '#1e1e1e',
                borderRadius: 1,
                p: 2,
                fontFamily: 'monospace',
                fontSize: '0.875rem',
                maxHeight: '60vh',
                overflow: 'auto',
              }}
            >
              {jobLogs.logs.map((log: any, index: number) => (
                <Box key={index} mb={0.5}>
                  <span style={{ color: '#666' }}>
                    [{formatTime(log.timestamp)}]
                  </span>{' '}
                  <span
                    style={{
                      color:
                        log.level === 'error'
                          ? '#f44336'
                          : log.level === 'warning'
                          ? '#ff9800'
                          : log.level === 'info'
                          ? '#2196f3'
                          : '#888',
                    }}
                  >
                    [{log.level}]
                  </span>{' '}
                  <span style={{ color: '#4caf50' }}>[{log.source}]</span>{' '}
                  <span style={{ color: '#fff' }}>{log.message}</span>
                </Box>
              ))}
              {jobLogs.has_more && (
                <Box mt={2} textAlign="center">
                  <Button
                    variant="outlined"
                    size="small"
                    onClick={async () => {
                      setLogsLoading(true);
                      try {
                        const moreLogsData = await api.getJobLogs(
                          logsJob!.id,
                          1000,
                          jobLogs.next_offset
                        );
                        setJobLogs({
                          ...moreLogsData,
                          logs: [...jobLogs.logs, ...moreLogsData.logs],
                        });
                      } catch (err) {
                        setError(err instanceof Error ? err.message : 'Failed to load more logs');
                      } finally {
                        setLogsLoading(false);
                      }
                    }}
                  >
                    Load More
                  </Button>
                </Box>
              )}
            </Box>
          ) : (
            <Box textAlign="center" py={4}>
              <Typography color="textSecondary">
                {logsJob && 
                 (logsJob.status === JobStatus.Running || 
                  logsJob.status === JobStatus.Pending || 
                  logsJob.status === JobStatus.Assigned)
                  ? 'Job is still running. Logs will be available after completion.'
                  : 'No logs available for this job.'}
              </Typography>
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setLogsDialogOpen(false)}>Close</Button>
          {jobLogs && jobLogs.logs && jobLogs.logs.length > 0 && (
            <Button
              variant="contained"
              onClick={() => {
                const logText = jobLogs.logs
                  .map(
                    (log: any) =>
                      `[${formatDateTime(log.timestamp)}] [${log.level}] [${log.source}] ${log.message}`
                  )
                  .join('\n');
                const blob = new Blob([logText], { type: 'text/plain' });
                const url = window.URL.createObjectURL(blob);
                const a = document.createElement('a');
                a.href = url;
                a.download = `${logsJob?.name || 'job'}-logs.txt`;
                a.click();
                window.URL.revokeObjectURL(url);
              }}
            >
              Download Logs
            </Button>
          )}
        </DialogActions>
      </Dialog>
      
      {/* Job Recovery Confirmation Dialog */}
      <Dialog
        open={recoveryDialogOpen}
        onClose={() => !recoveryLoading && setRecoveryDialogOpen(false)}
      >
        <DialogTitle>Recover Orphaned Jobs</DialogTitle>
        <DialogContent>
          <Typography paragraph>
            This action will recover jobs that may have been orphaned due to bot disconnections 
            or system issues. Orphaned jobs will be reset to pending status and made available 
            for reassignment.
          </Typography>
          <Typography variant="body2" color="textSecondary">
            <strong>Note:</strong> This operation is safe to run and will only affect jobs that 
            are genuinely orphaned (assigned to bots that are no longer active).
          </Typography>
          {recoveryLoading && (
            <Box display="flex" alignItems="center" gap={2} mt={2}>
              <CircularProgress size={20} />
              <Typography variant="body2">Recovering jobs...</Typography>
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          <Button 
            onClick={() => setRecoveryDialogOpen(false)}
            disabled={recoveryLoading}
          >
            Cancel
          </Button>
          <Button 
            onClick={handleRecoverJobs} 
            color="warning"
            variant="contained"
            disabled={recoveryLoading}
            startIcon={recoveryLoading ? <CircularProgress size={16} /> : <RecoverIcon />}
          >
            {recoveryLoading ? 'Recovering...' : 'Recover Jobs'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Snackbar for notifications */}
      <Snackbar
        open={snackbar.open}
        autoHideDuration={4000}
        onClose={() => setSnackbar({ ...snackbar, open: false })}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
      >
        <Alert
          onClose={() => setSnackbar({ ...snackbar, open: false })}
          severity={snackbar.severity}
          sx={{ width: '100%' }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </Box>
  );
}

export default Jobs;