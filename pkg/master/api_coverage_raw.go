package master

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ethpandaops/pandafuzz/pkg/storage"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// handleGetRawCoverageFiles returns list of raw coverage files for a job
func (s *Server) handleGetRawCoverageFiles(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if jobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID is required", nil)
		return
	}

	s.logger.WithField("job_id", jobID).Debug("Getting raw coverage files for job")

	// Get SQLite connection directly
	sqliteStorage, ok := s.state.db.(*storage.SQLiteStorage)
	if !ok {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Database type not supported", nil)
		return
	}

	db := sqliteStorage.GetDB()

	// Query database for raw coverage reports
	query := `
		SELECT id, format, storage_path, fuzzer_stats_path, plot_data_path, fuzz_bitmap_path, size, created_at
		FROM coverage_reports 
		WHERE job_id = ? AND file_type = 'raw'
		ORDER BY created_at DESC
	`

	s.logger.WithField("query", query).WithField("jobID", jobID).Debug("Executing raw coverage query")
	rows, err := db.Query(query, jobID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to query raw coverage files")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to query coverage files", err)
		return
	}
	defer rows.Close()

	rowCount := 0

	type RawCoverageFile struct {
		ID              string `json:"id"`
		Format          string `json:"format"`
		StoragePath     string `json:"storage_path,omitempty"`
		FuzzerStatsPath string `json:"fuzzer_stats_path,omitempty"`
		PlotDataPath    string `json:"plot_data_path,omitempty"`
		FuzzBitmapPath  string `json:"fuzz_bitmap_path,omitempty"`
		Size            int64  `json:"size"`
		CreatedAt       string `json:"created_at"`
	}

	var files []RawCoverageFile
	for rows.Next() {
		rowCount++
		var file RawCoverageFile
		var storagePath sql.NullString
		var fuzzerStatsPath, plotDataPath, fuzzBitmapPath sql.NullString

		err := rows.Scan(&file.ID, &file.Format, &storagePath,
			&fuzzerStatsPath, &plotDataPath, &fuzzBitmapPath,
			&file.Size, &file.CreatedAt)
		if err != nil {
			s.logger.WithError(err).Error("Failed to scan coverage file row")
			continue
		}

		if storagePath.Valid {
			file.StoragePath = storagePath.String
		}
		if fuzzerStatsPath.Valid {
			file.FuzzerStatsPath = fuzzerStatsPath.String
		}
		if plotDataPath.Valid {
			file.PlotDataPath = plotDataPath.String
		}
		if fuzzBitmapPath.Valid {
			file.FuzzBitmapPath = fuzzBitmapPath.String
		}

		files = append(files, file)
	}

	s.logger.WithFields(logrus.Fields{
		"job_id":      jobID,
		"rows_found":  rowCount,
		"files_added": len(files),
	}).Debug("Raw coverage query completed")

	s.writeJSONResponse(w, map[string]interface{}{
		"job_id": jobID,
		"files":  files,
		"count":  len(files),
	})
}

// handleDownloadRawCoverageFile downloads a specific raw coverage file
func (s *Server) handleDownloadRawCoverageFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]
	fileType := vars["fileType"]

	if jobID == "" || fileType == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID and file type are required", nil)
		return
	}

	// Validate file type
	validTypes := map[string]bool{
		"fuzzer_stats": true,
		"plot_data":    true,
		"fuzz_bitmap":  true,
	}

	if !validTypes[fileType] {
		s.writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid file type: %s", fileType), nil)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"job_id":    jobID,
		"file_type": fileType,
	}).Debug("Downloading raw coverage file")

	// Get SQLite connection directly
	sqliteStorage, ok := s.state.db.(*storage.SQLiteStorage)
	if !ok {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Database type not supported", nil)
		return
	}

	db := sqliteStorage.GetDB()

	// Query database for the file path
	var column string
	switch fileType {
	case "fuzzer_stats":
		column = "fuzzer_stats_path"
	case "plot_data":
		column = "plot_data_path"
	case "fuzz_bitmap":
		column = "fuzz_bitmap_path"
	}

	query := fmt.Sprintf(`
		SELECT %s FROM coverage_reports 
		WHERE job_id = ? AND file_type = 'raw' AND %s IS NOT NULL
		ORDER BY created_at DESC
		LIMIT 1
	`, column, column)

	var storagePath sql.NullString
	err := db.QueryRow(query, jobID).Scan(&storagePath)
	if err != nil || !storagePath.Valid {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"job_id":    jobID,
			"file_type": fileType,
		}).Error("Failed to find raw coverage file")
		s.writeErrorResponse(w, http.StatusNotFound, "Coverage file not found", err)
		return
	}

	// Retrieve file from storage
	reader, err := s.storageBackend.Retrieve(r.Context(), storagePath.String)
	if err != nil {
		s.logger.WithError(err).WithField("storage_path", storagePath.String).Error("Failed to retrieve file from storage")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve file", err)
		return
	}
	defer reader.Close()

	// Set headers for file download
	filename := fmt.Sprintf("%s_%s.txt", jobID, fileType)
	if fileType == "fuzz_bitmap" {
		filename = fmt.Sprintf("%s_%s.bin", jobID, fileType)
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	// Copy file to response
	written, err := io.Copy(w, reader)
	if err != nil {
		s.logger.WithError(err).Error("Failed to write file to response")
		return
	}

	s.logger.WithFields(logrus.Fields{
		"job_id":    jobID,
		"file_type": fileType,
		"size":      written,
	}).Info("Raw coverage file downloaded successfully")
}

