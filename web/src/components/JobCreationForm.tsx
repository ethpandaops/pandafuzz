import React, { useState, useRef, useEffect } from 'react';
import {
  Box,
  Button,
  TextField,
  Grid,
  FormControlLabel,
  Switch,
  Alert,
  MenuItem,
  Checkbox,
  Typography,
  Divider,
  Collapse,
  FormControl,
  InputLabel,
  Select,
} from '@mui/material';
import {
  CloudUpload as UploadIcon,
} from '@mui/icons-material';
import { JobPriority, CoverageFormat } from '../types';

export interface JobFormData {
  name: string;
  fuzzer: string;
  target: string;
  target_args: string;
  priority: JobPriority;
  timeout_sec: number;
  memory_limit: number;
  collection_id: string;
  enableCoverage: boolean;
  coverageFormat: CoverageFormat;
}

interface JobCreationFormProps {
  initialData?: Partial<JobFormData>;
  corpusCollections: any[];
  onSubmit: (
    formData: JobFormData,
    useFileUpload: boolean,
    targetBinaryFile: File | null,
    seedCorpusFiles: File[]
  ) => Promise<void>;
  onCancel: () => void;
  error?: string | null;
  loading?: boolean;
}

const JobCreationForm: React.FC<JobCreationFormProps> = ({
  initialData,
  corpusCollections = [],
  onSubmit,
  onCancel,
  error,
  loading = false,
}) => {
  const [formData, setFormData] = useState<JobFormData>({
    name: '',
    fuzzer: 'aflplusplus',
    target: '',
    target_args: '',
    priority: JobPriority.Normal,
    timeout_sec: 3600,
    memory_limit: 2048,
    collection_id: '',
    enableCoverage: false,
    coverageFormat: 'json',
    ...initialData,
  });

  const [useFileUpload, setUseFileUpload] = useState(true);
  const [targetBinaryFile, setTargetBinaryFile] = useState<File | null>(null);
  const [seedCorpusFiles, setSeedCorpusFiles] = useState<File[]>([]);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const corpusInputRef = useRef<HTMLInputElement>(null);

  // Reset form when initialData changes
  useEffect(() => {
    if (initialData) {
      setFormData(prev => ({ ...prev, ...initialData }));
    }
  }, [initialData]);

  const handleSubmit = async () => {
    await onSubmit(formData, useFileUpload, targetBinaryFile, seedCorpusFiles);
  };

  const handleInputChange = (field: keyof JobFormData, value: any) => {
    setFormData(prev => ({ ...prev, [field]: value }));
  };

  const isFormValid = () => {
    return (
      formData.name &&
      formData.fuzzer &&
      (useFileUpload ? targetBinaryFile : formData.target)
    );
  };

  return (
    <Box>
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
            value={formData.name}
            onChange={(e) => handleInputChange('name', e.target.value)}
            required
          />
        </Grid>

        <Grid item xs={6}>
          <TextField
            fullWidth
            select
            label="Fuzzer"
            value={formData.fuzzer}
            onChange={(e) => handleInputChange('fuzzer', e.target.value)}
          >
            <MenuItem value="aflplusplus">AFL++</MenuItem>
            <MenuItem value="libfuzzer">LibFuzzer</MenuItem>
            <MenuItem value="honggfuzz">Honggfuzz</MenuItem>
          </TextField>
        </Grid>

        <Grid item xs={6}>
          <TextField
            fullWidth
            select
            label="Priority"
            value={formData.priority}
            onChange={(e) => handleInputChange('priority', e.target.value as JobPriority)}
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
                checked={!useFileUpload}
                onChange={(e) => {
                  setUseFileUpload(!e.target.checked);
                  if (!e.target.checked) {
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
              value={formData.target}
              onChange={(e) => handleInputChange('target', e.target.value)}
              helperText="Path to the binary on the fuzzing bot"
              required
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
                <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5, display: 'block' }}>
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
            select
            label="Corpus Collection (Optional)"
            value={formData.collection_id}
            onChange={(e) => handleInputChange('collection_id', e.target.value)}
            helperText="Use an existing corpus collection for this job"
          >
            <MenuItem value="">None</MenuItem>
            {corpusCollections.map((collection) => (
              <MenuItem key={collection.id} value={collection.id}>
                {collection.name} ({collection.file_count} files, {(collection.total_size / 1024 / 1024).toFixed(2)} MB)
              </MenuItem>
            ))}
          </TextField>
        </Grid>

        <Grid item xs={12}>
          <TextField
            fullWidth
            label="Target Arguments"
            value={formData.target_args}
            onChange={(e) => handleInputChange('target_args', e.target.value)}
            helperText="Space-separated arguments"
          />
        </Grid>

        <Grid item xs={6}>
          <TextField
            fullWidth
            label="Timeout (seconds)"
            type="number"
            value={formData.timeout_sec}
            onChange={(e) => handleInputChange('timeout_sec', parseInt(e.target.value) || 3600)}
          />
        </Grid>

        <Grid item xs={6}>
          <TextField
            fullWidth
            label="Memory Limit (MB)"
            type="number"
            value={formData.memory_limit}
            onChange={(e) => handleInputChange('memory_limit', parseInt(e.target.value) || 2048)}
          />
        </Grid>

        {/* Coverage Configuration Section */}
        <Grid item xs={12}>
          <Divider sx={{ my: 2 }}>
            <Typography variant="body2" color="text.secondary">
              Code Coverage Options
            </Typography>
          </Divider>
        </Grid>

        <Grid item xs={12}>
          <FormControlLabel
            control={
              <Checkbox
                checked={formData.enableCoverage}
                onChange={(e) => handleInputChange('enableCoverage', e.target.checked)}
                color="primary"
              />
            }
            label={
              <Box>
                <Typography variant="body1">
                  Enable code coverage collection
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  Collect and analyze code coverage information during fuzzing
                </Typography>
              </Box>
            }
          />
        </Grid>

        <Collapse in={formData.enableCoverage}>
          <Grid container spacing={2} sx={{ mt: 0, pl: 2 }}>
            <Grid item xs={12}>
              <FormControl fullWidth>
                <InputLabel>Coverage Format</InputLabel>
                <Select
                  value={formData.coverageFormat}
                  label="Coverage Format"
                  onChange={(e) => handleInputChange('coverageFormat', e.target.value as CoverageFormat)}
                >
                  <MenuItem value="json">JSON</MenuItem>
                  <MenuItem value="html">HTML</MenuItem>
                  <MenuItem value="lcov">LCOV</MenuItem>
                  <MenuItem value="cobertura">Cobertura XML</MenuItem>
                </Select>
              </FormControl>
              <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5, display: 'block' }}>
                Choose the format for coverage reports. JSON is recommended for programmatic analysis, HTML for visual reports.
              </Typography>
            </Grid>
          </Grid>
        </Collapse>
      </Grid>

      <Box sx={{ mt: 3, display: 'flex', gap: 2, justifyContent: 'flex-end' }}>
        <Button onClick={onCancel} disabled={loading}>
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          disabled={!isFormValid() || loading}
        >
          {loading ? 'Creating...' : 'Create Job'}
        </Button>
      </Box>
    </Box>
  );
};

export default JobCreationForm;