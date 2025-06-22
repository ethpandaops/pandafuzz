import React, { useEffect, useState, useRef, useCallback, useMemo } from 'react';
import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Fab,
  IconButton,
  LinearProgress,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
  MenuItem,
  Grid,
  FormControlLabel,
  Switch,
  Alert,
  Tooltip,
  Skeleton,
  Grow,
  Collapse,
  Snackbar,
  Zoom,
  alpha,
} from '@mui/material';
import {
  Add as AddIcon,
  Cancel as CancelIcon,
  Delete as DeleteIcon,
  Info as InfoIcon,
  Description as LogsIcon,
  CloudUpload as UploadIcon,
  CheckCircle as SuccessIcon,
  Error as ErrorIcon,
  Schedule as PendingIcon,
  PlayArrow as RunningIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';
import api from '../api/client';
import { Job, JobStatus, JobPriority, Bot } from '../types';

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
  const [logsDialogOpen, setLogsDialogOpen] = useState(false);
  const [logsJob, setLogsJob] = useState<Job | null>(null);
  const [jobLogs, setJobLogs] = useState<any>(null);
  const [logsLoading, setLogsLoading] = useState(false);
  
  // Snackbar state
  const [snackbar, setSnackbar] = useState<{
    open: boolean;
    message: string;
    severity: 'success' | 'error' | 'info' | 'warning';
  }>({ open: false, message: '', severity: 'info' });
  
  // New job form state
  const [newJob, setNewJob] = useState({
    name: '',
    fuzzer: 'afl++',
    target: '',
    target_args: '',
    priority: JobPriority.Normal,
    timeout_sec: 3600,
    memory_limit: 2048,
  });
  
  // File upload state
  const [useFileUpload, setUseFileUpload] = useState(true);
  const [targetBinaryFile, setTargetBinaryFile] = useState<File | null>(null);
  const [seedCorpusFiles, setSeedCorpusFiles] = useState<File[]>([]);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const corpusInputRef = useRef<HTMLInputElement>(null);

  const fetchJobs = useCallback(async (showLoading = true) => {
    try {
      if (showLoading) {
        setLoading(true);
      } else {
        setIsRefreshing(true);
      }
      const data = await api.getJobs({ limit: 100 });
      
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
  }, []);

  const fetchBots = async () => {
    try {
      const data = await api.getBots();
      setBots(data);
    } catch (err) {
      console.error('Failed to fetch bots:', err);
    }
  };

  useEffect(() => {
    // Initial load with loading indicator
    fetchJobs(true);
    fetchBots();
    
    // Subsequent updates without loading indicator
    const interval = setInterval(() => {
      fetchJobs(false);
    }, 10000); // Update every 10 seconds instead of 5
    
    return () => clearInterval(interval);
  }, []);

  const getBotName = useCallback((botId: string | undefined): string => {
    if (!botId) return '-';
    const bot = bots.find(b => b.id === botId);
    return bot ? bot.name : botId;
  }, [bots]);

  const handleCreateJob = async () => {
    try {
      const jobData = {
        ...newJob,
        target_args: newJob.target_args.split(' ').filter(arg => arg),
      };
      
      if (useFileUpload && targetBinaryFile) {
        // Use upload endpoint
        await api.createJobWithUpload(jobData, targetBinaryFile, seedCorpusFiles);
      } else {
        // Use regular endpoint
        await api.createJob(jobData);
      }
      
      setCreateDialogOpen(false);
      setNewJob({
        name: '',
        fuzzer: 'afl++',
        target: '',
        target_args: '',
        priority: JobPriority.Normal,
        timeout_sec: 3600,
        memory_limit: 2048,
      });
      setTargetBinaryFile(null);
      setSeedCorpusFiles([]);
      setUseFileUpload(true);
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
    const startDate = new Date(start);
    const endDate = end ? new Date(end) : new Date();
    const diffMs = endDate.getTime() - startDate.getTime();
    const hours = Math.floor(diffMs / 3600000);
    const minutes = Math.floor((diffMs % 3600000) / 60000);
    return `${hours}h ${minutes}m`;
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
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          onClick={() => {
            setCreateDialogOpen(true);
            setNewJob({
              name: '',
              fuzzer: 'afl++',
              target: '',
              target_args: '',
              priority: JobPriority.Normal,
              timeout_sec: 3600,
              memory_limit: 2048,
            });
            setTargetBinaryFile(null);
            setSeedCorpusFiles([]);
            setUseFileUpload(true);
            setError(null);
          }}
        >
          New Job
        </Button>
      </Box>

      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Status</TableCell>
              <TableCell>Priority</TableCell>
              <TableCell>Fuzzer</TableCell>
              <TableCell>Target</TableCell>
              <TableCell>Bot</TableCell>
              <TableCell>Duration</TableCell>
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
                  {formatDuration(job.started_at, job.completed_at)}
                </TableCell>
                <TableCell>
                  <IconButton
                    size="small"
                    onClick={() => setSelectedJob(job)}
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
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Create New Job</DialogTitle>
        <DialogContent>
          {error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {error}
            </Alert>
          )}
          {useFileUpload ? (
            <Alert severity="info" sx={{ mb: 2 }}>
              Upload your target binary and optionally include seed corpus files. The binary will be stored and made available to fuzzing bots.
            </Alert>
          ) : (
            <Alert severity="warning" sx={{ mb: 2 }}>
              Advanced mode: Specify a path to an existing binary on the fuzzing bots. Make sure the binary exists at the specified location on all bots.
            </Alert>
          )}
          <Grid container spacing={2} sx={{ mt: 1 }}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Job Name"
                value={newJob.name}
                onChange={(e) => setNewJob({ ...newJob, name: e.target.value })}
              />
            </Grid>
            <Grid item xs={6}>
              <TextField
                fullWidth
                select
                label="Fuzzer"
                value={newJob.fuzzer}
                onChange={(e) => setNewJob({ ...newJob, fuzzer: e.target.value })}
              >
                <MenuItem value="afl++">AFL++</MenuItem>
                <MenuItem value="libfuzzer">LibFuzzer</MenuItem>
              </TextField>
            </Grid>
            <Grid item xs={6}>
              <TextField
                fullWidth
                select
                label="Priority"
                value={newJob.priority}
                onChange={(e) => setNewJob({ ...newJob, priority: e.target.value as JobPriority })}
              >
                <MenuItem value={JobPriority.Low}>Low</MenuItem>
                <MenuItem value={JobPriority.Normal}>Normal</MenuItem>
                <MenuItem value={JobPriority.High}>High</MenuItem>
              </TextField>
            </Grid>
            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={useFileUpload}
                    onChange={(e) => {
                      setUseFileUpload(e.target.checked);
                      if (e.target.checked) {
                        setTargetBinaryFile(null);
                        setSeedCorpusFiles([]);
                      }
                    }}
                  />
                }
                label="Use existing binary path (advanced)"
              />
            </Grid>
            {!useFileUpload ? (
              <Grid item xs={12}>
                <TextField
                  fullWidth
                  label="Target Binary Path"
                  value={newJob.target}
                  onChange={(e) => setNewJob({ ...newJob, target: e.target.value })}
                  helperText="Path to the binary on the fuzzing bot"
                />
              </Grid>
            ) : (
              <>
                <Grid item xs={12}>
                  <input
                    type="file"
                    hidden
                    ref={fileInputRef}
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) {
                        setTargetBinaryFile(file);
                      }
                    }}
                    accept="*"
                  />
                  <Button
                    variant="outlined"
                    fullWidth
                    startIcon={<UploadIcon />}
                    onClick={() => fileInputRef.current?.click()}
                    sx={{ justifyContent: 'flex-start', textAlign: 'left' }}
                    color={targetBinaryFile ? 'primary' : 'inherit'}
                  >
                    {targetBinaryFile ? targetBinaryFile.name : 'Select Target Binary (Required)'}
                  </Button>
                  {targetBinaryFile && (
                    <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
                      Size: {(targetBinaryFile.size / 1024 / 1024).toFixed(2)} MB
                    </Typography>
                  )}
                </Grid>
                <Grid item xs={12}>
                  <input
                    type="file"
                    hidden
                    multiple
                    ref={corpusInputRef}
                    onChange={(e) => {
                      const files = Array.from(e.target.files || []);
                      setSeedCorpusFiles(files);
                    }}
                  />
                  <Button
                    variant="outlined"
                    fullWidth
                    startIcon={<UploadIcon />}
                    onClick={() => corpusInputRef.current?.click()}
                    sx={{ justifyContent: 'flex-start', textAlign: 'left' }}
                  >
                    {seedCorpusFiles.length > 0
                      ? `${seedCorpusFiles.length} seed corpus files selected`
                      : 'Select Seed Corpus (Optional)'}
                  </Button>
                </Grid>
              </>
            )}
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Target Arguments"
                value={newJob.target_args}
                onChange={(e) => setNewJob({ ...newJob, target_args: e.target.value })}
                helperText="Space-separated arguments"
              />
            </Grid>
            <Grid item xs={6}>
              <TextField
                fullWidth
                label="Timeout (seconds)"
                type="number"
                value={newJob.timeout_sec}
                onChange={(e) => setNewJob({ ...newJob, timeout_sec: parseInt(e.target.value) })}
              />
            </Grid>
            <Grid item xs={6}>
              <TextField
                fullWidth
                label="Memory Limit (MB)"
                type="number"
                value={newJob.memory_limit}
                onChange={(e) => setNewJob({ ...newJob, memory_limit: parseInt(e.target.value) })}
              />
            </Grid>
          </Grid>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setCreateDialogOpen(false);
            setUseFileUpload(true);
            setTargetBinaryFile(null);
            setSeedCorpusFiles([]);
          }}>Cancel</Button>
          <Button 
            onClick={handleCreateJob} 
            variant="contained"
            disabled={!newJob.name || !newJob.fuzzer || (useFileUpload ? !targetBinaryFile : !newJob.target)}
          >
            Create
          </Button>
        </DialogActions>
      </Dialog>

      {/* Job Details Dialog */}
      <Dialog
        open={selectedJob !== null}
        onClose={() => setSelectedJob(null)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>Job Details</DialogTitle>
        <DialogContent>
          {selectedJob && (
            <Box>
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
                    {new Date(selectedJob.created_at).toLocaleString()}
                  </Typography>
                </Grid>
                <Grid item xs={6}>
                  <Typography variant="subtitle2" color="textSecondary">
                    Timeout
                  </Typography>
                  <Typography variant="body1" gutterBottom>
                    {new Date(selectedJob.timeout_at).toLocaleString()}
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
            </Box>
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
            {jobLogs && jobLogs.has_more && (
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
          ) : jobLogs && jobLogs.logs.length > 0 ? (
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
                    [{new Date(log.timestamp).toLocaleTimeString()}]
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
          {jobLogs && jobLogs.logs.length > 0 && (
            <Button
              variant="contained"
              onClick={() => {
                const logText = jobLogs.logs
                  .map(
                    (log: any) =>
                      `[${new Date(log.timestamp).toLocaleString()}] [${log.level}] [${log.source}] ${log.message}`
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