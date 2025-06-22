import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Button,
  IconButton,
  Chip,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Alert,
  CircularProgress,
  Grid,
  Card,
  CardContent,
  LinearProgress,
} from '@mui/material';
import {
  CloudUpload as UploadIcon,
  Download as DownloadIcon,
  Delete as DeleteIcon,
  Refresh as RefreshIcon,
  Folder as FolderIcon,
  InsertDriveFile as FileIcon,
} from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import api from '../api/client';
import { Job } from '../types';

interface CorpusFile {
  name: string;
  size: number;
  hash: string;
  uploaded_at: string;
}

interface CorpusStats {
  total_files: number;
  total_size: number;
  last_updated: string;
  unique_hashes: number;
}

export function Corpus() {
  const navigate = useNavigate();
  const [jobs, setJobs] = useState<Job[]>([]);
  const [selectedJob, setSelectedJob] = useState<string>('');
  const [corpusFiles, setCorpusFiles] = useState<CorpusFile[]>([]);
  const [corpusStats, setCorpusStats] = useState<CorpusStats | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [uploadDialogOpen, setUploadDialogOpen] = useState(false);
  const [selectedFiles, setSelectedFiles] = useState<FileList | null>(null);
  const [uploading, setUploading] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [fileToDelete, setFileToDelete] = useState<CorpusFile | null>(null);

  // Fetch jobs on mount
  useEffect(() => {
    fetchJobs();
  }, []);

  // Fetch corpus when job is selected
  useEffect(() => {
    if (selectedJob) {
      fetchCorpusData();
    }
  }, [selectedJob]);

  const fetchJobs = async () => {
    try {
      const data = await api.getJobs();
      setJobs(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch jobs');
    }
  };

  const fetchCorpusData = async () => {
    if (!selectedJob) return;

    setLoading(true);
    try {
      // Fetch corpus files
      const filesResponse = await api.getJobCorpus(selectedJob);
      setCorpusFiles(filesResponse.files || []);

      // Fetch corpus stats
      const statsResponse = await api.getJobCorpusStats(selectedJob);
      setCorpusStats(statsResponse);

      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch corpus data');
      setCorpusFiles([]);
      setCorpusStats(null);
    } finally {
      setLoading(false);
    }
  };

  const handleUpload = async () => {
    if (!selectedFiles || !selectedJob) return;

    setUploading(true);
    try {
      const formData = new FormData();
      Array.from(selectedFiles).forEach((file) => {
        formData.append('files', file);
      });

      await api.uploadJobCorpus(selectedJob, formData);
      setUploadDialogOpen(false);
      setSelectedFiles(null);
      fetchCorpusData(); // Refresh corpus data
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to upload corpus files');
    } finally {
      setUploading(false);
    }
  };

  const handleDownload = async (filename: string) => {
    if (!selectedJob) return;

    try {
      const blob = await api.downloadCorpusFile(selectedJob, filename);
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = filename;
      a.click();
      window.URL.revokeObjectURL(url);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to download corpus file');
    }
  };

  const handleDelete = async () => {
    if (!fileToDelete || !selectedJob) return;

    try {
      await api.deleteCorpusFile(selectedJob, fileToDelete.name);
      setDeleteDialogOpen(false);
      setFileToDelete(null);
      fetchCorpusData(); // Refresh corpus data
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete corpus file');
    }
  };

  const formatFileSize = (bytes: number): string => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  const formatDate = (dateString: string): string => {
    const date = new Date(dateString);
    return date.toLocaleDateString() + ' ' + date.toLocaleTimeString();
  };

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
        <Typography variant="h4" component="h1">
          Corpus Management
        </Typography>
        <Box display="flex" gap={2}>
          <FormControl sx={{ minWidth: 200 }}>
            <InputLabel>Select Job</InputLabel>
            <Select
              value={selectedJob}
              onChange={(e) => setSelectedJob(e.target.value)}
              label="Select Job"
              size="small"
            >
              <MenuItem value="">
                <em>None</em>
              </MenuItem>
              {jobs.map((job) => (
                <MenuItem key={job.id} value={job.id}>
                  {job.name} ({job.id.substring(0, 8)})
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          {selectedJob && (
            <>
              <Button
                variant="contained"
                startIcon={<UploadIcon />}
                onClick={() => setUploadDialogOpen(true)}
              >
                Upload Corpus
              </Button>
              <IconButton onClick={fetchCorpusData} title="Refresh">
                <RefreshIcon />
              </IconButton>
            </>
          )}
        </Box>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      {!selectedJob ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <FolderIcon sx={{ fontSize: 64, color: 'text.secondary', mb: 2 }} />
          <Typography variant="h6" color="text.secondary">
            Select a job to view its corpus
          </Typography>
        </Paper>
      ) : loading ? (
        <Box display="flex" justifyContent="center" p={4}>
          <CircularProgress />
        </Box>
      ) : (
        <>
          {/* Corpus Statistics */}
          {corpusStats && (
            <Grid container spacing={3} sx={{ mb: 3 }}>
              <Grid item xs={12} sm={6} md={3}>
                <Card>
                  <CardContent>
                    <Typography color="textSecondary" gutterBottom>
                      Total Files
                    </Typography>
                    <Typography variant="h4">
                      {corpusStats.total_files}
                    </Typography>
                  </CardContent>
                </Card>
              </Grid>
              <Grid item xs={12} sm={6} md={3}>
                <Card>
                  <CardContent>
                    <Typography color="textSecondary" gutterBottom>
                      Total Size
                    </Typography>
                    <Typography variant="h4">
                      {formatFileSize(corpusStats.total_size)}
                    </Typography>
                  </CardContent>
                </Card>
              </Grid>
              <Grid item xs={12} sm={6} md={3}>
                <Card>
                  <CardContent>
                    <Typography color="textSecondary" gutterBottom>
                      Unique Files
                    </Typography>
                    <Typography variant="h4">
                      {corpusStats.unique_hashes}
                    </Typography>
                  </CardContent>
                </Card>
              </Grid>
              <Grid item xs={12} sm={6} md={3}>
                <Card>
                  <CardContent>
                    <Typography color="textSecondary" gutterBottom>
                      Last Updated
                    </Typography>
                    <Typography variant="body1">
                      {corpusStats.last_updated ? formatDate(corpusStats.last_updated) : 'Never'}
                    </Typography>
                  </CardContent>
                </Card>
              </Grid>
            </Grid>
          )}

          {/* Corpus Files Table */}
          <TableContainer component={Paper}>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell>File Name</TableCell>
                  <TableCell>Size</TableCell>
                  <TableCell>Uploaded</TableCell>
                  <TableCell>Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {corpusFiles.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={4} align="center">
                      <Box py={4}>
                        <FileIcon sx={{ fontSize: 48, color: 'text.secondary', mb: 1 }} />
                        <Typography color="text.secondary">
                          No corpus files found
                        </Typography>
                      </Box>
                    </TableCell>
                  </TableRow>
                ) : (
                  corpusFiles.map((file) => (
                    <TableRow key={file.name}>
                      <TableCell>{file.name}</TableCell>
                      <TableCell>{formatFileSize(file.size)}</TableCell>
                      <TableCell>{formatDate(file.uploaded_at)}</TableCell>
                      <TableCell>
                        <IconButton
                          size="small"
                          onClick={() => handleDownload(file.name)}
                          title="Download"
                        >
                          <DownloadIcon />
                        </IconButton>
                        <IconButton
                          size="small"
                          onClick={() => {
                            setFileToDelete(file);
                            setDeleteDialogOpen(true);
                          }}
                          title="Delete"
                        >
                          <DeleteIcon />
                        </IconButton>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </TableContainer>
        </>
      )}

      {/* Upload Dialog */}
      <Dialog
        open={uploadDialogOpen}
        onClose={() => setUploadDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Upload Corpus Files</DialogTitle>
        <DialogContent>
          <Box py={2}>
            <Typography variant="body2" color="textSecondary" gutterBottom>
              Corpus files are used as seed inputs for fuzzing. Upload valid test cases, 
              examples, or interesting inputs that exercise different code paths.
            </Typography>
            <Button
              variant="outlined"
              component="label"
              fullWidth
              sx={{ mt: 2 }}
              startIcon={<UploadIcon />}
            >
              Select Files
              <input
                type="file"
                multiple
                hidden
                onChange={(e) => setSelectedFiles(e.target.files)}
              />
            </Button>
            {selectedFiles && (
              <Box mt={2}>
                <Typography variant="body2" color="textSecondary">
                  Selected {selectedFiles.length} file(s):
                </Typography>
                <Box mt={1}>
                  {Array.from(selectedFiles).map((file, index) => (
                    <Chip
                      key={index}
                      label={`${file.name} (${formatFileSize(file.size)})`}
                      size="small"
                      sx={{ mr: 1, mb: 1 }}
                    />
                  ))}
                </Box>
              </Box>
            )}
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setUploadDialogOpen(false)}>Cancel</Button>
          <Button
            onClick={handleUpload}
            variant="contained"
            disabled={!selectedFiles || uploading}
            startIcon={uploading ? <CircularProgress size={20} /> : <UploadIcon />}
          >
            {uploading ? 'Uploading...' : 'Upload'}
          </Button>
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
            Are you sure you want to delete the corpus file "{fileToDelete?.name}"?
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteDialogOpen(false)}>Cancel</Button>
          <Button onClick={handleDelete} color="error" variant="contained">
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}