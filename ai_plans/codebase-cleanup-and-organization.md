# Codebase Cleanup and Organization Implementation Plan

## Executive Summary
> The pandafuzz codebase has grown organically through multiple development iterations, resulting in:
> - Stale implementation plans that are fully completed
> - Duplicate test files and scripts scattered at the root level
> - Inconsistent naming patterns across packages
> - Compiled binaries tracked in version control
> - Documentation sprawl with overlapping content
> - Poor directory organization making navigation difficult
> 
> This plan provides a systematic approach to clean up and reorganize the codebase, making it more maintainable and easier to navigate while preserving all active functionality.

## Goals & Objectives
### Primary Goals
- Clean up root directory by moving files to appropriate subdirectories
- Remove or archive completed AI plan files and stale documentation
- Establish consistent naming patterns across all packages
- Improve gitignore coverage to exclude build artifacts

### Secondary Objectives
- Consolidate duplicate test programs and scripts
- Organize documentation into a clear hierarchy
- Create standard directory structure for test data
- Improve test coverage tracking

## Solution Overview
### Approach
Systematically reorganize the codebase by:
1. Creating new directory structure
2. Moving files to appropriate locations
3. Updating import paths and references
4. Removing duplicates and stale files
5. Standardizing naming conventions

### Key Components
1. **Directory Reorganization**: Create logical subdirectories for scripts, test targets, configs, and documentation
2. **File Consolidation**: Merge duplicate functionality into single, parameterized versions
3. **Archive Management**: Move completed AI plans to archive directory
4. **Build Artifact Cleanup**: Update gitignore and remove tracked binaries

### Expected Outcomes
- Root directory contains only essential files (README, go.mod, docker-compose.yml)
- All scripts consolidated in scripts/ directory
- Test targets organized in test_targets/ directory
- Documentation follows clear hierarchy in docs/ directory
- No compiled binaries in version control
- Consistent file naming patterns throughout codebase

## Implementation Tasks

### CRITICAL IMPLEMENTATION RULES
1. **NO PLACEHOLDER CODE**: Every implementation must be production-ready. NEVER write "TODO", "in a real implementation", or similar placeholders unless explicitly requested by the user.
2. **CROSS-DIRECTORY TASKS**: Group related changes across directories into single tasks to ensure consistency. Never create isolated changes that require follow-up work in sibling directories.
3. **COMPLETE IMPLEMENTATIONS**: Each task must fully implement its feature including all consumers, type updates, and integration points.
4. **DETAILED SPECIFICATIONS**: Each task must include EXACTLY what to implement, including specific functions, types, and integration points to avoid "breaking change" confusion.
5. **CONTEXT AWARENESS**: Each task is part of a larger system - specify how it connects to other parts.
6. **MAKE BREAKING CHANGES**: Unless explicitly requested by the user, you MUST make breaking changes.

### Visual Dependency Tree
[Shows the target directory structure after cleanup]

```
pandafuzz/
├── .gitignore (Task #0: Update to exclude all build artifacts)
├── README.md (existing)
├── go.mod (Task #1: Run go mod tidy)
├── docker-compose.yml (existing)
│
├── ai_plans/
│   └── archived/ (Task #3: Move completed plans here)
│       ├── fix-sqlite-deadlock.md
│       ├── fuzzer-integration-and-resource-cleanup.md
│       └── fuzzing-features-implementation.md
│
├── cmd/ (existing structure)
│   ├── bot/
│   └── master/
│
├── pkg/ (existing structure)
│
├── scripts/ (Task #4: Consolidate all scripts)
│   ├── create_job.sh (unified from multiple scripts)
│   ├── test_local_setup.sh
│   ├── build.sh
│   └── stop_all.sh
│
├── test_targets/ (Task #5: Organize test binaries)
│   ├── crashers/
│   │   ├── test_crasher.c
│   │   └── segfault.c
│   ├── fuzzers/
│   │   ├── libfuzzer_test.cc
│   │   └── test_fuzzer.c
│   └── vulnerable/
│       └── vulnerable_app.c
│
├── test_data/ (Task #6: Organize test data)
│   ├── seeds/
│   │   ├── seed_input.txt
│   │   └── seed_corpus.zip
│   └── corpus/
│
├── configs/ (Task #7: Example configurations)
│   ├── bot.example.yaml
│   └── bot.docker.example.yaml
│
├── docs/ (Task #8: Reorganize documentation)
│   ├── architecture.md (from PANDAFUZZ_DESIGN.md + ARCHITECTURE_ANALYSIS.md)
│   ├── development.md (from TODO.md + PROJECT_STATUS.md)
│   ├── api.md (existing)
│   └── archive/
│       ├── IMPLEMENTATION_PLAN.md
│       └── CRITIQUE.md
│
└── web/ (existing structure)
```

### Execution Plan

#### Group A: Foundation (Execute all in parallel)
- [x] **Task #0**: Update .gitignore file
  - File: `.gitignore`
  - Add sections:
    ```gitignore
    # Compiled binaries
    test_crasher
    test_crash
    test_fuzzer
    libfuzzer_test
    *.test
    
    # Crash artifacts
    crash-*
    
    # Go build artifacts
    /cmd/bot/bot
    /cmd/master/master
    
    # Database files
    /data/*.db
    /data/*.db-journal
    
    # Temporary files
    *.tmp
    *.log
    ```
  - Context: Prevents build artifacts from being tracked in git

- [x] **Task #1**: Clean up Go dependencies
  - Command: `go mod tidy`
  - Verify: All imports are properly declared in go.mod
  - Context: Ensures all dependencies are properly tracked

