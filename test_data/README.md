# Test Data Directory

This directory contains test data used for fuzzing campaigns.

## Directory Structure

```
test_data/
├── seeds/       # Initial seed inputs for fuzzing
└── corpus/      # Generated test corpus (managed by fuzzers)
```

## Seeds Directory

The `seeds/` directory contains initial test inputs used to seed fuzzing campaigns. These files help fuzzers get started with meaningful inputs.

### Seed File Formats

- **Text seeds**: Plain text files containing sample inputs
- **Binary seeds**: Binary files for testing binary parsers
- **Corpus archives**: ZIP files containing collections of seed inputs

### Example Seeds

- `seed_input.txt`: Basic text input for testing
- `crash_seed.txt`: Known crash-inducing input
- `seed_corpus.zip`: Collection of seed inputs in ZIP format

## Corpus Directory

The `corpus/` directory is managed by the fuzzing engine and contains:
- Interesting test cases discovered during fuzzing
- Minimized inputs that trigger new code paths
- Crash reproducers

Note: Contents of the corpus directory are typically generated during fuzzing runs and should not be manually edited.