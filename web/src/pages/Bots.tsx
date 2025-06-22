import React, { useEffect, useState } from 'react';
import {
  Box,
  Card,
  CardContent,
  Chip,
  IconButton,
  LinearProgress,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Paper,
} from '@mui/material';
import {
  Delete as DeleteIcon,
  Info as InfoIcon,
  Circle as CircleIcon,
} from '@mui/icons-material';
import api from '../api/client';
import { Bot, BotStatus } from '../types';

const statusColors: Record<BotStatus, string> = {
  [BotStatus.Idle]: '#4caf50',
  [BotStatus.Busy]: '#2196f3',
  [BotStatus.Offline]: '#757575',
  [BotStatus.Failed]: '#f44336',
};

function Bots() {
  const [bots, setBots] = useState<Bot[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedBot, setSelectedBot] = useState<Bot | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [botToDelete, setBotToDelete] = useState<Bot | null>(null);

  const fetchBots = async () => {
    try {
      setLoading(true);
      const data = await api.getBots();
      setBots(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch bots');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchBots();
    const interval = setInterval(fetchBots, 3000); // Refresh every 3 seconds
    return () => clearInterval(interval);
  }, []);

  const handleDelete = async () => {
    if (!botToDelete) return;

    try {
      await api.deleteBot(botToDelete.id);
      setDeleteDialogOpen(false);
      setBotToDelete(null);
      fetchBots();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete bot');
    }
  };

  const formatLastSeen = (lastSeen: string) => {
    const date = new Date(lastSeen);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    
    if (diffMins < 1) return 'Just now';
    if (diffMins < 60) return `${diffMins}m ago`;
    if (diffMins < 1440) return `${Math.floor(diffMins / 60)}h ago`;
    return `${Math.floor(diffMins / 1440)}d ago`;
  };

  if (loading && bots.length === 0) {
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
      <Typography variant="h4" gutterBottom>
        Bot Agents
      </Typography>

      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Status</TableCell>
              <TableCell>Name</TableCell>
              <TableCell>Hostname</TableCell>
              <TableCell>IP Address</TableCell>
              <TableCell>Capabilities</TableCell>
              <TableCell>Current Job</TableCell>
              <TableCell>Last Seen</TableCell>
              <TableCell>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {bots.map((bot) => (
              <TableRow key={bot.id}>
                <TableCell>
                  <Box display="flex" alignItems="center" gap={1}>
                    <CircleIcon
                      sx={{
                        fontSize: 12,
                        color: statusColors[bot.status],
                      }}
                    />
                    <Typography variant="body2">{bot.status}</Typography>
                  </Box>
                </TableCell>
                <TableCell>{bot.name || bot.hostname}</TableCell>
                <TableCell>{bot.hostname}</TableCell>
                <TableCell>{bot.ip}</TableCell>
                <TableCell>
                  <Box display="flex" gap={0.5} flexWrap="wrap">
                    {bot.capabilities.map((cap) => (
                      <Chip key={cap} label={cap} size="small" />
                    ))}
                  </Box>
                </TableCell>
                <TableCell>
                  {bot.current_job ? (
                    <Chip label={bot.current_job} size="small" color="primary" />
                  ) : (
                    '-'
                  )}
                </TableCell>
                <TableCell>{formatLastSeen(bot.last_seen)}</TableCell>
                <TableCell>
                  <IconButton
                    size="small"
                    onClick={() => setSelectedBot(bot)}
                    title="View details"
                  >
                    <InfoIcon />
                  </IconButton>
                  <IconButton
                    size="small"
                    onClick={() => {
                      setBotToDelete(bot);
                      setDeleteDialogOpen(true);
                    }}
                    disabled={bot.status === BotStatus.Busy}
                    title="Delete bot"
                  >
                    <DeleteIcon />
                  </IconButton>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>

      {/* Bot Details Dialog */}
      <Dialog
        open={selectedBot !== null}
        onClose={() => setSelectedBot(null)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Bot Details</DialogTitle>
        <DialogContent>
          {selectedBot && (
            <Box>
              <Typography variant="subtitle1" gutterBottom>
                <strong>ID:</strong> {selectedBot.id}
              </Typography>
              <Typography variant="subtitle1" gutterBottom>
                <strong>Name:</strong> {selectedBot.name || selectedBot.hostname}
              </Typography>
              <Typography variant="subtitle1" gutterBottom>
                <strong>Hostname:</strong> {selectedBot.hostname}
              </Typography>
              <Typography variant="subtitle1" gutterBottom>
                <strong>IP Address:</strong> {selectedBot.ip}
              </Typography>
              <Typography variant="subtitle1" gutterBottom>
                <strong>Status:</strong> {selectedBot.status}
              </Typography>
              <Typography variant="subtitle1" gutterBottom>
                <strong>Registered:</strong>{' '}
                {new Date(selectedBot.registered_at).toLocaleString()}
              </Typography>
              <Typography variant="subtitle1" gutterBottom>
                <strong>Last Seen:</strong>{' '}
                {new Date(selectedBot.last_seen).toLocaleString()}
              </Typography>
              <Typography variant="subtitle1" gutterBottom>
                <strong>Failure Count:</strong> {selectedBot.failure_count}
              </Typography>
              <Typography variant="subtitle1" gutterBottom>
                <strong>Capabilities:</strong>
              </Typography>
              <Box display="flex" gap={0.5} flexWrap="wrap" ml={2}>
                {selectedBot.capabilities.map((cap) => (
                  <Chip key={cap} label={cap} />
                ))}
              </Box>
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setSelectedBot(null)}>Close</Button>
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
            Are you sure you want to delete bot "{botToDelete?.hostname}"?
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteDialogOpen(false)}>Cancel</Button>
          <Button onClick={handleDelete} color="error">
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

export default Bots;