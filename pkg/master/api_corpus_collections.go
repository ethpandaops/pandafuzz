package master

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// CorpusCollection represents a collection of corpus files
type CorpusCollection struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	FileCount   int       `json:"file_count"`
	TotalSize   int64     `json:"total_size"`
	Tags        []string  `json:"tags"`
}

// CorpusCollectionFile represents a file in a corpus collection
type CorpusCollectionFile struct {
	ID           string    `json:"id"`
	CollectionID string    `json:"collection_id"`
	Filename     string    `json:"filename"`
	Hash         string    `json:"hash"`
	Size         int64     `json:"size"`
	UploadedAt   time.Time `json:"uploaded_at"`
}

// getStorage returns the SQLiteStorage if available, or nil
func (s *Server) getStorage() *storage.SQLiteStorage {
	if sqliteDB, ok := s.state.db.(*storage.SQLiteStorage); ok {
		return sqliteDB
	}
	return nil
}

// handleListCorpusCollections returns all corpus collections
func (s *Server) handleListCorpusCollections(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	s.logger.Info("handleListCorpusCollections called")

	// Get storage
	db := s.getStorage()
	if db == nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Storage not available", nil)
		return
	}

	// Get all corpus collections from storage
	collections, err := db.GetCorpusCollections(ctx)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get corpus collections from database")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve corpus collections list", err)
		return
	}

	// Convert to API response format
	apiCollections := make([]CorpusCollection, 0, len(collections))
	for _, c := range collections {
		apiCollections = append(apiCollections, CorpusCollection{
			ID:          c.ID,
			Name:        c.Name,
			Description: c.Description,
			CreatedAt:   c.CreatedAt,
			UpdatedAt:   c.UpdatedAt,
			FileCount:   c.FileCount,
			TotalSize:   c.TotalSize,
			Tags:        c.Tags,
		})
	}

	response := map[string]any{
		"collections": apiCollections,
		"count":       len(apiCollections),
	}

	s.writeJSONResponse(w, response)
}

// handleCreateCorpusCollection creates a new corpus collection
func (s *Server) handleCreateCorpusCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}

	if err := s.decodeJSONBody(w, r, &req); err != nil {
		return
	}

	// Validate request
	if req.Name == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Name is required", nil)
		return
	}

	// Create collection
	collection := &common.CorpusCollection{
		ID:          "collection-" + uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		FileCount:   0,
		TotalSize:   0,
		Tags:        req.Tags,
	}

	// Get storage
	db := s.getStorage()
	if db == nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Storage not available", nil)
		return
	}

	if err := db.CreateCorpusCollection(ctx, collection); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to create corpus collection", err)
		return
	}

	s.logger.WithField("collection_id", collection.ID).Info("Created corpus collection")

	w.WriteHeader(http.StatusCreated)
	s.writeJSONResponse(w, CorpusCollection{
		ID:          collection.ID,
		Name:        collection.Name,
		Description: collection.Description,
		CreatedAt:   collection.CreatedAt,
		UpdatedAt:   collection.UpdatedAt,
		FileCount:   collection.FileCount,
		TotalSize:   collection.TotalSize,
		Tags:        collection.Tags,
	})
}

// handleGetCorpusCollection returns a specific corpus collection
func (s *Server) handleGetCorpusCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	collectionID := vars["id"]

	// Get storage
	db := s.getStorage()
	if db == nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Storage not available", nil)
		return
	}

	collection, err := db.GetCorpusCollection(ctx, collectionID)
	if err != nil {
		if common.IsNotFoundError(err) {
			s.writeErrorResponse(w, http.StatusNotFound, "Corpus collection not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get corpus collection", err)
		}
		return
	}

	s.writeJSONResponse(w, CorpusCollection{
		ID:          collection.ID,
		Name:        collection.Name,
		Description: collection.Description,
		CreatedAt:   collection.CreatedAt,
		UpdatedAt:   collection.UpdatedAt,
		FileCount:   collection.FileCount,
		TotalSize:   collection.TotalSize,
		Tags:        collection.Tags,
	})
}

