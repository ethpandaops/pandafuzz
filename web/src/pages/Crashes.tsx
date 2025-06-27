import React, { useEffect, useState } from 'react';
import {
  Box,
  Card,
  CardContent,
  Chip,
  Dialog,
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
  TextField,
  MenuItem,
  Grid,
  Button,
  DialogActions,
} from '@mui/material';
import {
  Download as DownloadIcon,
  Info as InfoIcon,
  ContentCopy as CopyIcon,
} from '@mui/icons-material';
import api from '../api/client';
import { CrashResult } from '../types';

function Crashes() {
  const [crashes, setCrashes] = useState<CrashResult[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedCrash, setSelectedCrash] = useState<CrashResult | null>(null);
  const [filter, setFilter] = useState({
    job_id: '',
    type: '',
  });

  const fetchCrashes = async () => {
    try {
      setLoading(true);
      const params = filter.job_id ? { job_id: filter.job_id } : undefined;
      const data = await api.getCrashes({ ...params, limit: 200 });
      const filtered = filter.type
        ? data.filter((c) => c.type === filter.type)
        : data;
      setCrashes(filtered);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch crashes');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchCrashes();
  }, [filter]);

  const downloadCrash = async (crash: CrashResult) => {
    try {
      // Use the new API endpoint to download crash input
      const response = await fetch(`/api/v1/results/crashes/${crash.id}/input`);
      if (!response.ok) {
        throw new Error('Failed to download crash input');
      }
      
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `crash_${crash.hash.substring(0, 8)}.bin`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (error) {
      console.error('Failed to download crash:', error);
      // Fallback to inline data if available
      if (crash.input) {
        const binaryString = atob(crash.input);
        const bytes = new Uint8Array(binaryString.length);
        for (let i = 0; i < binaryString.length; i++) {
          bytes[i] = binaryString.charCodeAt(i);
        }
        const blob = new Blob([bytes], { type: 'application/octet-stream' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `crash_${crash.hash.substring(0, 8)}.bin`;
        a.click();
        URL.revokeObjectURL(url);
      }
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  const uniqueCrashTypes = Array.from(new Set(crashes.map((c) => c.type)));

  if (loading && crashes.length === 0) {
    return <LinearProgress />;
  }

  if (error) {
    return (
      <Box p={2}>
        <Typography color="error">Error: {error}</Typography>
      </Box>
    );
  }

  // Group crashes by hash to show unique crashes
  const uniqueCrashes = crashes.reduce((acc, crash) => {
    if (!acc[crash.hash]) {
      acc[crash.hash] = { crash, count: 0 };
    }
    acc[crash.hash].count++;
    return acc;
  }, {} as Record<string, { crash: CrashResult; count: number }>);

  return (
    <Box>
      <Typography variant="h4" gutterBottom>
        Crash Results
      </Typography>

      <Card sx={{ mb: 2 }}>
        <CardContent>
          <Grid container spacing={2} alignItems="center">
            <Grid item xs={12} sm={4}>
              <TextField
                fullWidth
                label="Filter by Job ID"
                value={filter.job_id}
                onChange={(e) => setFilter({ ...filter, job_id: e.target.value })}
                size="small"
              />
            </Grid>
            <Grid item xs={12} sm={4}>
              <TextField
                fullWidth
                select
                label="Filter by Type"
                value={filter.type}
                onChange={(e) => setFilter({ ...filter, type: e.target.value })}
                size="small"
              >
                <MenuItem value="">All</MenuItem>
                {uniqueCrashTypes.map((type) => (
                  <MenuItem key={type} value={type}>
                    {type}
                  </MenuItem>
                ))}
              </TextField>
            </Grid>
            <Grid item xs={12} sm={4}>
              <Box display="flex" gap={1}>
                <Chip
                  label={`${crashes.length} total crashes`}
                  color="primary"
                />
                <Chip
                  label={`${Object.keys(uniqueCrashes).length} unique`}
                  color="secondary"
                />
              </Box>
            </Grid>
          </Grid>
        </CardContent>
      </Card>

      {crashes.length === 0 ? (
        <Card>
          <CardContent>
            <Box display="flex" flexDirection="column" alignItems="center" py={4}>
              <Typography variant="h6" color="textSecondary" gutterBottom>
                No crashes found yet
              </Typography>
              <Typography variant="body2" color="textSecondary">
                Crashes will appear here when fuzzing jobs discover them
              </Typography>
            </Box>
          </CardContent>
        </Card>
      ) : (
      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Hash</TableCell>
              <TableCell>Type</TableCell>
              <TableCell>Job ID</TableCell>
              <TableCell>Bot ID</TableCell>
              <TableCell>Size</TableCell>
              <TableCell>Count</TableCell>
              <TableCell>First Seen</TableCell>
              <TableCell>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {Object.entries(uniqueCrashes).map(([hash, { crash, count }]) => (
              <TableRow key={crash.id}>
                <TableCell>
                  <Typography
                    variant="body2"
                    sx={{ fontFamily: 'monospace' }}
                    title={hash}
                  >
                    {hash.substring(0, 12)}...
                  </Typography>
                </TableCell>
                <TableCell>
                  <Chip label={crash.type} size="small" color="error" />
                </TableCell>
                <TableCell>
                  <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                    {crash.job_id.substring(0, 8)}...
                  </Typography>
                </TableCell>
                <TableCell>
                  <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                    {crash.bot_id.substring(0, 8)}...
                  </Typography>
                </TableCell>
                <TableCell>{formatSize(crash.size)}</TableCell>
                <TableCell>
                  {count > 1 && (
                    <Chip label={`${count}x`} size="small" color="warning" />
                  )}
                </TableCell>
                <TableCell>
                  {new Date(crash.timestamp).toLocaleString()}
                </TableCell>
                <TableCell>
                  <IconButton
                    size="small"
                    onClick={() => setSelectedCrash(crash)}
                    title="View details"
                  >
                    <InfoIcon />
                  </IconButton>
                  <IconButton
                    size="small"
                    onClick={() => downloadCrash(crash)}
                    title="Download input"
                  >
                    <DownloadIcon />
                  </IconButton>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      )}

      {/* Crash Details Dialog */}
      <Dialog
        open={selectedCrash !== null}
        onClose={() => setSelectedCrash(null)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>
          Crash Details
          <IconButton
            sx={{ position: 'absolute', right: 8, top: 8 }}
            onClick={() => selectedCrash && downloadCrash(selectedCrash)}
          >
            <DownloadIcon />
          </IconButton>
        </DialogTitle>
        <DialogContent>
          {selectedCrash && (
            <Box>
              <Grid container spacing={2}>
                <Grid item xs={12}>
                  <Typography variant="subtitle2" color="textSecondary">
                    Hash
                  </Typography>
                  <Box display="flex" alignItems="center" gap={1}>
                    <Typography
                      variant="body1"
                      sx={{ fontFamily: 'monospace' }}
                      gutterBottom
                    >
                      {selectedCrash.hash}
                    </Typography>
                    <IconButton
                      size="small"
                      onClick={() => copyToClipboard(selectedCrash.hash)}
                    >
                      <CopyIcon fontSize="small" />
                    </IconButton>
                  </Box>
                </Grid>
                <Grid item xs={6}>
                  <Typography variant="subtitle2" color="textSecondary">
                    Type
                  </Typography>
                  <Typography variant="body1" gutterBottom>
                    <Chip label={selectedCrash.type} color="error" size="small" />
                  </Typography>
                </Grid>
                <Grid item xs={6}>
                  <Typography variant="subtitle2" color="textSecondary">
                    Size
                  </Typography>
                  <Typography variant="body1" gutterBottom>
                    {formatSize(selectedCrash.size)}
                  </Typography>
                </Grid>
                <Grid item xs={6}>
                  <Typography variant="subtitle2" color="textSecondary">
                    Job ID
                  </Typography>
                  <Typography
                    variant="body1"
                    sx={{ fontFamily: 'monospace' }}
                    gutterBottom
                  >
                    {selectedCrash.job_id}
                  </Typography>
                </Grid>
                <Grid item xs={6}>
                  <Typography variant="subtitle2" color="textSecondary">
                    Bot ID
                  </Typography>
                  <Typography
                    variant="body1"
                    sx={{ fontFamily: 'monospace' }}
                    gutterBottom
                  >
                    {selectedCrash.bot_id}
                  </Typography>
                </Grid>
                <Grid item xs={12}>
                  <Typography variant="subtitle2" color="textSecondary">
                    Timestamp
                  </Typography>
                  <Typography variant="body1" gutterBottom>
                    {new Date(selectedCrash.timestamp).toLocaleString()}
                  </Typography>
                </Grid>
                {selectedCrash.output && (
                  <Grid item xs={12}>
                    <Typography variant="subtitle2" color="textSecondary">
                      Output
                    </Typography>
                    <Paper sx={{ p: 2, bgcolor: 'background.default' }}>
                      <Typography
                        variant="body2"
                        component="pre"
                        sx={{
                          fontFamily: 'monospace',
                          fontSize: '0.85rem',
                          overflow: 'auto',
                        }}
                      >
                        {selectedCrash.output}
                      </Typography>
                    </Paper>
                  </Grid>
                )}
                {selectedCrash.stack_trace && (
                  <Grid item xs={12}>
                    <Typography variant="subtitle2" color="textSecondary">
                      Stack Trace
                    </Typography>
                    <Paper sx={{ p: 2, bgcolor: 'background.default' }}>
                      <Typography
                        variant="body2"
                        component="pre"
                        sx={{
                          fontFamily: 'monospace',
                          fontSize: '0.85rem',
                          overflow: 'auto',
                        }}
                      >
                        {selectedCrash.stack_trace}
                      </Typography>
                    </Paper>
                  </Grid>
                )}
              </Grid>
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setSelectedCrash(null)}>Close</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

export default Crashes;