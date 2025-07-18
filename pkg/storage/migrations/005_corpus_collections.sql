-- Create corpus collections table
CREATE TABLE IF NOT EXISTS corpus_collections (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    file_count INTEGER DEFAULT 0,
    total_size BIGINT DEFAULT 0,
    tags TEXT, -- JSON array of tags
    UNIQUE(name)
);

-- Create corpus collection files table
CREATE TABLE IF NOT EXISTS corpus_collection_files (
    id VARCHAR(255) PRIMARY KEY,
    collection_id VARCHAR(255) NOT NULL,
    filename VARCHAR(255) NOT NULL,
    hash VARCHAR(64) NOT NULL,
    size BIGINT NOT NULL,
    uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (collection_id) REFERENCES corpus_collections(id) ON DELETE CASCADE,
    UNIQUE(collection_id, hash)
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_corpus_collections_name ON corpus_collections(name);
CREATE INDEX IF NOT EXISTS idx_corpus_collection_files_collection_id ON corpus_collection_files(collection_id);
CREATE INDEX IF NOT EXISTS idx_corpus_collection_files_hash ON corpus_collection_files(hash);