// handleGetAllRawFiles downloads all raw coverage files as a zip
func (s *Server) handleGetAllRawFiles(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if jobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID is required", nil)
		return
	}

	s.logger.WithField("job_id", jobID).Debug("Downloading all raw coverage files as zip")

	// Get SQLite connection directly
	sqliteStorage, ok := s.state.db.(*storage.SQLiteStorage)
	if !ok {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Database type not supported", nil)
		return
	}

	db := sqliteStorage.GetDB()

	// Query database for all file paths
	query := `
		SELECT fuzzer_stats_path, plot_data_path, fuzz_bitmap_path
		FROM coverage_reports 
		WHERE job_id = ? AND file_type = 'raw'
		ORDER BY created_at DESC
		LIMIT 1
	`

	var fuzzerStatsPath, plotDataPath, fuzzBitmapPath sql.NullString
	err := db.QueryRow(query, jobID).Scan(&fuzzerStatsPath, &plotDataPath, &fuzzBitmapPath)
	if err != nil {
		s.logger.WithError(err).WithField("job_id", jobID).Error("Failed to find raw coverage files")
		s.writeErrorResponse(w, http.StatusNotFound, "Coverage files not found", err)
		return
	}

	// Create a temporary directory for files
	tempDir := filepath.Join("/tmp", fmt.Sprintf("coverage_%s", jobID))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		s.logger.WithError(err).Error("Failed to create temp directory")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to prepare files", err)
		return
	}
	defer os.RemoveAll(tempDir)

	// Download each file
	files := map[string]sql.NullString{
		"fuzzer_stats": fuzzerStatsPath,
		"plot_data":    plotDataPath,
		"fuzz_bitmap":  fuzzBitmapPath,
	}

	for fileType, pathValue := range files {
		if !pathValue.Valid || pathValue.String == "" {
			continue
		}

		reader, err := s.storageBackend.Retrieve(r.Context(), pathValue.String)
		if err != nil {
			s.logger.WithError(err).WithField("file_type", fileType).Warn("Failed to retrieve file")
			continue
		}

		destPath := filepath.Join(tempDir, fileType)
		if fileType == "fuzz_bitmap" {
			destPath += ".bin"
		} else {
			destPath += ".txt"
		}

		file, err := os.Create(destPath)
		if err != nil {
			reader.Close()
			s.logger.WithError(err).WithField("file_type", fileType).Warn("Failed to create temp file")
			continue
		}

		_, err = io.Copy(file, reader)
		file.Close()
		reader.Close()

		if err != nil {
			s.logger.WithError(err).WithField("file_type", fileType).Warn("Failed to write temp file")
			continue
		}
	}

	// Create zip file
	zipPath := filepath.Join("/tmp", fmt.Sprintf("coverage_%s.zip", jobID))
	if err := createZipFile(tempDir, zipPath); err != nil {
		s.logger.WithError(err).Error("Failed to create zip file")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to create zip file", err)
		return
	}
	defer os.Remove(zipPath)

	// Send zip file
	zipFile, err := os.Open(zipPath)
	if err != nil {
		s.logger.WithError(err).Error("Failed to open zip file")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to read zip file", err)
		return
	}
	defer zipFile.Close()

	stat, _ := zipFile.Stat()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"coverage_%s.zip\"", jobID))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))

	_, err = io.Copy(w, zipFile)
	if err != nil {
		s.logger.WithError(err).Error("Failed to send zip file")
		return
	}

	s.logger.WithField("job_id", jobID).Info("All raw coverage files downloaded as zip")
}

// createZipFile creates a zip file from a directory
func createZipFile(sourceDir, destPath string) error {
	zipFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		zipEntry, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(zipEntry, file)
		return err
	})
}
