# PandaFuzz Technical Debt TODO List

This document tracks technical debt items identified in the codebase, prioritized by severity and impact on short-term stability and security.

## 🔴 CRITICAL - Security & Stability (Fix within 1 week)

### 1. Fix Path Traversal Vulnerability in Corpus Download
**File**: `pkg/master/api_corpus.go:257-258`
**Problem**: Insufficient path sanitization allows directory traversal attacks
**Solution**:
```go
// Replace current sanitization with:
cleanPath := filepath.Clean(filepath.Join("/", filename))
if !strings.HasPrefix(cleanPath, "/") {
    return "", errors.New("invalid filename")
}
filename = filepath.Base(cleanPath)
```
**Assignee**: Security Team
**Deadline**: Immediate

### 2. Implement Authentication & Authorization
**Files**: All API endpoints
**Problem**: No authentication mechanism despite frontend auth token support
**Solution**:
- Implement JWT-based authentication middleware
- Add role-based access control (admin, user, bot)
- Secure bot registration with pre-shared keys
- Add API key support for programmatic access
**Assignee**: Backend Team
**Deadline**: Week 1

### 3. Fix SQL Injection Pattern
**File**: `pkg/storage/migrations.go:130`
**Problem**: Dynamic table names in SQL queries using fmt.Sprintf
**Solution**:
```go
// Use a whitelist approach:
var allowedTables = map[string]bool{
    "bots": true, "jobs": true, "crashes": true,
}
if !allowedTables[table] {
    return fmt.Errorf("invalid table name: %s", table)
}
```
**Assignee**: Database Team
**Deadline**: Week 1

## 🟠 HIGH PRIORITY - Incomplete Features (Fix within 2 weeks)

### 4. Complete Fuzzer Integration
**Files**: `pkg/bot/executor_real.go`, `pkg/fuzzer/*.go`
**Problems**:
- Stub implementations for AFL++ and LibFuzzer
- Missing corpus hash calculation
- No real fuzzer process management
**Solution**:
- Implement actual AFL++ command execution
- Add LibFuzzer integration
- Implement proper process lifecycle management
- Add corpus file hashing (SHA256)
**Assignee**: Fuzzing Team
**Deadline**: Week 2

### 5. Fix Resource Leaks
**Files**: `pkg/fuzzer/aflplusplus.go`, `pkg/fuzzer/libfuzzer.go`
**Problem**: Goroutines and processes may leak on errors
**Solution**:
```go
// Add defer cleanup pattern:
defer func() {
    if cmd.Process != nil {
        cmd.Process.Kill()
    }
    cancel()
    wg.Wait()
}()
```
**Assignee**: Backend Team
**Deadline**: Week 2

### 6. Complete Database Migration
**File**: `pkg/storage/migrations.go:173-178`
**Problem**: Missing data migration from JSON columns to normalized tables
**Solution**:
- Write migration script for existing data
- Add rollback capability
- Test with production-like data volumes
**Assignee**: Database Team
**Deadline**: Week 2

## 🟡 MEDIUM PRIORITY - Reliability (Fix within 1 month)

### 7. Add Input Validation
**Files**: All API handlers
**Problem**: Missing validation on user inputs
**Solution**:
- Add validation middleware using a library like `go-playground/validator`
- Validate file uploads (size, type, content)
- Sanitize all string inputs
- Add request size limits
**Assignee**: Backend Team
**Deadline**: Week 3

### 8. Implement Rate Limiting
**Files**: `pkg/master/server.go`
**Problem**: No rate limiting despite configuration support
**Solution**:
- Implement token bucket rate limiter
- Add per-IP and per-user limits
- Configure different limits for different endpoints
**Assignee**: Backend Team
**Deadline**: Week 3

### 9. Fix Error Handling
**Files**: Throughout codebase
**Problems**:
- Errors logged but not propagated
- Missing error context
- Silently caught errors in frontend
**Solution**:
- Implement consistent error wrapping with context
- Add error monitoring/alerting
- Remove error suppression in frontend API client
**Assignee**: Full Stack Team
**Deadline**: Week 4

### 10. Add Proper Logging
**Files**: All services
**Problem**: Inconsistent logging, missing operation context
**Solution**:
- Implement structured logging with request IDs
- Add operation timing logs
- Configure log levels properly
- Ensure no sensitive data in logs
**Assignee**: DevOps Team
**Deadline**: Week 4

## 📋 TECHNICAL IMPROVEMENTS (Ongoing)

### 11. Add Missing Tests
**Priority Areas**:
- Security tests (auth, validation, path traversal)
- Concurrency tests (race conditions, deadlocks)
- Integration tests (full workflow testing)
- Error recovery tests
**Solution**:
- Aim for 80% code coverage
- Add fuzzing tests for security-critical code
- Implement load testing suite

### 12. Configuration Management
**Problems**:
- No validation on startup
- Insecure defaults
- Hardcoded values
**Solution**:
- Add configuration validation
- Implement secure defaults
- Use environment variables consistently
- Add configuration documentation

### 13. Improve Crash Analysis
**File**: `pkg/analysis/crash.go`
**Problem**: Basic deduplication might miss variants
**Solution**:
- Implement stack trace similarity analysis
- Add crash bucketing by root cause
- Integrate with symbolization tools

### 14. Add Monitoring & Metrics
**Problem**: No observability into system health
**Solution**:
- Add Prometheus metrics
- Implement health check endpoints
- Add distributed tracing
- Create operational dashboards

## 📅 Implementation Schedule

### Week 1 (Critical Security)
- [ ] Path traversal fix
- [ ] Basic authentication
- [ ] SQL injection prevention

### Week 2 (Core Functionality)
- [ ] Complete fuzzer integration
- [ ] Fix resource leaks
- [ ] Database migration

### Week 3-4 (Reliability)
- [ ] Input validation
- [ ] Rate limiting
- [ ] Error handling improvements
- [ ] Logging enhancements

### Ongoing
- [ ] Test coverage improvement
- [ ] Documentation updates
- [ ] Performance optimization
- [ ] Security hardening

## 📊 Success Metrics

- Zero critical security vulnerabilities
- 80%+ test coverage
- All TODO comments addressed
- Proper error handling throughout
- Complete feature implementations
- Performance benchmarks established

## 🚀 Quick Wins

1. Fix path traversal (1 hour)
2. Add basic auth middleware (4 hours)
3. Complete corpus hashing (2 hours)
4. Fix error suppression in frontend (1 hour)
5. Add input validation to job creation (2 hours)

## 📝 Notes

- Prioritize security fixes before any production deployment
- Consider feature freeze until critical issues are resolved
- Set up security scanning in CI/CD pipeline
- Schedule regular security reviews
- Document all security-sensitive code

---

Last Updated: 2025-06-21
Next Review: 2025-06-28