// handleUploadCorpusToCollection handles file upload to a collection
func (s *Server) handleUploadCorpusToCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	collectionID := vars["id"]

	// Verify collection exists
	// Get storage
	db := s.getStorage()
	if db == nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Storage not available", nil)
		return
	}

	collection, err := db.GetCorpusCollection(ctx, collectionID)
	if err != nil {
		if common.IsNotFoundError(err) {
			s.writeErrorResponse(w, http.StatusNotFound, "Corpus collection not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get corpus collection", err)
		}
		return
	}

	// Parse multipart form
	err = r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Failed to parse multipart form", err)
		return
	}

	uploadedFiles := []CorpusCollectionFile{}
	totalSize := int64(0)

	// Process each uploaded file
	for _, headers := range r.MultipartForm.File {
		for _, header := range headers {
			// Open uploaded file
			file, err := header.Open()
			if err != nil {
				s.logger.WithError(err).Error("Failed to open uploaded file")
				continue
			}
			defer file.Close()

			// Read file content
			data, err := io.ReadAll(file)
			if err != nil {
				s.logger.WithError(err).Error("Failed to read uploaded file")
				continue
			}

			// Calculate hash
			hasher := sha256.New()
			hasher.Write(data)
			hash := hex.EncodeToString(hasher.Sum(nil))

			// Create file record
			fileRecord := &common.CorpusCollectionFile{
				ID:           "file-" + uuid.New().String(),
				CollectionID: collectionID,
				Filename:     header.Filename,
				Hash:         hash,
				Size:         int64(len(data)),
				UploadedAt:   time.Now(),
			}

			// Store file metadata
			if err := db.AddCorpusCollectionFile(ctx, fileRecord); err != nil {
				s.logger.WithError(err).Error("Failed to store corpus file metadata")
				continue
			}

			// Store file content using file storage
			// For now, we'll store files directly on disk
			// TODO: Use proper file storage service when available
			storageBasePath := "./storage"
			filePath := fmt.Sprintf("%s/collections/%s/%s", storageBasePath, collectionID, hash)

			// Create directory if needed
			fileDir := fmt.Sprintf("%s/collections/%s", storageBasePath, collectionID)
			if err := os.MkdirAll(fileDir, 0755); err != nil {
				s.logger.WithError(err).Error("Failed to create collection directory")
				continue
			}

			// Write file
			if err := os.WriteFile(filePath, data, 0644); err != nil {
				s.logger.WithError(err).Error("Failed to store corpus file content")
				// Remove metadata if content storage failed
				db.DeleteCorpusCollectionFile(ctx, fileRecord.ID)
				continue
			}

			uploadedFiles = append(uploadedFiles, CorpusCollectionFile{
				ID:           fileRecord.ID,
				CollectionID: fileRecord.CollectionID,
				Filename:     fileRecord.Filename,
				Hash:         fileRecord.Hash,
				Size:         fileRecord.Size,
				UploadedAt:   fileRecord.UploadedAt,
			})
			totalSize += fileRecord.Size
		}
	}

	// Update collection stats
	collection.FileCount += len(uploadedFiles)
	collection.TotalSize += totalSize
	collection.UpdatedAt = time.Now()
	db.UpdateCorpusCollection(ctx, collection)

	response := map[string]any{
		"collection_id":  collectionID,
		"uploaded_files": uploadedFiles,
		"count":          len(uploadedFiles),
		"total_size":     totalSize,
	}

	w.WriteHeader(http.StatusCreated)
	s.writeJSONResponse(w, response)
}

