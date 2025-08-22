-- Add columns to store raw AFL++ coverage file paths
ALTER TABLE coverage_reports ADD COLUMN fuzzer_stats_path TEXT;
ALTER TABLE coverage_reports ADD COLUMN plot_data_path TEXT;
ALTER TABLE coverage_reports ADD COLUMN fuzz_bitmap_path TEXT;
ALTER TABLE coverage_reports ADD COLUMN file_type TEXT; -- 'synthetic' or 'raw'

-- Create index for file type
CREATE INDEX IF NOT EXISTS idx_coverage_reports_file_type ON coverage_reports(file_type);