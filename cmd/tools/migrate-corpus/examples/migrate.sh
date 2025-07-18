#!/bin/bash
# Example migration script for PandaFuzz corpus

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}PandaFuzz Corpus Migration Script${NC}"
echo "=================================="

# Check if migrate-corpus binary exists
if [ ! -f "./migrate-corpus" ]; then
    echo -e "${YELLOW}Building migrate-corpus tool...${NC}"
    go build -o migrate-corpus ../main.go
fi

# Function to run migration with nice output
run_migration() {
    local source=$1
    local dest=$2
    local prefix=$3
    local extra_args=$4
    
    echo -e "\n${GREEN}Starting migration:${NC}"
    echo "  Source: $source"
    echo "  Destination: $dest"
    if [ -n "$prefix" ]; then
        echo "  Prefix: $prefix"
    fi
    echo ""
    
    # First, do a dry run
    echo -e "${YELLOW}Performing dry run...${NC}"
    ./migrate-corpus -source "$source" -dest "$dest" -prefix "$prefix" -dry-run $extra_args
    
    # Ask for confirmation
    echo -e "\n${YELLOW}Do you want to proceed with the actual migration? (y/N)${NC}"
    read -r response
    
    if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
        echo -e "\n${GREEN}Starting actual migration...${NC}"
        ./migrate-corpus -source "$source" -dest "$dest" -prefix "$prefix" $extra_args
        echo -e "\n${GREEN}Migration completed!${NC}"
    else
        echo -e "${RED}Migration cancelled.${NC}"
    fi
}

# Example 1: Migrate from filesystem to MinIO
echo -e "\n${YELLOW}Example 1: Filesystem to MinIO${NC}"
echo "This will migrate all corpus files from local filesystem to MinIO"
# run_migration "filesystem.yaml" "minio.yaml" "" "-parallel 8"

# Example 2: Migrate specific campaign
echo -e "\n${YELLOW}Example 2: Migrate specific campaign${NC}"
echo "This will migrate only files for campaign 'test-campaign'"
# run_migration "filesystem.yaml" "s3-aws.yaml" "corpus/test-campaign" "-parallel 4"

# Example 3: Full migration with cleanup
echo -e "\n${YELLOW}Example 3: Full migration with source cleanup${NC}"
echo "WARNING: This will delete source files after successful migration!"
# run_migration "minio.yaml" "s3-aws.yaml" "" "-parallel 16 -delete-source"

# Interactive mode
echo -e "\n${YELLOW}Interactive Migration${NC}"
echo "========================"

# Get source config
echo -e "\nAvailable source configs:"
ls -1 *.yaml | grep -v examples | cat -n
echo -n "Select source config number: "
read source_num
source_file=$(ls -1 *.yaml | grep -v examples | sed -n "${source_num}p")

# Get destination config
echo -e "\nAvailable destination configs:"
ls -1 *.yaml | grep -v examples | grep -v "$source_file" | cat -n
echo -n "Select destination config number: "
read dest_num
dest_file=$(ls -1 *.yaml | grep -v examples | grep -v "$source_file" | sed -n "${dest_num}p")

# Get prefix
echo -n -e "\nEnter prefix to migrate (leave empty for all): "
read prefix

# Get parallel workers
echo -n "Number of parallel workers (default: 8): "
read parallel
parallel=${parallel:-8}

# Delete source?
echo -n "Delete source files after migration? (y/N): "
read delete_source
if [[ "$delete_source" =~ ^([yY][eE][sS]|[yY])$ ]]; then
    delete_arg="-delete-source"
else
    delete_arg=""
fi

# Run the migration
run_migration "$source_file" "$dest_file" "$prefix" "-parallel $parallel $delete_arg"