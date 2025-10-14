import React, { useState, useEffect } from 'react';
import { Button, Card, CardContent, Typography, Box, Chip, CircularProgress, Alert } from '@mui/material';
import { Download, Folder, InsertDriveFile } from '@mui/icons-material';
import { formatDateTime } from '../utils/dateFormat';

interface RawCoverageFile {
  id: string;
  format: string;
  fuzzer_stats_path?: string;
  plot_data_path?: string;
  fuzz_bitmap_path?: string;
  size: number;
  created_at: string;
}

interface RawCoverageViewProps {
  jobId: string;
}

export const RawCoverageView: React.FC<RawCoverageViewProps> = ({ jobId }) => {
  const [files, setFiles] = useState<RawCoverageFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchRawCoverageFiles();
  }, [jobId]);

  const fetchRawCoverageFiles = async () => {
    try {
      setLoading(true);
      const response = await fetch(`/api/v1/jobs/${jobId}/coverage/raw`);
      if (!response.ok) {
        throw new Error('Failed to fetch raw coverage files');
      }
      const data = await response.json();
      setFiles(data.files || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load coverage files');
    } finally {
      setLoading(false);
    }
  };

  const downloadFile = async (fileType: string) => {
    try {
      const response = await fetch(`/api/v1/jobs/${jobId}/coverage/raw/${fileType}`);
      if (!response.ok) {
        throw new Error(`Failed to download ${fileType}`);
      }
      
      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${jobId}_${fileType}.${fileType === 'fuzz_bitmap' ? 'bin' : 'txt'}`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (err) {
      alert(`Failed to download ${fileType}: ${err}`);
    }
  };

  const downloadAllAsZip = async () => {
    try {
      const response = await fetch(`/api/v1/jobs/${jobId}/coverage/raw/all/zip`);
      if (!response.ok) {
        throw new Error('Failed to download zip file');
      }
      
      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${jobId}_coverage.zip`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (err) {
      alert(`Failed to download zip: ${err}`);
    }
  };

  const formatBytes = (bytes: number): string => {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
  };


  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="200px">
        <CircularProgress />
      </Box>
    );
  }

  if (error) {
    return (
      <Alert severity="error" sx={{ mt: 2 }}>
        {error}
      </Alert>
    );
  }

  const latestFile = files.length > 0 ? files[0] : null;
  const hasRawFiles = latestFile && (
    latestFile.fuzzer_stats_path || 
    latestFile.plot_data_path || 
    latestFile.fuzz_bitmap_path
  );

  if (!hasRawFiles) {
    return (
      <Alert severity="info" sx={{ mt: 2 }}>
        No raw AFL++ coverage files available for this job.
      </Alert>
    );
  }

  return (
    <Card sx={{ mt: 2 }}>
      <CardContent>
        <Box display="flex" justifyContent="space-between" alignItems="center" mb={2}>
          <Typography variant="h6">
            Raw AFL++ Coverage Files
          </Typography>
          <Button
            variant="contained"
            color="primary"
            startIcon={<Folder />}
            onClick={downloadAllAsZip}
            size="small"
          >
            Download All as ZIP
          </Button>
        </Box>

        <Box display="flex" flexDirection="column" gap={2}>
          {latestFile.fuzzer_stats_path && (
            <Box display="flex" alignItems="center" justifyContent="space-between" p={2} bgcolor="background.paper" borderRadius={1} border="1px solid" borderColor="divider">
              <Box display="flex" alignItems="center" gap={2}>
                <InsertDriveFile color="primary" />
                <Box>
                  <Typography variant="subtitle1" fontWeight="bold">
                    fuzzer_stats
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    AFL++ statistics and metrics
                  </Typography>
                </Box>
              </Box>
              <Box display="flex" alignItems="center" gap={2}>
                <Chip label="Text" size="small" variant="outlined" />
                <Button
                  variant="outlined"
                  size="small"
                  startIcon={<Download />}
                  onClick={() => downloadFile('fuzzer_stats')}
                >
                  Download
                </Button>
              </Box>
            </Box>
          )}

          {latestFile.plot_data_path && (
            <Box display="flex" alignItems="center" justifyContent="space-between" p={2} bgcolor="background.paper" borderRadius={1} border="1px solid" borderColor="divider">
              <Box display="flex" alignItems="center" gap={2}>
                <InsertDriveFile color="primary" />
                <Box>
                  <Typography variant="subtitle1" fontWeight="bold">
                    plot_data
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    Time-series coverage data for plotting
                  </Typography>
                </Box>
              </Box>
              <Box display="flex" alignItems="center" gap={2}>
                <Chip label="CSV" size="small" variant="outlined" />
                <Button
                  variant="outlined"
                  size="small"
                  startIcon={<Download />}
                  onClick={() => downloadFile('plot_data')}
                >
                  Download
                </Button>
              </Box>
            </Box>
          )}

          {latestFile.fuzz_bitmap_path && (
            <Box display="flex" alignItems="center" justifyContent="space-between" p={2} bgcolor="background.paper" borderRadius={1} border="1px solid" borderColor="divider">
              <Box display="flex" alignItems="center" gap={2}>
                <InsertDriveFile color="primary" />
                <Box>
                  <Typography variant="subtitle1" fontWeight="bold">
                    fuzz_bitmap
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    Binary coverage bitmap data
                  </Typography>
                </Box>
              </Box>
              <Box display="flex" alignItems="center" gap={2}>
                <Chip label="Binary" size="small" variant="outlined" />
                <Button
                  variant="outlined"
                  size="small"
                  startIcon={<Download />}
                  onClick={() => downloadFile('fuzz_bitmap')}
                >
                  Download
                </Button>
              </Box>
            </Box>
          )}
        </Box>

        {latestFile && (
          <Box mt={2} pt={2} borderTop="1px solid" borderColor="divider">
            <Typography variant="caption" color="text.secondary">
              Total size: {formatBytes(latestFile.size)} • 
              Collected: {formatDateTime(latestFile.created_at)}
            </Typography>
          </Box>
        )}
      </CardContent>
    </Card>
  );
};