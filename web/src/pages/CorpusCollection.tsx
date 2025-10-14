import React, { useEffect, useState, useRef } from 'react';
import {
  Box,
  Button,
  Card,
  CardContent,
  CardActions,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Fab,
  IconButton,
  Paper,
  TextField,
  Typography,
  Grid,
  Alert,
  Tooltip,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  Divider,
  LinearProgress,
  Snackbar,
} from '@mui/material';
import {
  Add as AddIcon,
  Delete as DeleteIcon,
  CloudUpload as UploadIcon,
  Download as DownloadIcon,
  Folder as FolderIcon,
  InsertDriveFile as FileIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';
import { formatDateTime } from '../utils/dateFormat';

interface CorpusCollection {
  id: string;
  name: string;
  description: string;
  created_at: string;
  updated_at: string;
  file_count: number;
  total_size: number;
  tags: string[];
}

interface CorpusFile {
  id: string;
  collection_id: string;
  filename: string;
  hash: string;
  size: number;
  uploaded_at: string;
}

function CorpusCollection() {
  const [collections, setCollections] = useState<CorpusCollection[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [uploadDialogOpen, setUploadDialogOpen] = useState(false);
  const [selectedCollection, setSelectedCollection] = useState<CorpusCollection | null>(null);
  const [collectionFiles, setCollectionFiles] = useState<CorpusFile[]>([]);
  const [filesLoading, setFilesLoading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState(0);
  const [snackbar, setSnackbar] = useState<{
    open: boolean;
    message: string;
    severity: 'success' | 'error' | 'info';
  }>({ open: false, message: '', severity: 'info' });

  // Form state
  const [newCollection, setNewCollection] = useState({
    name: '',
    description: '',
    tags: '',
  });

  // File upload state
  const [uploadFiles, setUploadFiles] = useState<File[]>([]);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const fetchCollections = async () => {
    try {
      setLoading(true);
      const response = await fetch('/api/v1/corpus/collections');
      if (!response.ok) throw new Error('Failed to fetch collections');
      const data = await response.json();
      setCollections(data.collections || []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch collections');
    } finally {
      setLoading(false);
    }
  };

  const fetchCollectionFiles = async (collectionId: string) => {
    try {
      setFilesLoading(true);
      const response = await fetch(`/api/v1/corpus/collections/${collectionId}/files`);
      if (!response.ok) throw new Error('Failed to fetch files');
      const data = await response.json();
      setCollectionFiles(data.files || []);
    } catch (err) {
      console.error('Failed to fetch collection files:', err);
    } finally {
      setFilesLoading(false);
    }
  };

  useEffect(() => {
    fetchCollections();
  }, []);

  const handleCreateCollection = async () => {
    try {
      const response = await fetch('/api/v1/corpus/collections', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: newCollection.name,
          description: newCollection.description,
          tags: newCollection.tags.split(',').map(t => t.trim()).filter(t => t),
        }),
      });

      if (!response.ok) {
        const error = await response.text();
        throw new Error(error || 'Failed to create collection');
      }

      setSnackbar({
        open: true,
        message: 'Collection created successfully',
        severity: 'success',
      });
      setCreateDialogOpen(false);
      setNewCollection({ name: '', description: '', tags: '' });
      fetchCollections();
    } catch (err) {
      setSnackbar({
        open: true,
        message: err instanceof Error ? err.message : 'Failed to create collection',
        severity: 'error',
      });
    }
  };

  const handleUploadFiles = async () => {
    if (!selectedCollection || uploadFiles.length === 0) return;

    try {
      setUploadProgress(0);
      const formData = new FormData();
      
      uploadFiles.forEach((file) => {
        formData.append('files', file);
      });

      const response = await fetch(`/api/v1/corpus/collections/${selectedCollection.id}/upload`, {
        method: 'POST',
        body: formData,
      });

      if (!response.ok) {
        throw new Error('Failed to upload files');
      }

      setSnackbar({
        open: true,
        message: `Successfully uploaded ${uploadFiles.length} files`,
        severity: 'success',
      });
      setUploadDialogOpen(false);
      setUploadFiles([]);
      fetchCollections();
      if (selectedCollection) {
        fetchCollectionFiles(selectedCollection.id);
      }
    } catch (err) {
      setSnackbar({
        open: true,
        message: err instanceof Error ? err.message : 'Failed to upload files',
        severity: 'error',
      });
    } finally {
      setUploadProgress(0);
    }
  };

  const handleDeleteCollection = async (collection: CorpusCollection) => {
    if (!window.confirm(`Are you sure you want to delete "${collection.name}"? This action cannot be undone.`)) {
      return;
    }

    try {
      const response = await fetch(`/api/v1/corpus/collections/${collection.id}`, {
        method: 'DELETE',
      });

      if (!response.ok) {
        throw new Error('Failed to delete collection');
      }

      setSnackbar({
        open: true,
        message: 'Collection deleted successfully',
        severity: 'success',
      });
      fetchCollections();
      if (selectedCollection?.id === collection.id) {
        setSelectedCollection(null);
        setCollectionFiles([]);
      }
    } catch (err) {
      setSnackbar({
        open: true,
        message: err instanceof Error ? err.message : 'Failed to delete collection',
        severity: 'error',
      });
    }
  };

  const formatFileSize = (bytes: number): string => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`;
    return `${(bytes / 1024 / 1024).toFixed(2)} MB`;
  };


  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
        <Typography variant="h4">Corpus Collection</Typography>
        <Box>
          <Tooltip title="Refresh">
            <IconButton onClick={fetchCollections} sx={{ mr: 1 }}>
              <RefreshIcon />
            </IconButton>
          </Tooltip>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => setCreateDialogOpen(true)}
          >
            Create Collection
          </Button>
        </Box>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      {collections.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <FolderIcon sx={{ fontSize: 60, color: 'text.secondary', mb: 2 }} />
          <Typography variant="h6" color="text.secondary" gutterBottom>
            No corpus collections yet
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            Create a collection to store and reuse corpus files across multiple fuzzing jobs
          </Typography>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => setCreateDialogOpen(true)}
          >
            Create First Collection
          </Button>
        </Paper>
      ) : (
        <Grid container spacing={3}>
          <Grid item xs={12} md={4}>
            <Typography variant="h6" gutterBottom>
              Collections
            </Typography>
            <List>
              {collections.map((collection) => (
                <ListItem
                  key={collection.id}
                  button
                  selected={selectedCollection?.id === collection.id}
                  onClick={() => {
                    setSelectedCollection(collection);
                    fetchCollectionFiles(collection.id);
                  }}
                >
                  <ListItemText
                    primary={collection.name}
                    secondary={`${collection.file_count} files • ${formatFileSize(collection.total_size)}`}
                  />
                  <ListItemSecondaryAction>
                    <IconButton
                      edge="end"
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDeleteCollection(collection);
                      }}
                    >
                      <DeleteIcon />
                    </IconButton>
                  </ListItemSecondaryAction>
                </ListItem>
              ))}
            </List>
          </Grid>

          <Grid item xs={12} md={8}>
            {selectedCollection ? (
              <Card>
                <CardContent>
                  <Box display="flex" justifyContent="space-between" alignItems="start" mb={2}>
                    <Box>
                      <Typography variant="h5" gutterBottom>
                        {selectedCollection.name}
                      </Typography>
                      {selectedCollection.description && (
                        <Typography variant="body2" color="text.secondary" paragraph>
                          {selectedCollection.description}
                        </Typography>
                      )}
                      <Box display="flex" gap={1} mb={2}>
                        <Chip
                          label={`${selectedCollection.file_count} files`}
                          size="small"
                        />
                        <Chip
                          label={formatFileSize(selectedCollection.total_size)}
                          size="small"
                        />
                        <Chip
                          label={`Created ${formatDateTime(selectedCollection.created_at)}`}
                          size="small"
                        />
                      </Box>
                      {selectedCollection.tags && selectedCollection.tags.length > 0 && (
                        <Box display="flex" gap={0.5}>
                          {selectedCollection.tags.map((tag, index) => (
                            <Chip
                              key={index}
                              label={tag}
                              size="small"
                              variant="outlined"
                            />
                          ))}
                        </Box>
                      )}
                    </Box>
                    <Button
                      variant="contained"
                      startIcon={<UploadIcon />}
                      onClick={() => setUploadDialogOpen(true)}
                    >
                      Upload Files
                    </Button>
                  </Box>

                  <Divider sx={{ my: 2 }} />

                  <Typography variant="h6" gutterBottom>
                    Files
                  </Typography>
                  {filesLoading ? (
                    <Box display="flex" justifyContent="center" p={4}>
                      <CircularProgress />
                    </Box>
                  ) : collectionFiles.length === 0 ? (
                    <Typography variant="body2" color="text.secondary" sx={{ py: 4, textAlign: 'center' }}>
                      No files in this collection yet
                    </Typography>
                  ) : (
                    <List>
                      {collectionFiles.map((file) => (
                        <ListItem key={file.id}>
                          <FileIcon sx={{ mr: 2, color: 'text.secondary' }} />
                          <ListItemText
                            primary={file.filename}
                            secondary={`${formatFileSize(file.size)} • ${file.hash.substring(0, 8)}... • ${formatDateTime(file.uploaded_at)}`}
                          />
                        </ListItem>
                      ))}
                    </List>
                  )}
                </CardContent>
              </Card>
            ) : (
              <Paper sx={{ p: 4, textAlign: 'center' }}>
                <Typography variant="body1" color="text.secondary">
                  Select a collection to view details
                </Typography>
              </Paper>
            )}
          </Grid>
        </Grid>
      )}

      {/* Create Collection Dialog */}
      <Dialog
        open={createDialogOpen}
        onClose={() => setCreateDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Create Corpus Collection</DialogTitle>
        <DialogContent>
          <Grid container spacing={2} sx={{ mt: 1 }}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Name"
                value={newCollection.name}
                onChange={(e) => setNewCollection({ ...newCollection, name: e.target.value })}
                required
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Description"
                value={newCollection.description}
                onChange={(e) => setNewCollection({ ...newCollection, description: e.target.value })}
                multiline
                rows={3}
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Tags (comma-separated)"
                value={newCollection.tags}
                onChange={(e) => setNewCollection({ ...newCollection, tags: e.target.value })}
                helperText="e.g., libfuzzer, network, crypto"
              />
            </Grid>
          </Grid>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateDialogOpen(false)}>Cancel</Button>
          <Button
            onClick={handleCreateCollection}
            variant="contained"
            disabled={!newCollection.name}
          >
            Create
          </Button>
        </DialogActions>
      </Dialog>

      {/* Upload Files Dialog */}
      <Dialog
        open={uploadDialogOpen}
        onClose={() => setUploadDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Upload Corpus Files</DialogTitle>
        <DialogContent>
          <input
            type="file"
            hidden
            multiple
            ref={fileInputRef}
            onChange={(e) => {
              const files = Array.from(e.target.files || []);
              setUploadFiles(files);
            }}
          />
          <Button
            variant="outlined"
            fullWidth
            startIcon={<UploadIcon />}
            onClick={() => fileInputRef.current?.click()}
            sx={{ mt: 2, mb: 2 }}
          >
            {uploadFiles.length > 0
              ? `${uploadFiles.length} files selected`
              : 'Select Files'}
          </Button>
          {uploadFiles.length > 0 && (
            <List>
              {uploadFiles.map((file, index) => (
                <ListItem key={index}>
                  <FileIcon sx={{ mr: 1, color: 'text.secondary' }} />
                  <ListItemText
                    primary={file.name}
                    secondary={formatFileSize(file.size)}
                  />
                </ListItem>
              ))}
            </List>
          )}
          {uploadProgress > 0 && (
            <LinearProgress
              variant="determinate"
              value={uploadProgress}
              sx={{ mt: 2 }}
            />
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setUploadDialogOpen(false);
            setUploadFiles([]);
          }}>
            Cancel
          </Button>
          <Button
            onClick={handleUploadFiles}
            variant="contained"
            disabled={uploadFiles.length === 0}
          >
            Upload
          </Button>
        </DialogActions>
      </Dialog>

      {/* Snackbar */}
      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={() => setSnackbar({ ...snackbar, open: false })}
      >
        <Alert
          onClose={() => setSnackbar({ ...snackbar, open: false })}
          severity={snackbar.severity}
          sx={{ width: '100%' }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>

      {/* FAB for quick collection creation */}
      <Fab
        color="primary"
        aria-label="add"
        sx={{ position: 'fixed', bottom: 16, right: 16 }}
        onClick={() => setCreateDialogOpen(true)}
      >
        <AddIcon />
      </Fab>
    </Box>
  );
}

export default CorpusCollection;