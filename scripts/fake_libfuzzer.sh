#!/bin/bash
# Fake libfuzzer that simulates crash detection

# Get the corpus directory (first argument)
CORPUS_DIR="$1"

echo "INFO: Running fake libfuzzer..."
echo "#10 INITED cov: 20 ft: 30 corp: 5/10b exec/s: 100 rss: 50Mb"

# Check if we have any crash seed
if [ -f "$CORPUS_DIR/crash_seed.txt" ]; then
    CONTENT=$(cat "$CORPUS_DIR/crash_seed.txt")
    if [[ "$CONTENT" == *"CRASH"* ]]; then
        # Create a crash file
        CRASH_FILE="./crash-$(date +%s)"
        cp "$CORPUS_DIR/crash_seed.txt" "$CRASH_FILE"
        
        echo "==12345==ERROR: AddressSanitizer: SEGV on unknown address 0x000000000000"
        echo "Test unit written to $CRASH_FILE"
        echo "SUMMARY: AddressSanitizer: SEGV"
        exit 1
    fi
fi

# Normal execution
for i in {1..10}; do
    echo "#$((100 * i)) DONE cov: $((20 + i)) ft: $((30 + i)) corp: $((5 + i))/100b exec/s: 100 rss: 50Mb"
    sleep 1
done

echo "Done 1000 runs in 10 second(s)"