// handleGetCollectionFiles returns all files in a collection
func (s *Server) handleGetCollectionFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	collectionID := vars["id"]

	// Get storage
	db := s.getStorage()
	if db == nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Storage not available", nil)
		return
	}

	// Verify collection exists
	_, err := db.GetCorpusCollection(ctx, collectionID)
	if err != nil {
		if common.IsNotFoundError(err) {
			s.writeErrorResponse(w, http.StatusNotFound, "Corpus collection not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get corpus collection", err)
		}
		return
	}

	// Get files in collection
	files, err := db.GetCorpusCollectionFiles(ctx, collectionID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get collection files", err)
		return
	}

	// Convert to API response format
	apiFiles := make([]CorpusCollectionFile, 0, len(files))
	for _, f := range files {
		apiFiles = append(apiFiles, CorpusCollectionFile{
			ID:           f.ID,
			CollectionID: f.CollectionID,
			Filename:     f.Filename,
			Hash:         f.Hash,
			Size:         f.Size,
			UploadedAt:   f.UploadedAt,
		})
	}

	response := map[string]any{
		"collection_id": collectionID,
		"files":         apiFiles,
		"count":         len(apiFiles),
	}

	s.writeJSONResponse(w, response)
}

// handleDeleteCorpusCollection deletes a corpus collection
func (s *Server) handleDeleteCorpusCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	collectionID := vars["id"]

	// Get collection to verify it exists
	// Get storage
	db := s.getStorage()
	if db == nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Storage not available", nil)
		return
	}

	collection, err := db.GetCorpusCollection(ctx, collectionID)
	if err != nil {
		if common.IsNotFoundError(err) {
			s.writeErrorResponse(w, http.StatusNotFound, "Corpus collection not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get corpus collection", err)
		}
		return
	}

	// Delete all files in collection from disk
	files, err := db.GetCorpusCollectionFiles(ctx, collectionID)
	if err == nil {
		storageBasePath := "./storage"
		for _, f := range files {
			filePath := fmt.Sprintf("%s/collections/%s/%s", storageBasePath, collectionID, f.Hash)
			if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
				s.logger.WithError(err).Warn("Failed to delete corpus file content")
			}
		}
		// Try to remove the collection directory too
		collectionDir := fmt.Sprintf("%s/collections/%s", storageBasePath, collectionID)
		os.RemoveAll(collectionDir)
	}

	// Delete collection and all associated files from database
	if err := db.DeleteCorpusCollection(ctx, collectionID); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete corpus collection", err)
		return
	}

	s.logger.WithField("collection_id", collection.ID).Info("Deleted corpus collection")
	w.WriteHeader(http.StatusNoContent)
}

// handleDownloadCollectionFile downloads a specific file from a corpus collection
func (s *Server) handleDownloadCollectionFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	collectionID := vars["id"]
	fileID := vars["fileId"]

	// Validate IDs
	if collectionID == "" || fileID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Collection ID and File ID are required", nil)
		return
	}

	// Get storage
	db := s.getStorage()
	if db == nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Storage not available", nil)
		return
	}

	// Get file metadata
	files, err := db.GetCorpusCollectionFiles(ctx, collectionID)
	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"collection_id": collectionID,
			"file_id":       fileID,
		}).Error("Failed to get collection files")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get collection files", err)
		return
	}

	// Find the specific file
	var targetFile *common.CorpusCollectionFile
	for _, file := range files {
		if file.ID == fileID {
			targetFile = file
			break
		}
	}

	if targetFile == nil {
		s.writeErrorResponse(w, http.StatusNotFound, "File not found in collection", nil)
		return
	}

	// Get the file content
	filePath := s.services.GetCollectionFilePath(collectionID, targetFile.Hash)
	file, err := os.Open(filePath)
	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"collection_id": collectionID,
			"file_id":       fileID,
			"file_path":     filePath,
		}).Error("Failed to open corpus file")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to open corpus file", err)
		return
	}
	defer file.Close()

	// Set response headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", targetFile.Filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", targetFile.Size))

	// Stream the file
	if _, err := io.Copy(w, file); err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"collection_id": collectionID,
			"file_id":       fileID,
		}).Error("Failed to send file")
		// Can't write error response after starting to write the file
		return
	}

	s.logger.WithFields(logrus.Fields{
		"collection_id": collectionID,
		"file_id":       fileID,
		"filename":      targetFile.Filename,
		"size":          targetFile.Size,
	}).Info("Corpus collection file downloaded")
}