- [x] **Task #2**: Create new directory structure
  - Create directories:
    - `mkdir -p ai_plans/archived`
    - `mkdir -p scripts`
    - `mkdir -p test_targets/{crashers,fuzzers,vulnerable}`
    - `mkdir -p test_data/{seeds,corpus}`
    - `mkdir -p configs`
    - `mkdir -p docs/archive`
  - Context: Establishes organized structure for file movement

#### Group B: File Movement (Execute after Group A completes)
- [x] **Task #3**: Archive completed AI plans
  - Move files:
    - `mv ai_plans/fix-sqlite-deadlock.md ai_plans/archived/`
    - `mv ai_plans/fuzzer-integration-and-resource-cleanup.md ai_plans/archived/`
    - `mv ai_plans/fuzzing-features-implementation.md ai_plans/archived/`
  - Update ai_plans/archived/README.md:
    - List archived plans with completion dates
    - Brief summary of what each plan accomplished
  - Context: Preserves historical plans while cleaning active directory

- [x] **Task #4**: Consolidate and move scripts
  - Create unified job creation script:
    - File: `scripts/create_job.sh`
    - Merge functionality from: create_test_job.sh, create_job_with_binary.sh, create_job_final.sh, create_libfuzzer_job.sh
    - Parameters: --type (test|binary|libfuzzer), --binary-path, --seeds, --timeout
  - Move other scripts:
    - `mv test_local_setup.sh scripts/`
    - `mv build.sh scripts/`
    - `mv stop_all.sh scripts/`
  - Delete original scripts after consolidation
  - Update any references in documentation
  - Context: Single parameterized script is easier to maintain than multiple variants

- [x] **Task #5**: Organize test targets
  - Move crash test programs:
    - `mv test_crasher.c test_targets/crashers/`
    - `mv test_crash.c test_targets/crashers/segfault.c` (rename for clarity)
  - Move fuzzer test programs:
    - `mv libfuzzer_test.cc test_targets/fuzzers/`
    - `mv test_fuzzer.c test_targets/fuzzers/`
  - Move vulnerable programs:
    - `mv samples/vulnerable_app.c test_targets/vulnerable/`
    - Remove examples/vulnerable_program.c (duplicate)
  - Update build scripts to reference new locations
  - Remove compiled binaries: test_crasher, test_crash, test_fuzzer, libfuzzer_test
  - Context: Centralizes all test programs in one location

- [x] **Task #6**: Organize test data
  - Move seed files:
    - `mv seed_input.txt test_data/seeds/`
    - `mv seed_corpus.zip test_data/seeds/`
    - `mv test_seeds/* test_data/seeds/`
    - `rmdir test_seeds`
  - Create README in test_data/:
    - Explain directory structure
    - Document seed file formats
  - Context: Standardizes test data location

- [x] **Task #7**: Clean up configuration files
  - Create example configs:
    - `cp bot.yaml configs/bot.example.yaml`
    - `cp bot-docker.yaml configs/bot.docker.example.yaml`
  - Add comments to example files explaining each option
  - Update .gitignore to exclude actual config files:
    ```gitignore
    bot.yaml
    bot-docker.yaml
    ```
  - Context: Separates example configs from actual deployment configs

#### Group C: Documentation Consolidation (Execute after Group B)
- [x] **Task #8**: Reorganize documentation
  - Consolidate architecture docs:
    - Merge PANDAFUZZ_DESIGN.md and ARCHITECTURE_ANALYSIS.md into docs/architecture.md
    - Remove originals after merge
  - Consolidate development docs:
    - Merge TODO.md and PROJECT_STATUS.md into docs/development.md
    - Remove originals after merge
  - Archive old docs:
    - `mv IMPLEMENTATION_PLAN.md docs/archive/`
    - `mv CRITIQUE.md docs/archive/`
  - Update main README.md:
    - Add links to new documentation structure
    - Update any outdated references
  - Context: Creates clear documentation hierarchy

#### Group D: Final Cleanup (Execute after Group C)
- [x] **Task #9**: Remove stale files and update references
  - Delete duplicate files:
    - `rm examples/vulnerable_program.c` (moved to test_targets)
    - `rm libfuzzer_test.c` (duplicate of .cc version)
  - Remove crash artifacts:
    - `rm crash-*`
  - Update any hardcoded paths in:
    - Go source files referencing moved test programs
    - Docker configurations
    - CI/CD scripts
  - Run tests to ensure nothing broke
  - Context: Final verification that cleanup is complete

- [x] **Task #10**: Update development documentation
  - Create docs/project-structure.md:
    - Document new directory organization
    - Explain where different file types belong
    - Include guidelines for future additions
  - Update README.md:
    - Add "Project Structure" section
    - Update any build/test instructions for new paths
  - Context: Ensures team understands new organization

---

## Implementation Workflow

This plan file serves as the authoritative checklist for implementation. When implementing:

### Required Process
1. **Load Plan**: Read this entire plan file before starting
2. **Sync Tasks**: Create TodoWrite tasks matching the checkboxes below
3. **Execute & Update**: For each task:
   - Mark TodoWrite as `in_progress` when starting
   - Update checkbox `[ ]` to `[x]` when completing
   - Mark TodoWrite as `completed` when done
4. **Maintain Sync**: Keep this file and TodoWrite synchronized throughout

### Critical Rules
- This plan file is the source of truth for progress
- Update checkboxes in real-time as work progresses
- Never lose synchronization between plan file and TodoWrite
- Mark tasks complete only when fully implemented (no placeholders)
- Tasks should be run in parallel, unless there are dependencies, using subtasks, to avoid context bloat.

### Progress Tracking
The checkboxes above represent the authoritative status of each task. Keep them updated as you work.