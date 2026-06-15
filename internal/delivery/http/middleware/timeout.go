package middleware

import "time"

// defaultLookupTimeout caps the database lookup inside middleware so a
// stalled DB doesn't hold the request open indefinitely.
const defaultLookupTimeout = 5 * time.Second
