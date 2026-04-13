# Security Review - LogSieve

**Date**: 2025-12-31
**Reviewer**: Security Audit (Automated)
**Scope**: Full codebase review

---

## [HIGH] Regex Denial of Service (ReDoS) Risk
- **File**: pkg/profiles/parser.go:91-130
- **Type**: Denial of Service / ReDoS
- **Description**: User-provided regex patterns compiled without validation for catastrophic backtracking.
- **Risk**: Patterns like `(a+)+b` cause exponential backtracking, leading to CPU exhaustion.
- **Recommendation**: Consider using RE2 library for guaranteed linear time complexity.

---

## [HIGH] Path Traversal in Profile Loading
- **File**: pkg/profiles/manager.go:129-149
- **Type**: Path Traversal
- **Description**: Profile files loaded without sanitizing filenames, symlinks could read sensitive files.
- **Recommendation**: Use `filepath.Clean()`, reject symlinks, set strict file permissions.

---

## [HIGH] Insufficient Profile Signature Verification
- **File**: pkg/profiles/manager.go:327-360
- **Type**: Insecure Authentication
- **Description**: In "relaxed" or "offline" trust modes, signature verification bypassed entirely.
- **Risk**: Default mode is "relaxed" - malicious profiles can be loaded.
- **Recommendation**: Make "strict" mode the default.

---

## [MEDIUM] SSRF via Output URLs
- **File**: pkg/output/loki.go, elasticsearch.go
- **Type**: Server-Side Request Forgery
- **Description**: Output URLs from config used without validation.
- **Risk**: Could probe internal services, cloud metadata endpoints.
- **Recommendation**: Implement URL validation, block private IP ranges.

---

## [MEDIUM] Path Traversal in Disk Buffer
- **File**: pkg/ingestion/disk_buffer.go:98, 187
- **Type**: Path Traversal
- **Description**: Disk buffer directory from config without validation.
- **Recommendation**: Validate diskPath, ensure absolute path within expected bounds.

---

## [MEDIUM] Unbounded Memory Consumption
- **File**: pkg/ingestion/handler.go:88-96
- **Type**: Denial of Service
- **Description**: JSON parsing without depth limits could exhaust memory.
- **Recommendation**: Use streaming JSON parser with depth limits.

---

## [MEDIUM] Regex Pattern Injection
- **File**: pkg/profiles/parser.go:215-230
- **Type**: Regular Expression Injection
- **Description**: Transform rules apply regex on untrusted log messages.
- **Recommendation**: Set timeouts for regex operations.

---

## [MEDIUM] Information Disclosure in Error Messages
- **File**: pkg/ingestion/handler.go:100-106
- **Type**: Information Disclosure
- **Description**: Detailed error messages returned to clients.
- **Recommendation**: Return generic errors, log details server-side.

---

## [MEDIUM] Missing Rate Limiting
- **File**: pkg/ingestion/handler.go:74-171
- **Type**: Denial of Service
- **Description**: `/ingest` endpoint has no rate limiting.
- **Recommendation**: Implement per-source or per-IP rate limiting.

---

## [MEDIUM] Weak Cryptographic Signature Scheme
- **File**: pkg/profiles/manager.go:362-390
- **Type**: Insecure Cryptography
- **Description**: Simple concatenation-based message construction, not standard canonicalization.
- **Recommendation**: Use canonical JSON, consider JWS standard.

---

## [LOW] World-Readable Config File
- **File**: pkg/config/loader.go:232
- **Type**: Insecure File Permissions
- **Description**: Example config created with 0644 permissions.
- **Recommendation**: Use 0600 permissions.

---

## [LOW] Missing Query Parameter Validation
- **File**: pkg/ingestion/handler.go:77-85
- **Type**: Input Validation
- **Description**: Query parameters accepted without validation.
- **Recommendation**: Validate against allowed values.

---

## [LOW] Lack of TLS/HTTPS Enforcement
- **File**: cmd/server/main.go:95-101
- **Type**: Insecure Transport
- **Description**: HTTP server starts without TLS.
- **Recommendation**: Add TLS configuration options.

---

## [LOW] Missing HTTP Security Headers
- **File**: cmd/server/main.go
- **Type**: Missing Security Controls
- **Description**: No security headers middleware.
- **Recommendation**: Add X-Content-Type-Options, X-Frame-Options.

---

## Summary

**Total Issues**: 14
- Critical: 0
- High: 3
- Medium: 7
- Low: 4

**Priority Actions**:
1. Enable strict signature mode by default
2. Add regex timeout protection
3. Implement SSRF prevention
4. Add rate limiting
