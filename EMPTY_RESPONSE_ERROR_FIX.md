# Fix: Improved Error Message for Empty API Responses

## Issue Description

**Location:** `provider.go`, line 324 (OpenAIProvider.Chat method)

**Severity:** Medium (improves debugging and diagnostics)

**Problem:** When an OpenAI-compatible API returns a response with an empty `choices` array, error message was generic and provided no useful diagnostic information:

```go
if len(chatResp.Choices) == 0 {
    return "", fmt.Errorf("no choices in response")
}
```

## Why This Matters

1. **Poor Diagnostics:** If an API returns an empty response due to:
   - Invalid model configuration
   - Rate limiting or quota issues
   - API changes or deprecations
   - Provider-specific issues
   
   The error `"no choices in response"` gives no context about what went wrong.

2. **Missing Debugging Info:** Developers or users cannot determine:
   - Which provider returned the error
   - What model was being used
   - The HTTP status code
   - The actual API response body (which might contain error details)

3. **Inconsistent with Other Errors:** Other error messages in the codebase include more context:
   - `"API error (status 429): Rate limit exceeded"`
   - `"request failed: connection reset by peer"`

## Example Scenario

**Before Fix:**
```
Error: no choices in response
```
User has no way to know if this is:
- A configuration issue (wrong model name)
- A provider issue (API outage)
- A quota issue (rate limit)
- A bug in the code

**After Fix:**
```
Error: no choices in API response (provider: openai, model: gpt-4.5-turbo, status: 200, body: {"error":{"message":"Model not found","type":"invalid_request_error","code":"model_not_found"}}...)
```
User immediately knows:
- The model name `gpt-4.5-turbo` is incorrect
- The API returned status 200 but with an error in the body
- The exact error message from the provider

## Solution

Replace the generic error message with a detailed, contextualized error:

```go
if len(chatResp.Choices) == 0 {
    // Include full response details in error for better debugging
    var responseSnippet string
    if len(body) > 500 {
        responseSnippet = string(body)[:500] + "..."
    } else {
        responseSnippet = string(body)
    }
    return "", fmt.Errorf("no choices in API response (provider: %s, model: %s, status: %d, body: %s)",
        p.Name(), model, resp.StatusCode, responseSnippet)
}
```

## Key Features of the Fix

1. **Provider Name:** Shows which API provider returned the error (`p.Name()`)
2. **Model Name:** Shows the model that was requested (`model` variable)
3. **HTTP Status Code:** Shows the actual HTTP status (`resp.StatusCode`)
4. **Response Body:** Shows the API response body with:
   - Truncation to 500 characters for very large responses
   - Ellipsis indicator (`...`) when truncated
   - Full body for smaller responses

## Benefits

1. **Better User Experience:** Users get actionable information immediately
2. **Faster Debugging:** Developers can identify issues without adding debug logging
3. **Production Readiness:** Operators can diagnose issues in production environments
4. **Maintains Backward Compatibility:** Error is still returned in the same format, just more detailed
5. **Memory Safe:** Limits response body length to prevent log flooding

## Testing

```bash
# 1. Build project
go build -o /tmp/reasoning-tools-check .
# Expected: No compilation errors

# 2. Run all tests
go test ./...
# Expected: All tests PASS

# 3. Run race detector
go test -race ./...
# Expected: No race conditions

# 4. Manual verification
# The fix will only be triggered when an API returns empty choices,
# which is an error condition that may be hard to test deterministically.
```

## Trade-offs and Alternatives Considered

### Alternative 1: Add Debug Logging Instead
- **Approach:** Keep error simple, add debug log with full details
- **Pros:** Less verbose in production
- **Cons:** Debug logs might not be available or accessible
- **Decision:** Rejected - error messages should be self-contained

### Alternative 2: Return Structured Error
- **Approach:** Return a struct with all details
- **Pros:** Programmatic access to all fields
- **Cons:** Breaking change to error handling contract
- **Decision:** Rejected - maintains backward compatibility

### Selected Approach (Implemented):
- **Pros:**
  - Error is human-readable and informative
  - No breaking changes to API
  - Immediate value for debugging
  - Safe memory usage with body truncation
- **Cons:**
  - Slightly more verbose error messages
- **Decision:** Accepted - benefits significantly outweigh minor verbosity increase

## Files Modified

1. `provider.go` (1 change):
   - Lines 324-332: Enhanced error message with context (provider, model, status, body)

## Verification Steps

The fix has been verified:
- ✅ Code compiles without errors
- ✅ All existing tests pass
- ✅ No race conditions detected
- ✅ Memory safety (body truncation to 500 chars)
- ✅ Maintains backward compatibility (still returns `error`)

## Future Considerations

1. **Consistent Error Format:** Consider standardizing error message format across all providers
2. **Error Categories:** Could categorize errors (configuration, rate limit, quota, etc.)
3. **Retry Logic:** Empty responses might be transient and worth retrying
4. **Metrics:** Could collect metrics on error types for monitoring

---

**Fix Applied:** 2026-01-19  
**Status:** ✅ Fixed and verified  
**Test Status:** ✅ All tests passing  
**Type:** Enhancement (improves diagnostics without changing behavior)
